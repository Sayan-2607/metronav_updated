package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/metronav/core/internal/auth"
	"github.com/metronav/core/internal/ml"
	"github.com/metronav/core/internal/realtime"
	"github.com/metronav/core/internal/route"
	"github.com/metronav/core/internal/sim"
	"github.com/metronav/core/internal/store"
)

const (
	RolePassenger = "PASSENGER"
	RoleOperator  = "OPERATOR"
	RoleAnalyst   = "ANALYST"
	RoleAdmin     = "ADMIN"
)

func (a *App) Handler() http.Handler {
	m := http.NewServeMux()
	ops := []string{RoleOperator, RoleAdmin}
	ana := []string{RoleOperator, RoleAnalyst, RoleAdmin}

	m.HandleFunc("GET /health", a.health)
	m.HandleFunc("GET /ready", a.ready)
	m.HandleFunc("GET /metrics", a.metrics)

	m.HandleFunc("GET /api/v1/network", a.getNetwork)
	m.HandleFunc("GET /api/v1/snapshot", a.getSnapshot)
	m.HandleFunc("GET /api/v1/stations", a.listStations)
	m.HandleFunc("GET /api/v1/stations/{id}", a.getStation)
	m.HandleFunc("GET /api/v1/stations/{id}/arrivals", a.getArrivals)
	m.HandleFunc("GET /api/v1/trains", a.listTrains)
	m.HandleFunc("GET /api/v1/trains/{id}", a.getTrain)
	m.HandleFunc("GET /api/v1/routes/search", a.searchRoutes)
	m.HandleFunc("GET /api/v1/crowd/stations/{id}", a.stationCrowd)
	m.HandleFunc("GET /api/v1/predictions/crowd", a.predictCrowd)
	m.HandleFunc("GET /api/v1/realtime", a.realtime)

	m.HandleFunc("POST /api/v1/auth/register", a.register)
	m.HandleFunc("POST /api/v1/auth/login", a.login)
	m.HandleFunc("GET /api/v1/auth/me", a.RequireRole(a.me))

	m.HandleFunc("POST /api/v1/tickets", a.RequireRole(a.createTicket))
	m.HandleFunc("GET /api/v1/tickets", a.RequireRole(a.myTickets))
	m.HandleFunc("GET /api/v1/tickets/{id}", a.RequireRole(a.getTicket))
	m.HandleFunc("POST /api/v1/tickets/validate", a.RequireRole(a.validateTicket, ops...))

	m.HandleFunc("GET /api/v1/incidents", a.RequireRole(a.listIncidents, ana...))
	m.HandleFunc("POST /api/v1/incidents", a.RequireRole(a.createIncident, ops...))
	m.HandleFunc("PATCH /api/v1/incidents/{id}", a.RequireRole(a.updateIncident, ops...))
	m.HandleFunc("GET /api/v1/admin/overview", a.RequireRole(a.overview, ana...))
	m.HandleFunc("GET /api/v1/ml/models", a.RequireRole(a.models, ana...))

	m.HandleFunc("POST /api/v1/simulation/scenarios", a.RequireRole(a.addScenario, ops...))
	m.HandleFunc("DELETE /api/v1/simulation/scenarios/{id}", a.RequireRole(a.clearScenario, ops...))
	m.HandleFunc("POST /api/v1/simulation/whatif", a.RequireRole(a.whatIf, ana...))

	m.HandleFunc("POST /api/v1/events/ingest", a.ingest)
	return a.Wrap(m)
}

// ---------- health ----------

func (a *App) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{"status": "ok", "uptime_s": int(time.Since(a.Started).Seconds())})
}

func (a *App) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	deps := map[string]string{"store": a.Store.Kind(), "redis": "disabled", "ml": "disabled"}
	status := 200
	if err := a.Store.Ping(ctx); err != nil {
		deps["store"] = "down"
		status = 503
	}
	if a.Redis != nil {
		deps["redis"] = "ok"
		if err := a.Redis.Ping(ctx); err != nil {
			deps["redis"] = "down (degraded)"
		}
	}
	if a.ML.Enabled() {
		deps["ml"] = "ok"
		if !a.ML.Healthy() {
			deps["ml"] = "breaker open (fallbacks active)"
		}
	}
	writeJSON(w, status, map[string]any{"ready": status == 200, "dependencies": deps})
}

