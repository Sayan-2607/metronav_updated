// Command loadtest is a small closed-loop HTTP + WebSocket load generator.
//
//	go run ./cmd/loadtest -url http://localhost:8080 -c 50 -d 20s -ws 500
//
// It reports throughput, latency percentiles and errors for a mix of API
// calls, and how many WebSocket clients stayed connected and received events.
// Results depend entirely on the machine; record the hardware with them.
package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

var paths = []string{
	"/api/v1/routes/search?from=PARK_STREET&to=SALT_LAKE_SECTOR_V",
	"/api/v1/routes/search?from=HOWRAH&to=KAVI_SUBHASH",
	"/api/v1/stations/ESPLANADE",
	"/api/v1/snapshot",
	"/api/v1/predictions/crowd?station=SEALDAH",
}

func main() {
	base := flag.String("url", "http://localhost:8080", "core base URL")
	conc := flag.Int("c", 50, "concurrent HTTP workers")
	dur := flag.Duration("d", 20*time.Second, "test duration")
	nws := flag.Int("ws", 0, "WebSocket clients to hold open")
	flag.Parse()

	// WebSocket clients
	var wsUp, wsMsgs, wsFail atomic.Int64
	stop := make(chan struct{})
	var wg sync.WaitGroup
	if *nws > 0 {
		u, _ := url.Parse(*base)
		u.Scheme = strings.Replace(u.Scheme, "http", "ws", 1)
		u.Path = "/api/v1/realtime"
		for i := 0; i < *nws; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
				if err != nil {
					wsFail.Add(1)
					return
				}
				wsUp.Add(1)
				go func() { <-stop; c.Close() }()
				for {
					if _, _, err := c.ReadMessage(); err != nil {
						return
					}
					wsMsgs.Add(1)
				}
			}()
			if i%100 == 99 {
				time.Sleep(50 * time.Millisecond) // avoid a SYN burst
			}
		}
	}

	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{MaxIdleConnsPerHost: *conc * 2}}
	var mu sync.Mutex
	var lat []time.Duration
	var errs, non2xx atomic.Int64
	deadline := time.Now().Add(*dur)
	var hw sync.WaitGroup
	for w := 0; w < *conc; w++ {
		hw.Add(1)
		go func(w int) {
			defer hw.Done()
			local := make([]time.Duration, 0, 4096)
			for i := w; time.Now().Before(deadline); i++ {
				t := time.Now()
				resp, err := client.Get(*base + paths[i%len(paths)])
				if err != nil {
					errs.Add(1)
					continue
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if resp.StatusCode >= 300 {
					non2xx.Add(1)
				}
				local = append(local, time.Since(t))
			}
			mu.Lock()
			lat = append(lat, local...)
			mu.Unlock()
		}(w)
	}
	hw.Wait()
	up := wsUp.Load()
	close(stop)
	wg.Wait()

	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	pct := func(p float64) time.Duration {
		if len(lat) == 0 {
			return 0
		}
		return lat[int(float64(len(lat)-1)*p)]
	}
	fmt.Printf("machine: %s/%s, %d CPUs\n", runtime.GOOS, runtime.GOARCH, runtime.NumCPU())
	fmt.Printf("http: %d requests in %s, %d workers -> %.0f req/s\n", len(lat), *dur, *conc, float64(len(lat))/dur.Seconds())
	fmt.Printf("latency: p50 %s  p95 %s  p99 %s  max %s\n", pct(.5), pct(.95), pct(.99), pct(1))
	fmt.Printf("errors: transport %d, non-2xx %d\n", errs.Load(), non2xx.Load())
	if *nws > 0 {
		fmt.Printf("websocket: %d/%d connected (%d failed), %d messages received\n", up, *nws, wsFail.Load(), wsMsgs.Load())
	}
}
