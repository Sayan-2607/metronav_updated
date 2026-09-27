package sim

import (
	"fmt"
	"math"

	"github.com/metronav/core/internal/network"
)

// WhatIfInput describes an operator "what happens if" question.
type WhatIfInput struct {
	StationID  string  `json:"station_id"`
	LineID     string  `json:"line_id"`
	DelayMin   float64 `json:"delay_min"`
	SurgePct   float64 `json:"surge_pct"`
	HorizonMin int     `json:"horizon_min"`
}

type WhatIfPoint struct {
	Minute        int     `json:"minute"`
	Queue         float64 `json:"queue"`
	PlatformPct   float64 `json:"platform_pct"`
	TrainArrived  bool    `json:"train_arrived"`
	LeftBehind    float64 `json:"left_behind"`
}

type WhatIfResult struct {
	Assumptions     []string      `json:"assumptions"`
	ArrivalPerMin   float64       `json:"arrival_rate_per_min"`
	BoardingPerTrain float64      `json:"boarding_capacity_per_train"`
	PeakQueue       float64       `json:"peak_queue"`
	PeakPlatformPct float64       `json:"peak_platform_pct"`
	SpilloverMinute int           `json:"spillover_minute"` // -1 = never exceeds platform capacity
	RecoveryMinute  int           `json:"recovery_minute"`  // first minute after delay with queue back to baseline, -1 = not within horizon
	MaxLeftBehind   float64       `json:"max_left_behind"`
	Series          []WhatIfPoint `json:"series"`
}

// WhatIf runs a deterministic minute-step fluid-queue projection for one
// platform. It is a first-order approximation, not a validated
// passenger-flow model; see Assumptions in the result.
func (s *Simulator) WhatIf(in WhatIfInput) (*WhatIfResult, error) {
	st, ok := s.net.Station(in.StationID)
	if !ok {
		return nil, fmt.Errorf("unknown station %q", in.StationID)
	}
	l, ok := s.net.Line(in.LineID)
	if !ok || l.Index(in.StationID) < 0 {
		return nil, fmt.Errorf("line %q does not serve %s", in.LineID, in.StationID)
	}
	if in.HorizonMin <= 0 || in.HorizonMin > 180 {
		in.HorizonMin = 45
	}
	if in.DelayMin < 0 || in.SurgePct < 0 {
		return nil, fmt.Errorf("delay_min and surge_pct must be non-negative")
	}

	snap := s.Snapshot()
	var density float64
	for _, x := range snap.Stations {
		if x.ID == st.ID {
			density = x.Density
		}
	}
	// average train load on this line right now
	load, n := 0.0, 0.0
	for _, t := range snap.Trains {
		if t.Line == l.ID {
			load += t.Occupancy
			n++
		}
	}
	if n > 0 {
		load /= n
	}
	cap := float64(st.PlatformCapacity)
	nLines := float64(len(s.net.LinesAt(st.ID)))
	q0 := density / 100 * cap / nLines // this line's share of the platform crowd
	lambda := q0 / l.HeadwayMin * (1 + in.SurgePct/100)
	board := float64(l.Coaches*l.CoachCapacity) * network.Clamp(1-load/100, 0.05, 1)

	res := &WhatIfResult{
		Assumptions: []string{
			"Platform crowd is a fluid queue; arrivals are constant at the current rate scaled by the surge.",
			"Current platform occupancy equals roughly one headway of arrivals for this line.",
			"Each train removes up to its free capacity (coaches x coach capacity x (1 - current line load)).",
			"No trains call during the delay; after it, trains resume at the scheduled headway (no bunching).",
			"Inputs are simulated unless a CV observation is live for this station.",
		},
		ArrivalPerMin: round1(lambda), BoardingPerTrain: round1(board),
		SpilloverMinute: -1, RecoveryMinute: -1,
	}
	q := q0
	nextTrain := in.DelayMin + l.HeadwayMin/2
	for m := 1; m <= in.HorizonMin; m++ {
		q += lambda
		p := WhatIfPoint{Minute: m}
		if float64(m) >= nextTrain {
			boarded := math.Min(q, board)
			q -= boarded
			p.TrainArrived, p.LeftBehind = true, round1(q)
			res.MaxLeftBehind = math.Max(res.MaxLeftBehind, q)
			nextTrain += l.HeadwayMin
		}
		p.Queue = round1(q)
		p.PlatformPct = round1(q * nLines / cap * 100)
		if q > res.PeakQueue {
			res.PeakQueue = q
		}
		if res.SpilloverMinute < 0 && q*nLines > cap {
			res.SpilloverMinute = m
		}
		if res.RecoveryMinute < 0 && float64(m) > in.DelayMin && p.TrainArrived && q <= q0*0.1+1 {
			res.RecoveryMinute = m
		}
		res.Series = append(res.Series, p)
	}
	res.PeakQueue = round1(res.PeakQueue)
	res.PeakPlatformPct = round1(res.PeakQueue * nLines / cap * 100)
	res.MaxLeftBehind = round1(res.MaxLeftBehind)
	return res, nil
}