func (a *App) metrics(w http.ResponseWriter, _ *http.Request) {
	a.Metrics.Set("metronav_ws_connections", nil, float64(a.Hub.Count()))
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	a.Metrics.Write(w)
}

// ---------- network / live state ----------

func (a *App) getNetwork(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{"note": a.Net.Note, "stations": a.Net.Stations, "lines": a.Net.Lines, "transfer_min": a.Net.TransferMin, "fare_slabs": a.Net.FareSlabs})
}

func (a *App) getSnapshot(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, a.Sim.Snapshot()) }

func (a *App) listStations(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, a.Sim.Snapshot().Stations)
}

func (a *App) findStation(id string) (sim.Station, bool) {
	for _, s := range a.Sim.Snapshot().Stations {
		if s.ID == id {
			return s, true
		}
	}
	return sim.Station{}, false
}

func (a *App) getStation(w http.ResponseWriter, r *http.Request) {
	st, ok := a.findStation(r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "station not found")
		return
	}
	type dirArr struct {
		Line     string        `json:"line"`
		Towards  string        `json:"towards"`
		Arrivals []sim.Arrival `json:"arrivals"`
	}
	var arr []dirArr
	for _, lid := range st.Lines {
		l, _ := a.Net.Line(lid)
		for _, d := range []int{1, -1} {
			idx := l.Index(st.ID)
			if (d == 1 && idx == len(l.Stations)-1) || (d == -1 && idx == 0) {
				continue
			}
			towards := l.Stations[len(l.Stations)-1]
			if d == -1 {
				towards = l.Stations[0]
			}
			arr = append(arr, dirArr{lid, towards, a.Sim.Arrivals(st.ID, lid, d, 3)})
		}
	}
	writeJSON(w, 200, map[string]any{"station": st, "departures": arr})
}

// getArrivals: ?line=BLUE&dest=KALIGHAT (direction inferred) and optional &limit
func (a *App) getArrivals(w http.ResponseWriter, r *http.Request) {
	sid, lid, dest := r.PathValue("id"), r.URL.Query().Get("line"), r.URL.Query().Get("dest")
	l, ok := a.Net.Line(lid)
	if !ok || l.Index(sid) < 0 || l.Index(dest) < 0 || sid == dest {
		writeErr(w, 400, "line must serve both station and dest, and they must differ")
		return
	}
	dir := 1
	if l.Index(dest) < l.Index(sid) {
		dir = -1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 6 {
		limit = 3
	}
	type out struct {
		sim.Arrival
		Coach CoachRecommendation `json:"coach_recommendation"`
	}
	var res []out
	for _, ar := range a.Sim.Arrivals(sid, lid, dir, limit) {
		res = append(res, out{ar, RecommendCoach(ar.Train.Coaches, dest)})
	}
	writeJSON(w, 200, map[string]any{"station": sid, "line": lid, "dest": dest, "direction": dir, "arrivals": res,
		"note": "Coach occupancy is the train's live (simulated) load now, not a forecast at boarding time."})
}

func (a *App) listTrains(w http.ResponseWriter, r *http.Request) {
	line := r.URL.Query().Get("line")
	var out []sim.Train
	for _, t := range a.Sim.Snapshot().Trains {
		if line == "" || t.Line == line {
			out = append(out, t)
		}
	}
	writeJSON(w, 200, out)
}

func (a *App) getTrain(w http.ResponseWriter, r *http.Request) {
	t, ok := a.Sim.Train(r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "train not found")
		return
	}
	resp := map[string]any{"train": t}
	if dest := r.URL.Query().Get("dest"); dest != "" {
		resp["coach_recommendation"] = RecommendCoach(t.Coaches, dest)
	}
	writeJSON(w, 200, resp)
}

// ---------- routing ----------

type routeOut struct {
	*route.Result
	ETA           ml.ETAPrediction `json:"eta_prediction"`
	NextDeparture *struct {
		sim.Arrival
		Coach CoachRecommendation `json:"coach_recommendation"`
	} `json:"next_departure,omitempty"`
}

