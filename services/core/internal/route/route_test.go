package route

import (
	"os"
	"testing"

	"github.com/metronav/core/internal/network"
)

func load(t *testing.T) *network.Network {
	t.Helper()
	b, err := os.ReadFile("../../../../data/network.json")
	if err != nil {
		t.Fatal(err)
	}
	n, err := network.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSameLineNoTransfer(t *testing.T) {
	e := New(load(t))
	r, err := e.Plan("PARK_STREET", "KALIGHAT", Fastest, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Transfers != 0 || len(r.Legs) != 1 || r.Legs[0].Line != "BLUE" {
		t.Fatalf("unexpected route %+v", r)
	}
	// Park Street -> Kalighat is 5 segments on the Blue line
	if r.StationsCount != 5 {
		t.Fatalf("stations travelled = %d, want 5", r.StationsCount)
	}
}

func TestInterchangeAtEsplanade(t *testing.T) {
	e := New(load(t))
	r, err := e.Plan("PARK_STREET", "SALT_LAKE_SECTOR_V", Fastest, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Transfers != 1 || r.Legs[0].To != "ESPLANADE" || r.Legs[1].Line != "GREEN" {
		t.Fatalf("expected Blue->Green at Esplanade, got %+v", r.Legs)
	}
	sum := r.WaitMin + r.RideMin + r.WalkTransferMin
	if d := sum - r.TotalMin; d > 0.2 || d < -0.2 {
		t.Fatalf("time components %.1f do not add to total %.1f", sum, r.TotalMin)
	}
}

func TestClosedInterchangeYieldsNoRoute(t *testing.T) {
	e := New(load(t))
	_, err := e.Plan("PARK_STREET", "SEALDAH", Fastest, Options{Closed: map[string]bool{"ESPLANADE": true}})
	if err != ErrNoRoute {
		t.Fatalf("want ErrNoRoute, got %v", err)
	}
}

func TestCrowdIncreasesLeastCrowdedCost(t *testing.T) {
	e := New(load(t))
	calm, _ := e.Plan("CENTRAL", "KALIGHAT", LeastCrowded, Options{Crowd: func(string) float64 { return 10 }})
	busy, _ := e.Plan("CENTRAL", "KALIGHAT", LeastCrowded, Options{Crowd: func(string) float64 { return 95 }})
	if busy.Cost <= calm.Cost {
		t.Fatalf("crowding should raise cost: calm=%.2f busy=%.2f", calm.Cost, busy.Cost)
	}
}

func TestPlanAllDeduplicates(t *testing.T) {
	e := New(load(t))
	rs, err := e.PlanAll("DUM_DUM", "PARK_STREET", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 || len(rs[0].Profiles) != len(AllProfiles) {
		t.Fatalf("single-line trip should collapse to one itinerary, got %d", len(rs))
	}
}

func TestFareSlabs(t *testing.T) {
	n := load(t)
	if n.Fare(1) != 5 || n.Fare(6) != 15 || n.Fare(40) != 25 {
		t.Fatal("fare slab mismatch")
	}
}
