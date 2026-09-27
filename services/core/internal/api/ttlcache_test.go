package api

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTTLCacheCoalescesAndCaches(t *testing.T) {
	c := newTTLCache[int](time.Minute)
	var calls atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := c.load("k", func() (int, error) { calls.Add(1); time.Sleep(20 * time.Millisecond); return 7, nil })
			if err != nil || v != 7 {
				t.Errorf("got %d %v", v, err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("expected 1 upstream call, got %d", calls.Load())
	}
	if _, err := c.load("k", func() (int, error) { t.Fatal("should be cached"); return 0, nil }); err != nil {
		t.Fatal(err)
	}
}

func TestTTLCacheDoesNotCacheErrors(t *testing.T) {
	c := newTTLCache[int](time.Minute)
	_, _ = c.load("k", func() (int, error) { return 0, errors.New("down") })
	v, err := c.load("k", func() (int, error) { return 3, nil })
	if err != nil || v != 3 {
		t.Fatal("error was cached")
	}
}

func TestRecommendCoach(t *testing.T) {
	r := RecommendCoach([]float64{90, 90, 10, 90}, "X")
	if r.Recommended != 3 {
		t.Fatalf("expected the near-empty coach, got %+v", r)
	}
}