func (a *App) searchRoutes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, to := q.Get("from"), q.Get("to")
	opt := route.Options{Crowd: a.Sim.Density, Closed: a.Sim.Closed()}
	results, err := a.Routes.PlanAll(from, to, opt)
	if err != nil {
		code := 422
		if errors.Is(err, route.ErrUnknownStation) {
			code = 404
		}
		writeErr(w, code, err.Error())
		return
	}
	snap := a.Sim.Snapshot()
	delayed := map[string]bool{}
	for _, t := range snap.Trains {
		if t.Delayed {
			delayed[t.Line] = true
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 1500*time.Millisecond)
	defer cancel()
	out := make([]routeOut, 0, len(results))
	for _, res := range results {
		ro := routeOut{Result: res}
		lineDelayed := false
		for _, l := range res.Legs {
			lineDelayed = lineDelayed || delayed[l.Line]
		}
		req := ml.ETARequest{ScheduledMin: res.TotalMin, Stops: res.StationsCount, Transfers: res.Transfers,
			MeanCrowd: res.CrowdScore, Timestamp: snap.SimTime, LineDelayed: lineDelayed}
		key := fmt.Sprintf("%s|%.0f|%t", route.Signature(res), res.CrowdScore/5, lineDelayed)
		if p, err := a.etaCache.load(key, func() (ml.ETAPrediction, error) { return a.ML.ETA(ctx, req) }); err == nil {
			ro.ETA = p
		} else {
			ro.ETA = FallbackETA(req) // not cached, so the model is retried once it recovers
		}
		first := res.Legs[0]
		if l, ok := a.Net.Line(first.Line); ok {
			dir := 1
			if l.Index(first.To) < l.Index(first.From) {
				dir = -1
			}
			if arr := a.Sim.Arrivals(first.From, first.Line, dir, 1); len(arr) > 0 {
				ro.NextDeparture = &struct {
					sim.Arrival
					Coach CoachRecommendation `json:"coach_recommendation"`
				}{arr[0], RecommendCoach(arr[0].Train.Coaches, first.To)}
			}
		}
		out = append(out, ro)
	}
	writeJSON(w, 200, map[string]any{"from": from, "to": to, "sim_time": snap.SimTime, "routes": out})
}

// ---------- crowd & predictions ----------

func (a *App) stationCrowd(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	st, ok := a.findStation(id)
	if !ok {
		writeErr(w, 404, "station not found")
		return
	}
	since := a.Sim.Now().Add(-6 * time.Hour)
	hist, err := a.Store.OccupancyHistory(r.Context(), id, since)
	if err != nil {
		writeErr(w, 500, "history unavailable")
		return
	}
	writeJSON(w, 200, map[string]any{"station": st, "history": hist})
}

func (a *App) forecast(ctx context.Context, stationID string) (ml.CrowdForecast, error) {
	st, ok := a.Net.Station(stationID)
	if !ok {
		return ml.CrowdForecast{}, route.ErrUnknownStation
	}
	now := a.Sim.Now()
	cur := a.Sim.Density(stationID)
	req := ml.CrowdRequest{StationID: stationID, Timestamp: now, Current: cur,
		Lag15: a.densityAgo(stationID, now, 15*time.Minute, cur), Lag30: a.densityAgo(stationID, now, 30*time.Minute, cur)}
	if f, err := a.crowdCache.load(stationID, func() (ml.CrowdForecast, error) { return a.ML.Crowd(ctx, req) }); err == nil {
		return f, nil
	}
	return FallbackCrowd(st, now, cur), nil
}

func (a *App) predictCrowd(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 1500*time.Millisecond)
	defer cancel()
	f, err := a.forecast(ctx, r.URL.Query().Get("station"))
	if err != nil {
		writeErr(w, 404, "unknown station")
		return
	}
	writeJSON(w, 200, map[string]any{"station": f.StationID, "current": a.Sim.Density(f.StationID), "sim_time": a.Sim.Now(),
		"forecast_15m": f.Forecast15, "forecast_30m": f.Forecast30, "forecast_60m": f.Forecast60, "model": f.Model, "source": f.Source})
}

func (a *App) realtime(w http.ResponseWriter, r *http.Request) {
	snap := a.Sim.Snapshot()
	a.Hub.Serve(w, r,
		a.Hub.NewEvent(realtime.TrainPositions, "snapshot", map[string]any{"sim_time": snap.SimTime, "trains": snap.Trains}),
		a.Hub.NewEvent(realtime.CrowdUpdated, "snapshot", map[string]any{"sim_time": snap.SimTime, "stations": snap.Stations, "scenarios": snap.Scenarios}))
}

