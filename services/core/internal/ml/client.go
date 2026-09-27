// Package ml calls the Python AI service with strict timeouts and a simple
// circuit breaker. Every call has a deterministic fallback so the platform
// keeps working (in degraded mode) when the ML service is slow or down.
package ml

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/metronav/core/internal/metrics"
)

var ErrUnavailable = errors.New("ml service unavailable")

type Client struct {
	base    string
	http    *http.Client
	metrics *metrics.Registry

	mu        sync.Mutex
	failures  int
	openUntil time.Time
}

func New(base string, timeout time.Duration, m *metrics.Registry) *Client {
	return &Client{base: base, http: &http.Client{Timeout: timeout}, metrics: m}
}

func (c *Client) Enabled() bool { return c != nil && c.base != "" }

// Healthy reports whether the breaker is closed.
func (c *Client) Healthy() bool {
	if !c.Enabled() {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return time.Now().After(c.openUntil)
}

func (c *Client) post(ctx context.Context, path string, in, out any) error {
	if !c.Enabled() {
		return ErrUnavailable
	}
	c.mu.Lock()
	if time.Now().Before(c.openUntil) {
		c.mu.Unlock()
		c.metrics.Inc("metronav_ml_calls_total", map[string]string{"path": path, "result": "breaker_open"})
		return ErrUnavailable
	}
	c.mu.Unlock()

	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	start := time.Now()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			err = fmt.Errorf("ml %s: status %d", path, resp.StatusCode)
		} else {
			err = json.NewDecoder(resp.Body).Decode(out)
		}
	}
	c.metrics.Observe("metronav_ml_latency_seconds", map[string]string{"path": path}, time.Since(start).Seconds())
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		c.failures++
		if c.failures >= 3 {
			c.openUntil = time.Now().Add(15 * time.Second)
			c.failures = 0
		}
		c.metrics.Inc("metronav_ml_calls_total", map[string]string{"path": path, "result": "error"})
		return err
	}
	c.failures = 0
	c.metrics.Inc("metronav_ml_calls_total", map[string]string{"path": path, "result": "ok"})
	return nil
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	if !c.Enabled() {
		return ErrUnavailable
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ml %s: status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// ---- request/response types (mirror ai/metronav_ai/api.py) ----

type CrowdRequest struct {
	StationID string    `json:"station_id"`
	Timestamp time.Time `json:"timestamp"`
	Current   float64   `json:"current_occupancy"`
	Lag15     float64   `json:"occupancy_15m_ago"`
	Lag30     float64   `json:"occupancy_30m_ago"`
	Rain      bool      `json:"rain"`
	Event     bool      `json:"event_nearby"`
	Holiday   bool      `json:"holiday"`
}

type CrowdForecast struct {
	StationID  string  `json:"station_id"`
	Forecast15 float64 `json:"forecast_15m"`
	Forecast30 float64 `json:"forecast_30m"`
	Forecast60 float64 `json:"forecast_60m"`
	Model      string  `json:"model"`
	Source     string  `json:"source"` // ml | fallback
}

func (c *Client) Crowd(ctx context.Context, r CrowdRequest) (CrowdForecast, error) {
	var out CrowdForecast
	err := c.post(ctx, "/ml/crowd/predict", r, &out)
	out.Source = "ml"
	return out, err
}

type AnomalyPoint struct {
	StationID string    `json:"station_id"`
	Timestamp time.Time `json:"timestamp"`
	Observed  float64   `json:"observed"`
}

type AnomalyResult struct {
	StationID string  `json:"station_id"`
	Expected  float64 `json:"expected"`
	Observed  float64 `json:"observed"`
	ZScore    float64 `json:"z_score"`
	Anomaly   bool    `json:"is_anomaly"`
}

func (c *Client) Anomalies(ctx context.Context, pts []AnomalyPoint) ([]AnomalyResult, error) {
	var out struct {
		Results []AnomalyResult `json:"results"`
	}
	err := c.post(ctx, "/ml/anomaly/detect", map[string]any{"points": pts}, &out)
	return out.Results, err
}

type ETARequest struct {
	ScheduledMin float64   `json:"scheduled_min"`
	Stops        int       `json:"stops"`
	Transfers    int       `json:"transfers"`
	MeanCrowd    float64   `json:"mean_crowd"`
	Timestamp    time.Time `json:"timestamp"`
	LineDelayed  bool      `json:"line_delayed"`
}

type ETAPrediction struct {
	P10    float64 `json:"p10_min"`
	P50    float64 `json:"p50_min"`
	P90    float64 `json:"p90_min"`
	Model  string  `json:"model"`
	Source string  `json:"source"`
}

func (c *Client) ETA(ctx context.Context, r ETARequest) (ETAPrediction, error) {
	var out ETAPrediction
	err := c.post(ctx, "/ml/eta/predict", r, &out)
	out.Source = "ml"
	return out, err
}

func (c *Client) Models(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	return out, c.get(ctx, "/ml/models", &out)
}
