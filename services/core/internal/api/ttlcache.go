package api

import (
	"sync"
	"time"
)

// ttlCache memoises ML predictions for a short time and coalesces concurrent
// misses for the same key into one upstream call (singleflight), so a burst
// of identical requests cannot stampede the model service.
// Forecast inputs change slowly (the simulation ticks every few seconds and
// demand moves over minutes), so a 30-second TTL loses little freshness.
type ttlCache[V any] struct {
	mu       sync.Mutex
	ttl      time.Duration
	m        map[string]entry[V]
	inflight map[string]*call[V]
}

type entry[V any] struct {
	v   V
	exp time.Time
}

type call[V any] struct {
	done chan struct{}
	v    V
	err  error
}

func newTTLCache[V any](ttl time.Duration) *ttlCache[V] {
	return &ttlCache[V]{ttl: ttl, m: map[string]entry[V]{}, inflight: map[string]*call[V]{}}
}

// load returns a cached value or runs fn once per key across concurrent
// callers. Errors are returned to every waiter and are not cached.
func (c *ttlCache[V]) load(k string, fn func() (V, error)) (V, error) {
	c.mu.Lock()
	if e, ok := c.m[k]; ok && time.Now().Before(e.exp) {
		c.mu.Unlock()
		return e.v, nil
	}
	if cl, ok := c.inflight[k]; ok {
		c.mu.Unlock()
		<-cl.done
		return cl.v, cl.err
	}
	cl := &call[V]{done: make(chan struct{})}
	c.inflight[k] = cl
	c.mu.Unlock()

	cl.v, cl.err = fn()

	c.mu.Lock()
	delete(c.inflight, k)
	if cl.err == nil {
		if len(c.m) > 10_000 { // bound memory; entries are cheap to recompute
			c.m = map[string]entry[V]{}
		}
		c.m[k] = entry[V]{cl.v, time.Now().Add(c.ttl)}
	}
	c.mu.Unlock()
	close(cl.done)
	return cl.v, cl.err
}