// ---------- auth ----------

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name,omitempty"`
}

func (a *App) register(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if err := readJSON(r, &c); err != nil {
		writeErr(w, 400, "invalid JSON body")
		return
	}
	c.Email = strings.ToLower(strings.TrimSpace(c.Email))
	if _, err := mail.ParseAddress(c.Email); err != nil || len(c.Password) < 8 || len(c.Password) > 128 {
		writeErr(w, 400, "valid email and a password of 8-128 characters are required")
		return
	}
	hash, err := auth.HashPassword(c.Password)
	if err != nil {
		writeErr(w, 500, "hashing failed")
		return
	}
	name := strings.TrimSpace(c.Name)
	if name == "" {
		name = strings.Split(c.Email, "@")[0]
	}
	u := &store.User{ID: NewID("USR"), Email: c.Email, Name: name, PasswordHash: hash, Role: RolePassenger, CreatedAt: time.Now()}
	if err := a.Store.CreateUser(r.Context(), u); err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeErr(w, 409, "email already registered")
			return
		}
		writeErr(w, 500, "could not create user")
		return
	}
	a.issue(w, u, 201)
}

func (a *App) login(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if err := readJSON(r, &c); err != nil {
		writeErr(w, 400, "invalid JSON body")
		return
	}
	u, err := a.Store.UserByEmail(r.Context(), strings.ToLower(strings.TrimSpace(c.Email)))
	if err != nil || !auth.CheckPassword(c.Password, u.PasswordHash) {
		writeErr(w, 401, "incorrect email or password")
		return
	}
	_ = a.Store.Audit(r.Context(), u.ID, "login", clientIP(r))
	a.issue(w, u, 200)
}

func (a *App) issue(w http.ResponseWriter, u *store.User, status int) {
	tok, err := auth.IssueJWT(a.Cfg.JWTSecret, auth.Claims{Sub: u.ID, Email: u.Email, Role: u.Role}, 12*time.Hour)
	if err != nil {
		writeErr(w, 500, "token error")
		return
	}
	writeJSON(w, status, map[string]any{"token": tok, "user": u})
}

func (a *App) me(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, claims(r)) }

// ---------- tickets ----------

