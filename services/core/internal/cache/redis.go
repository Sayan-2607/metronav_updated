// Package cache mirrors live state into Redis (keys + a capped event stream)
// so other processes (e.g. Python workers, extra gateway replicas) can consume
// it. Redis is optional: if unreachable, the core runs from in-process state.
package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const StreamKey = "metronav:events"

type Redis struct{ c *redis.Client }

func Open(ctx context.Context, addr string) (*Redis, error) {
	c := redis.NewClient(&redis.Options{Addr: addr, DialTimeout: 2 * time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second})
	var err error
	for i := 0; i < 10; i++ {
		cx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err = c.Ping(cx).Err()
		cancel()
		if err == nil {
			return &Redis{c: c}, nil
		}
		time.Sleep(time.Second)
	}
	c.Close()
	return nil, fmt.Errorf("redis unreachable: %w", err)
}

func (r *Redis) Ping(ctx context.Context) error { return r.c.Ping(ctx).Err() }

// PutState writes a batch of key->value pairs with a TTL in one pipeline.
func (r *Redis) PutState(ctx context.Context, kv map[string]string, ttl time.Duration) error {
	p := r.c.Pipeline()
	for k, v := range kv {
		p.Set(ctx, k, v, ttl)
	}
	_, err := p.Exec(ctx)
	return err
}

// Publish appends an event to the capped stream (approx. last 10k events).
func (r *Redis) Publish(ctx context.Context, eventType string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return r.c.XAdd(ctx, &redis.XAddArgs{
		Stream: StreamKey, MaxLen: 10000, Approx: true,
		Values: map[string]any{"type": eventType, "payload": string(b)},
	}).Err()
}

// Allow implements a fixed-window rate limit shared across replicas.
func (r *Redis) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	k := fmt.Sprintf("rl:%s:%d", key, time.Now().Unix()/int64(window.Seconds()))
	n, err := r.c.Incr(ctx, k).Result()
	if err != nil {
		return true, err
	}
	if n == 1 {
		r.c.Expire(ctx, k, window)
	}
	return n <= int64(limit), nil
}
