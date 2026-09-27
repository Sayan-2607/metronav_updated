// Package route implements multi-objective journey planning on a
// line-expanded metro graph: node = (line, station), edges = ride segments
// and in-station transfers. Dijkstra runs once per preference profile, each
// profile weighting time, transfers and crowding differently.
package route

import (
	"container/heap"
	"errors"
	"math"
	"sort"

	"github.com/metronav/core/internal/network"
)

const (
	DwellMin           = 0.5 // assumed dwell per intermediate stop
	DefaultTransferMin = 5.0
)

type Profile string

const (
	Fastest         Profile = "fastest"
	LeastCrowded    Profile = "least_crowded"
	FewestTransfers Profile = "fewest_transfers"
	Balanced        Profile = "balanced"
)

var AllProfiles = []Profile{Fastest, LeastCrowded, FewestTransfers, Balanced}

// Weights define a profile's cost function:
// cost = Time*minutes + Transfer*transfers + Crowd*crowdPenalty(minutes, occupancy)
type Weights struct{ Time, Transfer, Crowd float64 }

func WeightsFor(p Profile) Weights {
	switch p {
	case LeastCrowded:
		return Weights{Time: 1, Transfer: 2, Crowd: 3}
	case FewestTransfers:
		return Weights{Time: 1, Transfer: 30, Crowd: 0}
	case Balanced:
		return Weights{Time: 1, Transfer: 5, Crowd: 1}
	default:
		return Weights{Time: 1, Transfer: 0, Crowd: 0}
	}
}

// CrowdFunc returns current occupancy (0-100) for a station.
type CrowdFunc func(stationID string) float64

type Options struct {
	Crowd  CrowdFunc
	Closed map[string]bool // stations unavailable for boarding, alighting or transfer
}

type Leg struct {
	Line    string   `json:"line"`
	Color   string   `json:"color"`
	From    string   `json:"from"`
	To      string   `json:"to"`
	Stops   []string `json:"stops"`
	Minutes float64  `json:"minutes"`
}

type Result struct {
	Profile         Profile `json:"profile"`
	Profiles        []Profile `json:"matched_profiles"`
	TotalMin        float64 `json:"total_min"`
	WaitMin         float64 `json:"wait_min"`
	RideMin         float64 `json:"ride_min"`
	WalkTransferMin float64 `json:"transfer_walk_min"`
	Transfers       int     `json:"transfers"`
	StationsCount   int     `json:"stations_travelled"`
	Fare            int     `json:"fare"`
	CrowdScore      float64 `json:"crowd_score"`
	Legs            []Leg   `json:"legs"`
	Cost            float64 `json:"cost"`
}

type node struct {
	line    string
	station string
}

type edge struct {
	to       node
	minutes  float64
	wait     float64
	transfer bool
	station  string // station whose crowding applies
}

type Engine struct {
	net *network.Network
	adj map[node][]edge
}

func New(n *network.Network) *Engine {
	e := &Engine{net: n, adj: map[node][]edge{}}
	for _, l := range n.Lines {
		for i := 0; i < len(l.Stations)-1; i++ {
			a := node{l.ID, l.Stations[i]}
			b := node{l.ID, l.Stations[i+1]}
			m := l.SegmentMin[i] + DwellMin
			e.adj[a] = append(e.adj[a], edge{to: b, minutes: m, station: b.station})
			e.adj[b] = append(e.adj[b], edge{to: a, minutes: m, station: a.station})
		}
	}
	for _, s := range n.Stations {
		lines := n.LinesAt(s.ID)
		walk := DefaultTransferMin
		if v, ok := n.TransferMin[s.ID]; ok {
			walk = v
		}
		for _, la := range lines {
			for _, lb := range lines {
				if la == lb {
					continue
				}
				lineB, _ := n.Line(lb)
				e.adj[node{la, s.ID}] = append(e.adj[node{la, s.ID}], edge{
					to: node{lb, s.ID}, minutes: walk, wait: lineB.HeadwayMin / 2, transfer: true, station: s.ID,
				})
			}
		}
	}
	return e
}

type item struct {
	n    node
	cost float64
	idx  int
}
type pq []*item

func (p pq) Len() int            { return len(p) }
func (p pq) Less(i, j int) bool  { return p[i].cost < p[j].cost }
func (p pq) Swap(i, j int)       { p[i], p[j] = p[j], p[i]; p[i].idx = i; p[j].idx = j }
func (p *pq) Push(x any)         { it := x.(*item); it.idx = len(*p); *p = append(*p, it) }
func (p *pq) Pop() any           { old := *p; it := old[len(old)-1]; *p = old[:len(old)-1]; return it }

var (
	ErrUnknownStation = errors.New("unknown station")
	ErrSameStation    = errors.New("origin and destination are the same")
	ErrNoRoute        = errors.New("no route available")
	ErrClosed         = errors.New("origin or destination is closed")
)

func crowdPenalty(minutes, occ float64) float64 {
	x := occ / 100
	return minutes * x * x // convex: crowding hurts more as it approaches capacity
}

