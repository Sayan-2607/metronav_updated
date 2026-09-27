// Package crowd turns station observations into anomaly incidents. The
// Python service supplies model-based expectations; if it is unavailable the
// monitor falls back to the demand-model baseline with a fixed noise scale.
package crowd

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/metronav/core/internal/ml"
	"github.com/metronav/core/internal/sim"
	"github.com/metronav/core/internal/store"
)

// FallbackSigma approximates the simulator's station-noise standard
// deviation (bounded random walk, step 1.2, bound 8). Used only in fallback.
const FallbackSigma = 4.0
const ZThreshold = 3.0

type Finding struct {
	StationID string  `json:"station_id"`
	Name      string  `json:"name"`
	Expected  float64 `json:"expected"`
	Observed  float64 `json:"observed"`
	Z         float64 `json:"z_score"`
	Method    string  `json:"method"` // ml | fallback
}

type Monitor struct {
	ml       *ml.Client
	mu       sync.Mutex
	lastOpen map[string]time.Time
	Cooldown time.Duration
}

func NewMonitor(c *ml.Client) *Monitor {
	return &Monitor{ml: c, lastOpen: map[string]time.Time{}, Cooldown: 10 * time.Minute}
}

func Severity(z, density float64) string {
	switch {
	case density >= 95 || z >= 6:
		return "CRITICAL"
	case z >= 4.5:
		return "HIGH"
	case z >= 3:
		return "MEDIUM"
	}
	return "LOW"
}

// Detect evaluates the snapshot and returns anomalous stations.
func (m *Monitor) Detect(ctx context.Context, snap sim.Snapshot) []Finding {
	var out []Finding
	names := map[string]string{}
	pts := make([]ml.AnomalyPoint, 0, len(snap.Stations))
	for _, st := range snap.Stations {
		if st.Closed {
			continue
		}
		names[st.ID] = st.Name
		pts = append(pts, ml.AnomalyPoint{StationID: st.ID, Timestamp: snap.SimTime, Observed: st.Density})
	}
	if res, err := m.ml.Anomalies(ctx, pts); err == nil {
		for _, r := range res {
			if r.Anomaly && r.ZScore > 0 { // only over-crowding is actionable
				out = append(out, Finding{r.StationID, names[r.StationID], r.Expected, r.Observed, r.ZScore, "ml"})
			}
		}
		return out
	}
	for _, st := range snap.Stations {
		if st.Closed {
			continue
		}
		z := (st.Density - st.Expected) / FallbackSigma
		if z >= ZThreshold {
			out = append(out, Finding{st.ID, st.Name, st.Expected, st.Density, round2(z), "fallback"})
		}
	}
	return out
}

// ToIncidents creates incidents for findings not already raised recently.
func (m *Monitor) ToIncidents(findings []Finding, now time.Time, newID func() string) []store.Incident {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []store.Incident
	for _, f := range findings {
		if t, ok := m.lastOpen[f.StationID]; ok && now.Sub(t) < m.Cooldown {
			continue
		}
		m.lastOpen[f.StationID] = now
		out = append(out, store.Incident{
			ID: newID(), Type: "CROWD_ANOMALY", Severity: Severity(f.Z, f.Observed), StationID: f.StationID,
			Title: fmt.Sprintf("Crowd anomaly at %s", f.Name),
			Detail: fmt.Sprintf("Expected %.0f%%, observed %.0f%% (z=%.1f, %s). Possible causes (unverified): train delay, platform blockage, event traffic, service disruption.",
				f.Expected, f.Observed, f.Z, f.Method),
			Source: "anomaly_detector", Status: "OPEN", CreatedAt: now, UpdatedAt: now,
		})
	}
	return out
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
