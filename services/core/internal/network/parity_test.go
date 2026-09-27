package network

import (
	"fmt"
	"testing"
	"time"
)

// TestParityValues pins values that ai/tests/test_ai.py also asserts, so the
// Go and Python demand models cannot silently drift apart.
func TestParityValues(t *testing.T) {
	if got := ExitCoach("ESPLANADE", 8); got != parityExitEsplanade8 {
		t.Fatalf("ExitCoach = %d", got)
	}
	if got := fmt.Sprintf("%.4f", BaseOccupancy(1.0, 18.5, time.Monday)); got != parityBase1185Mon {
		t.Fatalf("BaseOccupancy = %s", got)
	}
	if FNV32("foobar") != 0xBF9CF968 {
		t.Fatal("FNV32 mismatch with standard vector")
	}
}

const (
	parityExitEsplanade8 = 2
	parityBase1185Mon    = "99.2935"
)