func (a *App) createTicket(w http.ResponseWriter, r *http.Request) {
	var in struct {
		From    string `json:"from"`
		To      string `json:"to"`
		Profile string `json:"profile"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, "invalid JSON body")
		return
	}
	p := route.Profile(in.Profile)
	if p == "" {
		p = route.Fastest
	}
	valid := false
	for _, x := range route.AllProfiles {
		valid = valid || x == p
	}
	if !valid {
		writeErr(w, 400, "unknown profile")
		return
	}
	res, err := a.Routes.Plan(in.From, in.To, p, route.Options{Crowd: a.Sim.Density, Closed: a.Sim.Closed()})
	if err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	coach := 0
	first := res.Legs[0]
	if l, ok := a.Net.Line(first.Line); ok {
		dir := 1
		if l.Index(first.To) < l.Index(first.From) {
			dir = -1
		}
		if arr := a.Sim.Arrivals(first.From, first.Line, dir, 1); len(arr) > 0 {
			coach = RecommendCoach(arr[0].Train.Coaches, first.To).Recommended
		}
	}
	rj, _ := json.Marshal(res)
	now := time.Now()
	t := &store.Ticket{ID: NewID("TKT"), UserID: claims(r).Sub, FromStation: in.From, ToStation: in.To, Profile: string(p),
		Coach: coach, Fare: res.Fare, RouteJSON: string(rj), Status: "ACTIVE", CreatedAt: now, ExpiresAt: now.Add(3 * time.Hour)}
	t.Token = auth.SignTicket(a.Cfg.TicketSecret, t.ID, t.ExpiresAt)
	if err := a.Store.CreateTicket(r.Context(), t); err != nil {
		writeErr(w, 500, "could not store ticket")
		return
	}
	a.Metrics.Inc("metronav_tickets_total", nil)
	a.Hub.Broadcast(a.Hub.NewEvent(realtime.TicketBooked, "ticket_service", map[string]any{"from": t.FromStation, "to": t.ToStation, "fare": t.Fare}))
	writeJSON(w, 201, map[string]any{"ticket": t, "route": res,
		"payment_note": "Payment is not integrated; this prototype issues tickets without charging."})
}

func (a *App) myTickets(w http.ResponseWriter, r *http.Request) {
	ts, err := a.Store.TicketsByUser(r.Context(), claims(r).Sub)
	if err != nil {
		writeErr(w, 500, "could not load tickets")
		return
	}
	if ts == nil {
		ts = []store.Ticket{}
	}
	writeJSON(w, 200, ts)
}

func (a *App) getTicket(w http.ResponseWriter, r *http.Request) {
	t, err := a.Store.Ticket(r.Context(), r.PathValue("id"))
	c := claims(r)
	if err != nil || (t.UserID != c.Sub && c.Role == RolePassenger) {
		writeErr(w, 404, "ticket not found") // do not reveal others' tickets
		return
	}
	writeJSON(w, 200, t)
}

func (a *App) validateTicket(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"token"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, "invalid JSON body")
		return
	}
	id, exp, err := auth.VerifyTicket(a.Cfg.TicketSecret, in.Token)
	if err != nil {
		writeJSON(w, 200, map[string]any{"valid": false, "reason": "signature invalid"})
		return
	}
	if time.Now().After(exp) {
		writeJSON(w, 200, map[string]any{"valid": false, "reason": "expired", "ticket_id": id})
		return
	}
	switch err := a.Store.MarkTicketUsed(r.Context(), id, time.Now()); {
	case errors.Is(err, store.ErrAlreadyUsed):
		writeJSON(w, 200, map[string]any{"valid": false, "reason": "already used", "ticket_id": id})
	case errors.Is(err, store.ErrNotFound):
		writeJSON(w, 200, map[string]any{"valid": false, "reason": "unknown ticket", "ticket_id": id})
	case err != nil:
		writeErr(w, 500, "validation failed")
	default:
		_ = a.Store.Audit(r.Context(), claims(r).Sub, "ticket_validated", id)
		writeJSON(w, 200, map[string]any{"valid": true, "ticket_id": id})
	}
}

// ---------- incidents ----------

var severities = map[string]bool{"LOW": true, "MEDIUM": true, "HIGH": true, "CRITICAL": true}

func (a *App) listIncidents(w http.ResponseWriter, r *http.Request) {
	inc, err := a.Store.Incidents(r.Context(), r.URL.Query().Get("status"), 100)
	if err != nil {
		writeErr(w, 500, "could not load incidents")
		return
	}
	if inc == nil {
		inc = []store.Incident{}
	}
	writeJSON(w, 200, inc)
}

func (a *App) createIncident(w http.ResponseWriter, r *http.Request) {
	var in store.Incident
	if err := readJSON(r, &in); err != nil || in.Title == "" || in.Type == "" || !severities[in.Severity] {
		writeErr(w, 400, "type, title and severity (LOW|MEDIUM|HIGH|CRITICAL) are required")
		return
	}
	now := time.Now()
	in.ID, in.Source, in.Status, in.CreatedAt, in.UpdatedAt = NewID("INC"), "operator", "OPEN", now, now
	if err := a.Store.CreateIncident(r.Context(), &in); err != nil {
		writeErr(w, 500, "could not create incident")
		return
	}
	_ = a.Store.Audit(r.Context(), claims(r).Sub, "incident_created", in.ID)
	a.Hub.Broadcast(a.Hub.NewEvent(realtime.IncidentCreated, "operator", in))
	writeJSON(w, 201, in)
}

func (a *App) updateIncident(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Status string `json:"status"`
	}
	if err := readJSON(r, &in); err != nil || (in.Status != "ACKNOWLEDGED" && in.Status != "RESOLVED" && in.Status != "OPEN") {
		writeErr(w, 400, "status must be OPEN, ACKNOWLEDGED or RESOLVED")
		return
	}
	if err := a.Store.SetIncidentStatus(r.Context(), r.PathValue("id"), in.Status); err != nil {
		writeErr(w, 404, "incident not found")
		return
	}
	_ = a.Store.Audit(r.Context(), claims(r).Sub, "incident_"+strings.ToLower(in.Status), r.PathValue("id"))
	writeJSON(w, 200, map[string]string{"id": r.PathValue("id"), "status": in.Status})
}

// ---------- admin overview ----------

func (a *App) overview(w http.ResponseWriter, r *http.Request) {
	snap := a.Sim.Snapshot()
	var occ, stationAvg float64
	delayedTrains, alerts := 0, 0
	for _, t := range snap.Trains {
		occ += t.Occupancy
		if t.Delayed {
			delayedTrains++
		}
	}
	for _, s := range snap.Stations {
		stationAvg += s.Density
		if s.Level == "high" || s.Level == "severe" {
			alerts++
		}
	}
	if n := float64(len(snap.Trains)); n > 0 {
		occ /= n
	}
	if n := float64(len(snap.Stations)); n > 0 {
		stationAvg /= n
	}
	midnight := time.Now().Truncate(24 * time.Hour)
	tickets, _ := a.Store.CountTicketsSince(r.Context(), midnight)
	open, _ := a.Store.Incidents(r.Context(), "OPEN", 100)
	writeJSON(w, 200, map[string]any{
		"sim_time": snap.SimTime, "active_trains": len(snap.Trains), "stations": len(snap.Stations),
		"avg_train_occupancy": round1(occ), "avg_station_density": round1(stationAvg),
		"crowded_stations": alerts, "delayed_trains": delayedTrains, "open_incidents": len(open),
		"tickets_today_utc": tickets, "ws_connections": a.Hub.Count(), "store": a.Store.Kind(),
		"redis": a.Redis != nil, "ml_healthy": a.ML.Healthy(), "scenarios": snap.Scenarios,
	})
}

func (a *App) models(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	m, err := a.ML.Models(ctx)
	if err != nil {
		writeErr(w, 503, "ML service unavailable; no model metrics to show")
		return
	}
	writeJSON(w, 200, m)
}

// ---------- simulation ----------

func (a *App) addScenario(w http.ResponseWriter, r *http.Request) {
	var in struct {
		sim.Scenario
		DurationMin float64 `json:"duration_min"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
		writeErr(w, 400, "invalid JSON body")
		return
	}
	sc, err := a.Sim.AddScenario(in.Scenario, in.DurationMin)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	_ = a.Store.Audit(r.Context(), claims(r).Sub, "scenario_added", sc.ID+" "+sc.Type)
	a.Hub.Broadcast(a.Hub.NewEvent(realtime.ScenarioChanged, "operator", sc))
	writeJSON(w, 201, sc)
}