// Plan runs one Dijkstra search for a profile.
func (e *Engine) Plan(from, to string, p Profile, opt Options) (*Result, error) {
	if _, ok := e.net.Station(from); !ok {
		return nil, ErrUnknownStation
	}
	if _, ok := e.net.Station(to); !ok {
		return nil, ErrUnknownStation
	}
	if from == to {
		return nil, ErrSameStation
	}
	if opt.Closed[from] || opt.Closed[to] {
		return nil, ErrClosed
	}
	crowd := opt.Crowd
	if crowd == nil {
		crowd = func(string) float64 { return 0 }
	}
	w := WeightsFor(p)

	dist := map[node]float64{}
	prev := map[node]node{}
	prevEdge := map[node]edge{}
	h := &pq{}
	for _, lid := range e.net.LinesAt(from) {
		l, _ := e.net.Line(lid)
		start := node{lid, from}
		// initial wait: half the headway, penalised by origin crowding
		c := w.Time*(l.HeadwayMin/2) + w.Crowd*crowdPenalty(l.HeadwayMin/2, crowd(from))
		dist[start] = c
		heap.Push(h, &item{n: start, cost: c})
	}
	var best *node
	for h.Len() > 0 {
		it := heap.Pop(h).(*item)
		if it.cost > dist[it.n] {
			continue
		}
		if it.n.station == to {
			n := it.n
			best = &n
			break
		}
		for _, ed := range e.adj[it.n] {
			if ed.transfer && opt.Closed[ed.station] {
				continue
			}
			mins := ed.minutes + ed.wait
			c := w.Time*mins + w.Crowd*crowdPenalty(mins, crowd(ed.station))
			if ed.transfer {
				c += w.Transfer
			}
			nc := it.cost + c
			if old, ok := dist[ed.to]; !ok || nc < old {
				dist[ed.to] = nc
				prev[ed.to] = it.n
				prevEdge[ed.to] = ed
				heap.Push(h, &item{n: ed.to, cost: nc})
			}
		}
	}
	if best == nil {
		return nil, ErrNoRoute
	}
	return e.build(*best, prev, prevEdge, dist[*best], p, crowd), nil
}

func (e *Engine) build(end node, prev map[node]node, prevEdge map[node]edge, cost float64, p Profile, crowd CrowdFunc) *Result {
	var path []node
	var edges []edge
	for n := end; ; {
		path = append([]node{n}, path...)
		pn, ok := prev[n]
		if !ok {
			break
		}
		edges = append([]edge{prevEdge[n]}, edges...)
		n = pn
	}
	firstLine, _ := e.net.Line(path[0].line)
	r := &Result{Profile: p, Profiles: []Profile{p}, Cost: math.Round(cost*100) / 100, WaitMin: firstLine.HeadwayMin / 2}
	cur := Leg{Line: path[0].line, Color: firstLine.Color, From: path[0].station, Stops: []string{path[0].station}}
	crowdSum, crowdN := crowd(path[0].station), 1.0
	for i, ed := range edges {
		nx := path[i+1]
		if ed.transfer {
			r.Legs = append(r.Legs, cur)
			r.Transfers++
			r.WalkTransferMin += ed.minutes
			r.WaitMin += ed.wait
			l, _ := e.net.Line(nx.line)
			cur = Leg{Line: nx.line, Color: l.Color, From: nx.station, Stops: []string{nx.station}}
			continue
		}
		cur.Stops = append(cur.Stops, nx.station)
		cur.To = nx.station
		cur.Minutes += ed.minutes
		r.RideMin += ed.minutes
		r.StationsCount++
		crowdSum += crowd(nx.station)
		crowdN++
	}
	r.Legs = append(r.Legs, cur)
	r.TotalMin = round1(r.WaitMin + r.RideMin + r.WalkTransferMin)
	r.RideMin = round1(r.RideMin)
	r.WaitMin = round1(r.WaitMin)
	r.CrowdScore = round1(crowdSum / crowdN)
	r.Fare = e.net.Fare(r.StationsCount)
	for i := range r.Legs {
		r.Legs[i].Minutes = round1(r.Legs[i].Minutes)
	}
	return r
}

// PlanAll runs every profile and merges identical itineraries, so the client
// sees genuine trade-offs rather than duplicated routes.
func (e *Engine) PlanAll(from, to string, opt Options) ([]*Result, error) {
	var out []*Result
	seen := map[string]*Result{}
	var lastErr error
	for _, p := range AllProfiles {
		r, err := e.Plan(from, to, p, opt)
		if err != nil {
			lastErr = err
			continue
		}
		key := Signature(r)
		if ex, ok := seen[key]; ok {
			ex.Profiles = append(ex.Profiles, p)
			continue
		}
		seen[key] = r
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, lastErr
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TotalMin < out[j].TotalMin })
	return out, nil
}

// Signature identifies an itinerary by its legs.
func Signature(r *Result) string {
	s := ""
	for _, l := range r.Legs {
		s += l.Line + ":" + l.From + ">" + l.To + ";"
	}
	return s
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
