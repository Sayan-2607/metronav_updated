// Package sim is MetroNav's synthetic "digital twin": it generates train
// positions, station crowding and coach occupancy from the documented demand
// model in package network, and applies operator scenarios (delays, surges,
// closures). Real sensor observations (e.g. from the CV pipeline) override
// simulated station density while they are fresh.
//
// Everything produced here is SIMULATED unless Source == "cv".
package sim

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/metronav/core/internal/network"
	"github.com/metronav/core/internal/route"
)

const TerminalTurnMin = 2.0

type Train struct {
	ID          string    `json:"id"`
	Line        string    `json:"line"`
	Color       string    `json:"color"`
	Direction   int       `json:"direction"` // +1 = towards last station in line list, -1 = towards first
	Towards     string    `json:"towards"`
	PrevStation string    `json:"prev_station"`
	NextStation string    `json:"next_station"`
	ETANextMin  float64   `json:"eta_next_min"`
	X           float64   `json:"x"`
	Y           float64   `json:"y"`
	AtTerminal  bool      `json:"at_terminal"`
	Delayed     bool      `json:"delayed"`
	Occupancy   float64   `json:"occupancy"`
	Coaches     []float64 `json:"coaches"`
	axis        float64   // position in minutes along forward axis [0, T]
	turnLeft    float64   // remaining terminal turnaround minutes
}

type Station struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Lines     []string `json:"lines"`
	Density   float64 `json:"density"`  // observed/simulated occupancy % of platform capacity
	Expected  float64 `json:"expected"` // demand-model baseline %
	People    int     `json:"people"`
	Capacity  int     `json:"capacity"`
	Source    string  `json:"source"` // simulated | cv
	Closed    bool    `json:"closed"`
	Level     string  `json:"level"` // low | medium | high | severe
}

type Scenario struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"` // train_delay | crowd_surge | station_closure
	StationID string    `json:"station_id,omitempty"`
	LineID    string    `json:"line_id,omitempty"`
	DelayMin  float64   `json:"delay_min,omitempty"`
	SurgePct  float64   `json:"surge_pct,omitempty"`
	Until     time.Time `json:"until"` // simulated time
}

type Snapshot struct {
	SimTime   time.Time  `json:"sim_time"`
	Trains    []Train    `json:"trains"`
	Stations  []Station  `json:"stations"`
	Scenarios []Scenario `json:"scenarios"`
}

type observation struct {
	value float64
	at    time.Time // wall clock
}

type Simulator struct {
	net   *network.Network
	loc   *time.Location
	speed float64
	start time.Time

	mu         sync.RWMutex
	rng        *rand.Rand
	noise      map[string]float64 // bounded random walk per station/train
	lineFreeze map[string]float64 // accumulated minutes trains on a line were held
	scenarios  []Scenario
	obs        map[string]observation
	snap       Snapshot
	seq        int
	lastTick   time.Time
}

func New(n *network.Network, loc *time.Location, speed float64, seed int64) *Simulator {
	s := &Simulator{
		net: n, loc: loc, speed: speed, start: time.Now(),
		rng: rand.New(rand.NewSource(seed)), noise: map[string]float64{},
		lineFreeze: map[string]float64{}, obs: map[string]observation{},
	}
	s.Tick()
	return s
}

// Now returns simulated time.
func (s *Simulator) Now() time.Time {
	el := time.Since(s.start)
	return s.start.Add(time.Duration(float64(el) * s.speed)).In(s.loc)
}

func (s *Simulator) walk(key string, step, bound float64) float64 {
	v := s.noise[key] + s.rng.NormFloat64()*step
	v = math.Max(-bound, math.Min(bound, v*0.97)) // mean-reverting
	s.noise[key] = v
	return v
}

func level(d float64) string {
	switch {
	case d >= 90:
		return "severe"
	case d >= 70:
		return "high"
	case d >= 40:
		return "medium"
	}
	return "low"
}

