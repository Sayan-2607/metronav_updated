// Package metrics is a minimal Prometheus text-format exporter (counters,
// gauges and fixed-bucket histograms) built on the standard library.
package metrics

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"sync"
)

var buckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}

type hist struct {
	counts []uint64
	sum    float64
	n      uint64
}

type Registry struct {
	mu       sync.Mutex
	counters map[string]float64
	gauges   map[string]float64
	hists    map[string]*hist
	help     map[string]string
}

func New() *Registry {
	return &Registry{counters: map[string]float64{}, gauges: map[string]float64{}, hists: map[string]*hist{}, help: map[string]string{}}
}

// key builds `name{k="v",...}` with sorted labels.
func key(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}
	ks := make([]string, 0, len(labels))
	for k := range labels {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	parts := make([]string, len(ks))
	for i, k := range ks {
		parts[i] = fmt.Sprintf(`%s=%q`, k, labels[k])
	}
	return name + "{" + strings.Join(parts, ",") + "}"
}

func (r *Registry) Inc(name string, labels map[string]string) { r.Add(name, labels, 1) }
func (r *Registry) Add(name string, labels map[string]string, v float64) {
	r.mu.Lock()
	r.counters[key(name, labels)] += v
	r.mu.Unlock()
}
func (r *Registry) Set(name string, labels map[string]string, v float64) {
	r.mu.Lock()
	r.gauges[key(name, labels)] = v
	r.mu.Unlock()
}
func (r *Registry) Observe(name string, labels map[string]string, seconds float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := key(name, labels)
	h, ok := r.hists[k]
	if !ok {
		h = &hist{counts: make([]uint64, len(buckets))}
		r.hists[k] = h
	}
	for i, b := range buckets {
		if seconds <= b {
			h.counts[i]++
		}
	}
	h.sum += seconds
	h.n++
}

func splitKey(k string) (name, labels string) {
	if i := strings.IndexByte(k, '{'); i >= 0 {
		return k[:i], k[i+1 : len(k)-1]
	}
	return k, ""
}

func withLabel(labels, extra string) string {
	if labels == "" {
		return "{" + extra + "}"
	}
	return "{" + labels + "," + extra + "}"
}

func (r *Registry) Write(w io.Writer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	emit := func(m map[string]float64, typ string) {
		ks := make([]string, 0, len(m))
		for k := range m {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		typed := map[string]bool{}
		for _, k := range ks {
			n, _ := splitKey(k)
			if !typed[n] {
				fmt.Fprintf(w, "# TYPE %s %s\n", n, typ)
				typed[n] = true
			}
			fmt.Fprintf(w, "%s %g\n", k, m[k])
		}
	}
	emit(r.counters, "counter")
	emit(r.gauges, "gauge")
	ks := make([]string, 0, len(r.hists))
	for k := range r.hists {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	typed := map[string]bool{}
	for _, k := range ks {
		h := r.hists[k]
		n, l := splitKey(k)
		if !typed[n] {
			fmt.Fprintf(w, "# TYPE %s histogram\n", n)
			typed[n] = true
		}
		for i, b := range buckets {
			fmt.Fprintf(w, "%s_bucket%s %d\n", n, withLabel(l, fmt.Sprintf(`le="%g"`, b)), h.counts[i])
		}
		fmt.Fprintf(w, "%s_bucket%s %d\n", n, withLabel(l, `le="+Inf"`), h.n)
		fmt.Fprintf(w, "%s_sum%s %g\n", n, braces(l), h.sum)
		fmt.Fprintf(w, "%s_count%s %d\n", n, braces(l), h.n)
	}
	_ = math.Inf
}

func braces(l string) string {
	if l == "" {
		return ""
	}
	return "{" + l + "}"
}