func (a *App) clearScenario(w http.ResponseWriter, r *http.Request) {
	if !a.Sim.ClearScenario(r.PathValue("id")) {
		writeErr(w, 404, "scenario not found")
		return
	}
	a.Hub.Broadcast(a.Hub.NewEvent(realtime.ScenarioChanged, "operator", map[string]string{"cleared": r.PathValue("id")}))
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) whatIf(w http.ResponseWriter, r *http.Request) {
	var in sim.WhatIfInput
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, "invalid JSON body")
		return
	}
	res, err := a.Sim.WhatIf(in)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, res)
}

// ---------- sensor ingestion (CV pipeline) ----------

func (a *App) ingest(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Ingest-Key") != a.Cfg.IngestKey {
		writeErr(w, 401, "invalid ingest key")
		return
	}
	var ev struct {
		EventType string `json:"event_type"`
		Source    string `json:"source"`
		StationID string `json:"station_id"`
		Payload   struct {
			PeopleCount *int     `json:"people_count"`
			Density     *float64 `json:"density"` // 0..1
			Zone        string   `json:"zone"`
		} `json:"payload"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&ev); err != nil || ev.EventType != "CROWD_UPDATE" {
		writeErr(w, 400, "expected a CROWD_UPDATE event")
		return
	}
	st, ok := a.Net.Station(ev.StationID)
	if !ok {
		writeErr(w, 404, "unknown station")
		return
	}
	var pct float64
	switch {
	case ev.Payload.Density != nil:
		pct = *ev.Payload.Density * 100
	case ev.Payload.PeopleCount != nil:
		pct = float64(*ev.Payload.PeopleCount) / float64(st.PlatformCapacity) * 100
	default:
		writeErr(w, 400, "payload needs density (0-1) or people_count")
		return
	}
	if err := a.Sim.Observe(st.ID, pct); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	a.Metrics.Inc("metronav_ingested_events_total", map[string]string{"source": ev.Source})
	writeJSON(w, 202, map[string]any{"accepted": true, "station_id": st.ID, "density_pct": round1(pct)})
}

func round1(v float64) float64 { return float64(int(v*10+0.5)) / 10 }