// Tick advances the simulation and rebuilds the snapshot.
func (s *Simulator) Tick() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.Now()
	wall := time.Now()
	dtSim := 0.0
	if !s.lastTick.IsZero() {
		dtSim = wall.Sub(s.lastTick).Minutes() * s.speed
	}
	s.lastTick = wall

	// expire scenarios, accumulate delays
	active := s.scenarios[:0]
	for _, sc := range s.scenarios {
		if now.Before(sc.Until) {
			active = append(active, sc)
			if sc.Type == "train_delay" {
				s.lineFreeze[sc.LineID] += dtSim
			}
		}
	}
	s.scenarios = active

	hour := network.HourOf(now)
	wd := now.Weekday()
	closed, surge, delayedLines := s.effectsLocked()

	stations := make([]Station, 0, len(s.net.Stations))
	density := map[string]float64{}
	for _, st := range s.net.Stations {
		exp := network.BaseOccupancy(st.Weight, hour, wd)
		d := exp + s.walk("st:"+st.ID, 1.2, 8)
		d *= 1 + surge[st.ID]/100
		for _, l := range s.net.LinesAt(st.ID) {
			if delayedLines[l] {
				d += 15 // passengers accumulate while service is held
			}
		}
		src := "simulated"
		if o, ok := s.obs[st.ID]; ok && wall.Sub(o.at) < 90*time.Second {
			d, src = o.value, "cv"
		}
		if closed[st.ID] {
			d = 2
		}
		d = network.Clamp(d, 0, 100)
		density[st.ID] = d
		stations = append(stations, Station{
			ID: st.ID, Name: st.Name, X: st.X, Y: st.Y, Lines: s.net.LinesAt(st.ID),
			Density: round1(d), Expected: round1(exp), People: int(d / 100 * float64(st.PlatformCapacity)),
			Capacity: st.PlatformCapacity, Source: src, Closed: closed[st.ID], Level: level(d),
		})
	}

	var trains []Train
	minOfDay := float64(now.Hour()*60+now.Minute()) + float64(now.Second())/60
	for _, l := range s.net.Lines {
		cum := cumulative(&l)
		T := cum[len(cum)-1]
		cycle := 2*T + 2*TerminalTurnMin
		n := int(math.Ceil(cycle / l.HeadwayMin))
		for i := 0; i < n; i++ {
			p := math.Mod(minOfDay-s.lineFreeze[l.ID]+float64(i)*l.HeadwayMin, cycle)
			if p < 0 {
				p += cycle
			}
			tr := s.placeTrain(&l, cum, T, p, i)
			tr.Delayed = delayedLines[l.ID]
			tr.Color = l.Color
			// occupancy: average demand over the next few stations in travel direction
			tr.Occupancy = s.trainLoad(&l, tr, density)
			anchor := tr.PrevStation
			if anchor == "" {
				anchor = tr.NextStation
			}
			bias := network.CoachBias(anchor, l.Coaches)
			tr.Coaches = make([]float64, l.Coaches)
			for c := range bias {
				v := tr.Occupancy*bias[c] + s.walk(fmt.Sprintf("tc:%s:%d", tr.ID, c), 0.8, 5)
				tr.Coaches[c] = round1(network.Clamp(v, 0, 100))
			}
			trains = append(trains, tr)
		}
	}
	s.snap = Snapshot{SimTime: now, Trains: trains, Stations: stations, Scenarios: append([]Scenario(nil), s.scenarios...)}
	return s.snap
}

func (s *Simulator) effectsLocked() (closed map[string]bool, surge map[string]float64, delayed map[string]bool) {
	closed, surge, delayed = map[string]bool{}, map[string]float64{}, map[string]bool{}
	for _, sc := range s.scenarios {
		switch sc.Type {
		case "station_closure":
			closed[sc.StationID] = true
			for _, l := range s.net.Lines { // neighbours absorb displaced demand
				if i := l.Index(sc.StationID); i >= 0 {
					if i > 0 {
						surge[l.Stations[i-1]] += 20
					}
					if i < len(l.Stations)-1 {
						surge[l.Stations[i+1]] += 20
					}
				}
			}
		case "crowd_surge":
			surge[sc.StationID] += sc.SurgePct
		case "train_delay":
			delayed[sc.LineID] = true
		}
	}
	return
}

