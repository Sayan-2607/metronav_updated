package sim

import (
	"os"
	"testing"
	"time"

	"github.com/metronav/core/internal/network"
)

func newSim(t *testing.T) *Simulator {
	t.Helper()
	b, err := os.ReadFile("../../../../data/network.json")
	if err != nil {
		t.Fatal(err)
	}
	n, err := network.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	return New(n, time.UTC, 1, 42)
}

func TestSnapshotInvariants(t *testing.T) {
	s := newSim(t)
	snap := s.Tick()
	if len(snap.Stations) != 40 {
		t.Fatalf("stations = %d", len(snap.Stations))
	}
	if len(snap.Trains) == 0 {
		t.Fatal("no trains generated")
	}
	for _, tr := range snap.Trains {
		if tr.PrevStation == "" || tr.NextStation == "" {
			t.Fatalf("train %s not placed: %+v", tr.ID, tr)
		}
		if tr.Occupancy < 0 || tr.Occupancy > 100 || tr.ETANextMin < 0 {
			t.Fatalf("train %s out of range: %+v", tr.ID, tr)
		}
		for _, c := range tr.Coaches {
			if c < 0 || c > 100 {
				t.Fatalf("coach occupancy out of range: %v", tr.Coaches)
			}
		}
	}
	for _, st := range snap.Stations {
		if st.Density < 0 || st.Density > 100 {
			t.Fatalf("density out of range: %+v", st)
		}
	}
}

func TestArrivalsSorted(t *testing.T) {
	s := newSim(t)
	arr := s.Arrivals("ESPLANADE", "BLUE", 1, 5)
	if len(arr) != 5 {
		t.Fatalf("got %d arrivals", len(arr))
	}
	for i := 1; i < len(arr); i++ {
		if arr[i].ETAMin < arr[i-1].ETAMin {
			t.Fatal("arrivals not sorted by ETA")
		}
	}
	// with 6-minute headway the next train should arrive within ~one headway
	if arr[0].ETAMin > 6.5 {
		t.Fatalf("first ETA %.1f exceeds headway", arr[0].ETAMin)
	}
}

func TestScenarios(t *testing.T) {
	s := newSim(t)
	if _, err := s.AddScenario(Scenario{Type: "station_closure", StationID: "ESPLANADE"}, 10); err != nil {
		t.Fatal(err)
	}
	if !s.Closed()["ESPLANADE"] {
		t.Fatal("closure not applied")
	}
	s.Tick()
	if d := s.Density("ESPLANADE"); d > 5 {
		t.Fatalf("closed station density = %.1f", d)
	}
	if _, err := s.AddScenario(Scenario{Type: "crowd_surge", StationID: "NOPE", SurgePct: 50}, 10); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestObservationOverrides(t *testing.T) {
	s := newSim(t)
	if err := s.Observe("HOWRAH", 77); err != nil {
		t.Fatal(err)
	}
	snap := s.Tick()
	for _, st := range snap.Stations {
		if st.ID == "HOWRAH" && (st.Source != "cv" || st.Density != 77) {
			t.Fatalf("observation not applied: %+v", st)
		}
	}
}
