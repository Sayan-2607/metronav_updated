package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/metronav/core/internal/auth"
)

type ctxKey int

const claimsKey ctxKey = 1

// ---- token-bucket rate limiter (per client IP, in-process) ----

type bucket struct {
	tokens float64
	last   time.Time
}

type limiter struct {
	mu      sync.Mutex
	rate    float64
	burst   float64
	buckets map[string]*bucket
}

func newLimiter(rate, burst float64) *limiter {
	return &limiter{rate: rate, burst: burst, buckets: map[string]*bucket{}}
}

func (l *limiter) allow(key string) bool {
	if l.rate <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	b, ok := l.buckets[key]
	if !ok {
		if len(l.buckets) > 50_000 { // crude memory bound
			l.buckets = map[string]*bucket{}
		}
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	b.tokens = minf(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.rate)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(s int) { w.status = s; w.ResponseWriter.WriteHeader(s) }

// Hijack is required for WebSocket upgrades through the wrapper.
func (w *statusWriter) Hijack() (net.Conn, *bufioReadWriter, error) { return hijack(w.ResponseWriter) }

func (a *App) allowedOrigin(o string) bool {
	for _, x := range strings.Split(a.Cfg.CORSOrigins, ",") {
		x = strings.TrimSpace(x)
		if x == "*" || x == o {
			return true
		}
	}
	return false
}

// Wrap applies recover, CORS, rate limiting, logging and metrics.
func (a *App) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		defer func() {
			if rec := recover(); rec != nil {
				a.Log.Error("panic", "err", rec, "stack", string(debug.Stack()))
				writeErr(sw, http.StatusInternalServerError, "internal error")
			}
			pattern := routeLabel(r.URL.Path)
			a.Metrics.Inc("metronav_http_requests_total", map[string]string{"route": pattern, "code": strconv.Itoa(sw.status)})
			a.Metrics.Observe("metronav_http_request_seconds", map[string]string{"route": pattern}, time.Since(start).Seconds())
			if r.URL.Path != "/metrics" && r.URL.Path != "/health" {
				a.Log.Info("http", "method", r.Method, "path", r.URL.Path, "status", sw.status, "ms", time.Since(start).Milliseconds())
			}
		}()
		if o := r.Header.Get("Origin"); o != "" && a.allowedOrigin(o) {
			h := sw.Header()
			h.Set("Access-Control-Allow-Origin", o)
			h.Set("Vary", "Origin")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Ingest-Key")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			sw.WriteHeader(http.StatusNoContent)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/v1/realtime" && !a.limiter.allow(clientIP(r)) {
			sw.Header().Set("Retry-After", "1")
			writeErr(sw, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		sw.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(sw, r)
	})
}

// RequireRole enforces JWT auth; an empty roles list means "any signed-in user".
func (a *App) RequireRole(h http.HandlerFunc, roles ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if tok == "" {
			writeErr(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		c, err := auth.ParseJWT(a.Cfg.JWTSecret, tok)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, err.Error())
			return
		}
		if len(roles) > 0 {
			ok := false
			for _, role := range roles {
				if c.Role == role {
					ok = true
				}
			}
			if !ok {
				writeErr(w, http.StatusForbidden, "insufficient role")
				return
			}
		}
		h(w, r.WithContext(context.WithValue(r.Context(), claimsKey, c)))
	}
}

func claims(r *http.Request) *auth.Claims {
	c, _ := r.Context().Value(claimsKey).(*auth.Claims)
	return c
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// routeLabel keeps metric cardinality bounded: /api/v1/stations/X -> /api/v1/stations
func routeLabel(p string) string {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	if len(parts) > 3 {
		parts = parts[:3]
	}
	return "/" + strings.Join(parts, "/")
}