func cumulative(l *network.Line) []float64 {
	cum := make([]float64, len(l.Stations))
	for i, seg := range l.SegmentMin {
		cum[i+1] = cum[i] + seg + route.DwellMin
	}
	return cum
}

func (s *Simulator) placeTrain(l *network.Line, cum []float64, T, p float64, i int) Train {
	tr := Train{ID: fmt.Sprintf("%s-%02d", l.ID[:1], i+1), Line: l.ID}
	first, last := l.Stations[0], l.Stations[len(l.Stations)-1]
	switch {
	case p < T:
		tr.Direction, tr.axis = 1, p
	case p < T+TerminalTurnMin:
		tr.Direction, tr.axis, tr.AtTerminal = -1, T, true
		tr.ETANextMin = T + TerminalTurnMin - p
	case p < 2*T+TerminalTurnMin:
		tr.Direction, tr.axis = -1, T-(p-T-TerminalTurnMin)
	default:
		tr.Direction, tr.axis, tr.AtTerminal = 1, 0, true
		tr.ETANextMin = 2*T + 2*TerminalTurnMin - p
	}
	if tr.Direction == 1 {
		tr.Towards = last
	} else {
		tr.Towards = first
	}
	if tr.AtTerminal {
		tr.turnLeft = tr.ETANextMin
		idx := 0
		if tr.axis > 0 {
			idx = len(l.Stations) - 1
		}
		tr.PrevStation = l.Stations[idx]
		if tr.Direction == 1 {
			tr.NextStation = l.Stations[1]
			tr.ETANextMin += cum[1]
		} else {
			tr.NextStation = l.Stations[idx-1]
			tr.ETANextMin += cum[idx] - cum[idx-1]
		}
	} else {
		// a: forward segment index with cum[a] <= axis < cum[a+1]
		a := sort.SearchFloat64s(cum, tr.axis)
		if a >= len(cum) || cum[a] > tr.axis {
			a--
		}
		if a < 0 {
			a = 0
		}
		if a > len(cum)-2 {
			a = len(cum) - 2
		}
		if tr.Direction == 1 {
			tr.PrevStation, tr.NextStation = l.Stations[a], l.Stations[a+1]
			tr.ETANextMin = cum[a+1] - tr.axis
		} else {
			tr.PrevStation, tr.NextStation = l.Stations[a+1], l.Stations[a]
			tr.ETANextMin = tr.axis - cum[a]
		}
	}
	tr.ETANextMin = round1(tr.ETANextMin)
	// interpolate schematic coordinates
	sa, _ := s.net.Station(tr.PrevStation)
	sb, _ := s.net.Station(tr.NextStation)
	segLen := math.Abs(cum[l.Index(tr.NextStation)] - cum[l.Index(tr.PrevStation)])
	f := 0.0
	if segLen > 0 && !tr.AtTerminal {
		f = 1 - tr.ETANextMin/segLen
	}
	f = network.Clamp(f, 0, 1)
	tr.X = round1(sa.X + (sb.X-sa.X)*f)
	tr.Y = round1(sa.Y + (sb.Y-sa.Y)*f)
	return tr
}

func (s *Simulator) trainLoad(l *network.Line, tr Train, density map[string]float64) float64 {
	i := l.Index(tr.NextStation)
	sum, n := 0.0, 0.0
	for k := 0; k < 4; k++ {
		j := i + k*tr.Direction
		if j < 0 || j >= len(l.Stations) {
			break
		}
		sum += density[l.Stations[j]]
		n++
	}
	occ := 0.0
	if n > 0 {
		occ = sum / n * 0.95
	}
	// trains fill towards the city centre; this is a synthetic assumption
	occ += s.walk("tr:"+tr.ID, 1.0, 6)
	if tr.Delayed {
		occ += 12
	}
	return round1(network.Clamp(occ, 0, 100))
}

// ---- read API ----

func (s *Simulator) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snap
}

