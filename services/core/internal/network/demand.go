package network

import (
	"hash/fnv"
	"math"
	"time"
)

// TimeOfDayFactor is a synthetic bimodal weekday demand curve (0..1).
// It is NOT fitted to real ridership; it is a documented modelling assumption.
func TimeOfDayFactor(hour float64) float64 {
	if hour < 6.0 || hour > 22.5 {
		return 0.02 // outside service hours
	}
	g := func(mu, sigma, amp float64) float64 {
		return amp * math.Exp(-((hour-mu)*(hour-mu))/(2*sigma*sigma))
	}
	v := 0.15 + g(9.5, 1.2, 0.85) + g(18.5, 1.5, 0.80) + g(13.5, 2.0, 0.25)
	return math.Min(v, 1.0)
}

// WeekdayFactor scales demand for weekends.
func WeekdayFactor(wd time.Weekday) float64 {
	switch wd {
	case time.Saturday:
		return 0.75
	case time.Sunday:
		return 0.55
	}
	return 1.0
}

// BaseOccupancy returns expected platform occupancy (% of capacity).
func BaseOccupancy(weight, hour float64, wd time.Weekday) float64 {
	v := 100 * (0.08 + 0.95*TimeOfDayFactor(hour)*WeekdayFactor(wd)*weight)
	return Clamp(v, 0, 100)
}

// FNV32 is 32-bit FNV-1a (mirrored in Python).
func FNV32(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}

// ExitCoach is the (synthetic) coach index nearest a station's main stairway.
func ExitCoach(stationID string, coaches int) int {
	return int(FNV32(stationID) % uint32(coaches))
}

// CoachBias gives per-coach load multipliers with mean 1.0: coaches that stop
// near the boarding station's main stairway load more heavily.
func CoachBias(stationID string, coaches int) []float64 {
	e := ExitCoach(stationID, coaches)
	out := make([]float64, coaches)
	denom := float64(coaches - 1)
	if denom < 1 {
		denom = 1
	}
	sum := 0.0
	for c := 0; c < coaches; c++ {
		d := math.Abs(float64(c-e)) / denom
		out[c] = 1 + 0.35*(1-d)
		sum += out[c]
	}
	mean := sum / float64(coaches)
	for c := range out {
		out[c] /= mean
	}
	return out
}

func Clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

// HourOf converts a time to fractional hour of day.
func HourOf(t time.Time) float64 {
	return float64(t.Hour()) + float64(t.Minute())/60 + float64(t.Second())/3600
}
