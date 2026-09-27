package crowd

import (
	"context"
	"testing"
	"time"

	"github.com/metronav/core/internal/metrics"
	"github.com/metronav/core/internal/ml"
	"github.com/metronav/core/internal/sim"
)

func TestFallbackDetectionAndCooldown(t *testing.T) {
	m := NewMonitor(ml.New("", time.Second, metrics.New())) // ML disabled => fallback
	snap := sim.Snapshot{SimTime: time.Now(), Stations: []sim.Station{
		{ID: "A", Name: "A", Expected: 50, Density: 52},
		{ID: "B", Name: "B", Expected: 50, Density: 80},
		{ID: "C", Name: "C", Expected: 50, Density: 10, Closed: true},
	}}
	f := m.Detect(context.Background(), snap)
	if len(f) != 1 || f[0].StationID != "B" || f[0].Method != "fallback" {
		t.Fatalf("unexpected findings %+v", f)
	}
	n := 0
	id := func() string { n++; return "I" }
	if inc := m.ToIncidents(f, time.Now(), id); len(inc) != 1 || inc[0].Severity != "CRITICAL" {
		t.Fatalf("incident not created correctly: %+v", inc)
	}
	if inc := m.ToIncidents(f, time.Now(), id); len(inc) != 0 {
		t.Fatal("cooldown not respected")
	}
}
