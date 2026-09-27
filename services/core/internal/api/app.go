// Package api wires the core services together and exposes the HTTP/WebSocket
// interface. The deployment is a modular monolith: route, ticket, crowd,
// train, realtime, incident and simulation "services" are separate packages
// with narrow interfaces, served from one Go binary. They can be split into
// separate processes later without changing the package boundaries.
package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/metronav/core/internal/cache"
	"github.com/metronav/core/internal/config"
	"github.com/metronav/core/internal/crowd"
	"github.com/metronav/core/internal/metrics"
	"github.com/metronav/core/internal/ml"
	"github.com/metronav/core/internal/network"
	"github.com/metronav/core/internal/realtime"
	"github.com/metronav/core/internal/route"
	"github.com/metronav/core/internal/sim"
	"github.com/metronav/core/internal/store"
)

type sample struct {
	at time.Time
	d  float64
}

type App struct {
	Cfg     config.Config
	Net     *network.Network
	Routes  *route.Engine
	Sim     *sim.Simulator
	Store   store.Store
	Redis   *cache.Redis // may be nil
	ML      *ml.Client
	Hub     *realtime.Hub
	Monitor *crowd.Monitor
	Metrics *metrics.Registry
	Log     *slog.Logger
	Started time.Time

	limiter *limiter
	etaCache   *ttlCache[ml.ETAPrediction]
	crowdCache *ttlCache[ml.CrowdForecast]
	histMu  sync.RWMutex
	history map[string][]sample // per-station density, simulated time, last 90 min
}

func NewID(prefix string) string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return prefix + "-" + strings.ToUpper(hex.EncodeToString(b))
}

func (a *App) Init() {
	a.history = map[string][]sample{}
	a.limiter = newLimiter(a.Cfg.RateLimitRPS, 2*a.Cfg.RateLimitRPS) // per client IP
	a.Started = time.Now()
	a.etaCache = newTTLCache[ml.ETAPrediction](30 * time.Second)
	a.crowdCache = newTTLCache[ml.CrowdForecast](30 * time.Second)
}

// Run drives the simulation clock and all periodic jobs until ctx ends.
func (a *App) Run(ctx context.Context) {
	t := time.NewTicker(a.Cfg.TickInterval)
	defer t.Stop()
	n := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		n++
		start := time.Now()
		snap := a.Sim.Tick()
		a.recordHistory(snap)

		a.Hub.Broadcast(a.Hub.NewEvent(realtime.TrainPositions, "simulator", map[string]any{"sim_time": snap.SimTime, "trains": snap.Trains}))
		a.Hub.Broadcast(a.Hub.NewEvent(realtime.CrowdUpdated, "simulator", map[string]any{"sim_time": snap.SimTime, "stations": snap.Stations, "scenarios": snap.Scenarios}))
		if a.Redis != nil {
			go a.mirrorToRedis(snap)
		}
		if n%5 == 0 {
			a.detectAnomalies(ctx, snap)
		}
		if n%15 == 0 {
			a.persistOccupancy(ctx, snap)
		}
		a.Metrics.Inc("metronav_ticks_total", nil)
		a.Metrics.Observe("metronav_tick_seconds", nil, time.Since(start).Seconds())
		a.Metrics.Set("metronav_active_trains", nil, float64(len(snap.Trains)))
	}
}

func (a *App) recordHistory(snap sim.Snapshot) {
	a.histMu.Lock()
	defer a.histMu.Unlock()
	cut := snap.SimTime.Add(-90 * time.Minute)
	for _, st := range snap.Stations {
		h := append(a.history[st.ID], sample{snap.SimTime, st.Density})
		i := 0
		for i < len(h) && h[i].at.Before(cut) {
			i++
		}
		a.history[st.ID] = h[i:]
	}
}

// densityAgo returns the recorded density closest to `ago` before the latest sample.
func (a *App) densityAgo(stationID string, now time.Time, ago time.Duration, fallback float64) float64 {
	a.histMu.RLock()
	defer a.histMu.RUnlock()
	h := a.history[stationID]
	target := now.Add(-ago)
	best, bestDiff := fallback, time.Duration(math.MaxInt64)
	for _, s := range h {
		d := s.at.Sub(target)
		if d < 0 {
			d = -d
		}
		if d < bestDiff {
			best, bestDiff = s.d, d
		}
	}
	if bestDiff > 5*time.Minute {
		return fallback
	}
	return best
}

