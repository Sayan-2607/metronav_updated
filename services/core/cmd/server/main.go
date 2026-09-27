// Command server runs the MetroNav core: routing, ticketing, simulation,
// realtime WebSockets, incidents and the API gateway, in one Go process.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // embed tz database so Asia/Kolkata works in scratch images

	"github.com/metronav/core/internal/api"
	"github.com/metronav/core/internal/auth"
	"github.com/metronav/core/internal/cache"
	"github.com/metronav/core/internal/config"
	"github.com/metronav/core/internal/crowd"
	"github.com/metronav/core/internal/metrics"
	"github.com/metronav/core/internal/ml"
	"github.com/metronav/core/internal/network"
	"github.com/metronav/core/internal/realtime"
	"github.com/metronav/core/internal/route"
	"github.com/metronav/core/internal/sim"
	"github.com/metronav/core/internal/store"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := config.Load()
	if cfg.JWTSecret == "dev-only-change-me" {
		log.Warn("JWT_SECRET is the development default; set it before any shared deployment")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	net, err := network.Load(cfg.NetworkFile)
	if err != nil {
		log.Error("load network", "err", err)
		os.Exit(1)
	}
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		log.Warn("timezone not found, using UTC", "tz", cfg.Timezone)
		loc = time.UTC
	}
	reg := metrics.New()

	// Persistence: PostgreSQL if configured and reachable, otherwise memory.
	var st store.Store = store.NewMemory()
	if cfg.DatabaseURL != "" {
		if pg, err := store.OpenPostgres(ctx, cfg.DatabaseURL); err != nil {
			log.Error("postgres unavailable; falling back to in-memory store (data will not persist)", "err", err)
		} else {
			st = pg
			if err := pg.SeedNetwork(ctx, lineRows(net), stationRows(net)); err != nil {
				log.Warn("seed network", "err", err)
			}
		}
	}
	var rds *cache.Redis
	if cfg.RedisAddr != "" {
		if r, err := cache.Open(ctx, cfg.RedisAddr); err != nil {
			log.Error("redis unavailable; continuing without state mirroring", "err", err)
		} else {
			rds = r
		}
	}
	mlc := ml.New(cfg.MLURL, cfg.MLTimeout, reg)
	if !mlc.Enabled() {
		log.Warn("ML_URL not set; heuristic fallbacks will be used for forecasts, ETA and anomalies")
	}

	app := &api.App{
		Cfg: cfg, Net: net, Routes: route.New(net), Sim: sim.New(net, loc, cfg.SimSpeed, time.Now().UnixNano()),
		Store: st, Redis: rds, ML: mlc, Monitor: crowd.NewMonitor(mlc), Metrics: reg, Log: log,
	}
	app.Hub = realtime.NewHub(func(o string) bool { return originAllowed(cfg.CORSOrigins, o) }, log)
	app.Init()
	seedStaff(ctx, app, log)

	go app.Run(ctx)
	srv := &http.Server{Addr: cfg.Addr, Handler: app.Handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 120 * time.Second}
	go func() {
		log.Info("metronav core listening", "addr", cfg.Addr, "store", st.Kind(), "redis", rds != nil, "ml", mlc.Enabled(), "sim_speed", cfg.SimSpeed)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server", "err", err)
			stop()
		}
	}()
	<-ctx.Done()
	sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(sctx)
	log.Info("shutdown complete")
}

func originAllowed(list, o string) bool {
	for _, x := range splitComma(list) {
		if x == "*" || x == o {
			return true
		}
	}
	return false
}

func splitComma(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ',' {
			out = append(out, cur)
			cur = ""
			continue
		}
		if r != ' ' {
			cur += string(r)
		}
	}
	return append(out, cur)
}

// seedStaff creates the admin and a demo operator on first start.
func seedStaff(ctx context.Context, a *api.App, log *slog.Logger) {
	for _, u := range []struct{ email, name, role string }{
		{a.Cfg.AdminEmail, "Administrator", api.RoleAdmin},
		{"operator@metronav.local", "Duty Operator", api.RoleOperator},
	} {
		if _, err := a.Store.UserByEmail(ctx, u.email); err == nil {
			continue
		}
		h, err := auth.HashPassword(a.Cfg.AdminPassword)
		if err != nil {
			continue
		}
		err = a.Store.CreateUser(ctx, &store.User{ID: api.NewID("USR"), Email: u.email, Name: u.name, PasswordHash: h, Role: u.role, CreatedAt: time.Now()})
		if err == nil {
			log.Info("seeded staff account", "email", u.email, "role", u.role)
		}
	}
}

func lineRows(n *network.Network) []store.LineRow {
	var out []store.LineRow
	for _, l := range n.Lines {
		out = append(out, store.LineRow{ID: l.ID, Name: l.Name, Color: l.Color, Coaches: l.Coaches, HeadwayMin: l.HeadwayMin, Stations: l.Stations})
	}
	return out
}

func stationRows(n *network.Network) []store.StationRow {
	var out []store.StationRow
	for _, s := range n.Stations {
		out = append(out, store.StationRow{ID: s.ID, Name: s.Name, Weight: s.Weight, Capacity: s.PlatformCapacity})
	}
	return out
}