func (s *Simulator) Density(stationID string) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, st := range s.snap.Stations {
		if st.ID == stationID {
			return st.Density
		}
	}
	return 0
}

func (s *Simulator) Closed() map[string]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, _, _ := s.effectsLocked()
	return c
}

func (s *Simulator) Train(id string) (Train, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.snap.Trains {
		if t.ID == id {
			return t, true
		}
	}
	return Train{}, false
}

type Arrival struct {
	Train  Train   `json:"train"`
	ETAMin float64 `json:"eta_min"`
}

// Arrivals lists trains on a line that will reach stationID travelling in dir.
func (s *Simulator) Arrivals(stationID, lineID string, dir int, limit int) []Arrival {
	l, ok := s.net.Line(lineID)
	if !ok {
		return nil
	}
	idx := l.Index(stationID)
	if idx < 0 {
		return nil
	}
	cum := cumulative(l)
	T := cum[len(cum)-1]
	target := cum[idx]
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Arrival
	for _, t := range s.snap.Trains {
		if t.Line != lineID {
			continue
		}
		var eta float64
		switch {
		case t.Direction == dir && dir == 1 && t.axis <= target:
			eta = target - t.axis
		case t.Direction == dir && dir == -1 && t.axis >= target:
			eta = t.axis - target
		case dir == 1: // must turn at far end (or already past)
			if t.Direction == -1 {
				eta = t.axis + TerminalTurnMin + target
			} else {
				eta = (T - t.axis) + TerminalTurnMin + T + TerminalTurnMin + target
			}
		default:
			if t.Direction == 1 {
				eta = (T - t.axis) + TerminalTurnMin + (T - target)
			} else {
				eta = t.axis + TerminalTurnMin + T + TerminalTurnMin + (T - target)
			}
		}
		eta += t.turnLeft
		out = append(out, Arrival{Train: t, ETAMin: round1(eta)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ETAMin < out[j].ETAMin })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// ---- write API ----

func (s *Simulator) AddScenario(sc Scenario, durationMin float64) (Scenario, error) {
	switch sc.Type {
	case "train_delay":
		if _, ok := s.net.Line(sc.LineID); !ok {
			return sc, fmt.Errorf("unknown line %q", sc.LineID)
		}
		if sc.DelayMin <= 0 || sc.DelayMin > 120 {
			return sc, fmt.Errorf("delay_min must be in (0,120]")
		}
		durationMin = sc.DelayMin
	case "crowd_surge":
		if _, ok := s.net.Station(sc.StationID); !ok {
			return sc, fmt.Errorf("unknown station %q", sc.StationID)
		}
		if sc.SurgePct <= 0 || sc.SurgePct > 300 {
			return sc, fmt.Errorf("surge_pct must be in (0,300]")
		}
	case "station_closure":
		if _, ok := s.net.Station(sc.StationID); !ok {
			return sc, fmt.Errorf("unknown station %q", sc.StationID)
		}
	default:
		return sc, fmt.Errorf("unknown scenario type %q", sc.Type)
	}
	if durationMin <= 0 {
		durationMin = 15
	}
	s.mu.Lock()
	s.seq++
	sc.ID = fmt.Sprintf("SCN-%04d", s.seq)
	sc.Until = s.Now().Add(time.Duration(durationMin * float64(time.Minute)))
	s.scenarios = append(s.scenarios, sc)
	s.mu.Unlock()
	return sc, nil
}

func (s *Simulator) ClearScenario(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, sc := range s.scenarios {
		if sc.ID == id {
			s.scenarios = append(s.scenarios[:i], s.scenarios[i+1:]...)
			return true
		}
	}
	return false
}

// Observe records a real measurement (e.g. from the CV pipeline).
func (s *Simulator) Observe(stationID string, density float64) error {
	if _, ok := s.net.Station(stationID); !ok {
		return fmt.Errorf("unknown station %q", stationID)
	}
	s.mu.Lock()
	s.obs[stationID] = observation{value: network.Clamp(density, 0, 100), at: time.Now()}
	s.mu.Unlock()
	return nil
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