func (a *App) mirrorToRedis(snap sim.Snapshot) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	kv := map[string]string{}
	for _, st := range snap.Stations {
		kv["station:"+st.ID+":density"] = fmt.Sprintf("%.1f", st.Density)
	}
	for _, tr := range snap.Trains {
		kv["train:"+tr.ID+":occupancy"] = fmt.Sprintf("%.1f", tr.Occupancy)
		kv["train:"+tr.ID+":location"] = fmt.Sprintf("%s>%s", tr.PrevStation, tr.NextStation)
		for c, v := range tr.Coaches {
			kv[fmt.Sprintf("coach:%s:C%d:occupancy", tr.ID, c+1)] = fmt.Sprintf("%.1f", v)
		}
	}
	if err := a.Redis.PutState(ctx, kv, time.Minute); err != nil {
		a.Metrics.Inc("metronav_redis_errors_total", nil)
		return
	}
	_ = a.Redis.Publish(ctx, realtime.CrowdUpdated, map[string]any{"sim_time": snap.SimTime, "n_stations": len(snap.Stations)})
}

func (a *App) detectAnomalies(ctx context.Context, snap sim.Snapshot) {
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	findings := a.Monitor.Detect(c, snap)
	incs := a.Monitor.ToIncidents(findings, time.Now(), func() string { return NewID("INC") })
	for i := range incs {
		if err := a.Store.CreateIncident(c, &incs[i]); err != nil {
			a.Log.Warn("store incident", "err", err)
			continue
		}
		a.Hub.Broadcast(a.Hub.NewEvent(realtime.IncidentCreated, "anomaly_detector", incs[i]))
		a.Metrics.Inc("metronav_incidents_total", map[string]string{"source": "anomaly_detector"})
	}
}

func (a *App) persistOccupancy(ctx context.Context, snap sim.Snapshot) {
	recs := make([]store.OccupancyRecord, 0, len(snap.Stations))
	for _, st := range snap.Stations {
		recs = append(recs, store.OccupancyRecord{StationID: st.ID, At: snap.SimTime, Density: st.Density, Expected: st.Expected, Source: st.Source})
	}
	c, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := a.Store.RecordOccupancy(c, recs); err != nil {
		a.Log.Warn("persist occupancy", "err", err)
	}
}

// ---- domain helpers used by handlers ----

// CoachRecommendation ranks coaches by a transparent weighted score:
// score = 0.7*occupancy/100 + 0.3*distance(coach, destination exit coach)/(n-1).
// Lower is better. The exit-coach position is synthetic (see network.ExitCoach).
type CoachRecommendation struct {
	Recommended int       `json:"recommended_coach"` // 1-based
	ExitCoach   int       `json:"destination_exit_coach"`
	Scores      []float64 `json:"scores"`
	Formula     string    `json:"formula"`
}

func RecommendCoach(coaches []float64, destID string) CoachRecommendation {
	n := len(coaches)
	exit := network.ExitCoach(destID, n)
	denom := math.Max(float64(n-1), 1)
	best, bestScore := 0, math.Inf(1)
	scores := make([]float64, n)
	for c, occ := range coaches {
		s := 0.7*occ/100 + 0.3*math.Abs(float64(c-exit))/denom
		scores[c] = math.Round(s*1000) / 1000
		if s < bestScore {
			best, bestScore = c, s
		}
	}
	return CoachRecommendation{Recommended: best + 1, ExitCoach: exit + 1, Scores: scores,
		Formula: "0.7*occupancy/100 + 0.3*|coach - exit_coach|/(n-1); lower is better"}
}

// FallbackCrowd forecasts by decaying the current residual towards the
// demand-model baseline: f(h) = base(t+h) + (obs - base(t)) * exp(-h/30).
func FallbackCrowd(st *network.Station, now time.Time, current float64) ml.CrowdForecast {
	base := func(t time.Time) float64 { return network.BaseOccupancy(st.Weight, network.HourOf(t), t.Weekday()) }
	resid := current - base(now)
	f := func(h float64) float64 {
		v := base(now.Add(time.Duration(h*float64(time.Minute)))) + resid*math.Exp(-h/30)
		return math.Round(network.Clamp(v, 0, 100)*10) / 10
	}
	return ml.CrowdForecast{StationID: st.ID, Forecast15: f(15), Forecast30: f(30), Forecast60: f(60), Model: "baseline-residual-decay", Source: "fallback"}
}

// FallbackETA is a heuristic used only when the ML service is unavailable.
func FallbackETA(r ml.ETARequest) ml.ETAPrediction {
	p50 := r.ScheduledMin*(1+0.08*r.MeanCrowd/100) + 0.5*float64(r.Transfers)
	if r.LineDelayed {
		p50 += 5
	}
	return ml.ETAPrediction{
		P10: math.Round((p50-1-0.03*r.ScheduledMin)*10) / 10, P50: math.Round(p50*10) / 10,
		P90: math.Round((p50+2+0.08*r.ScheduledMin)*10) / 10, Model: "heuristic", Source: "fallback",
	}
}
