// Package store defines persistence for users, tickets, incidents,
// occupancy history and audit logs. PostgreSQL is the source of truth; the
// in-memory implementation is used for tests and as a degraded-mode fallback
// when DATABASE_URL is unset or unreachable at startup.
package store

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

var (
	ErrNotFound    = errors.New("not found")
	ErrConflict    = errors.New("already exists")
	ErrAlreadyUsed = errors.New("ticket already used")
)

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"` // PASSENGER | OPERATOR | ANALYST | ADMIN
	CreatedAt    time.Time `json:"created_at"`
}

type Ticket struct {
	ID          string     `json:"id"`
	UserID      string     `json:"user_id"`
	FromStation string     `json:"from_station"`
	ToStation   string     `json:"to_station"`
	Profile     string     `json:"profile"`
	Coach       int        `json:"coach"` // recommended coach (1-based), 0 = none
	Fare        int        `json:"fare"`
	RouteJSON   string     `json:"route_json"`
	Token       string     `json:"token"`
	Status      string     `json:"status"` // ACTIVE | USED
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   time.Time  `json:"expires_at"`
	UsedAt      *time.Time `json:"used_at,omitempty"`
}

type Incident struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Severity    string    `json:"severity"` // LOW | MEDIUM | HIGH | CRITICAL
	StationID   string    `json:"station_id,omitempty"`
	LineID      string    `json:"line_id,omitempty"`
	Title       string    `json:"title"`
	Detail      string    `json:"detail"`
	Source      string    `json:"source"` // anomaly_detector | operator | scenario
	Status      string    `json:"status"` // OPEN | ACKNOWLEDGED | RESOLVED
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type OccupancyRecord struct {
	StationID string    `json:"station_id"`
	At        time.Time `json:"at"`
	Density   float64   `json:"density"`
	Expected  float64   `json:"expected"`
	Source    string    `json:"source"`
}

type Store interface {
	Kind() string
	Ping(ctx context.Context) error

	CreateUser(ctx context.Context, u *User) error
	UserByEmail(ctx context.Context, email string) (*User, error)
	CountUsers(ctx context.Context) (int, error)

	CreateTicket(ctx context.Context, t *Ticket) error
	Ticket(ctx context.Context, id string) (*Ticket, error)
	TicketsByUser(ctx context.Context, userID string) ([]Ticket, error)
	MarkTicketUsed(ctx context.Context, id string, at time.Time) error
	CountTicketsSince(ctx context.Context, since time.Time) (int, error)

	CreateIncident(ctx context.Context, i *Incident) error
	Incidents(ctx context.Context, status string, limit int) ([]Incident, error)
	SetIncidentStatus(ctx context.Context, id, status string) error

	RecordOccupancy(ctx context.Context, recs []OccupancyRecord) error
	OccupancyHistory(ctx context.Context, stationID string, since time.Time) ([]OccupancyRecord, error)

	Audit(ctx context.Context, actor, action, detail string) error
}

// ---------------- memory ----------------

type Memory struct {
	mu        sync.RWMutex
	users     map[string]*User // by email
	tickets   map[string]*Ticket
	incidents []*Incident
	occ       []OccupancyRecord
	audit     []string
}

func NewMemory() *Memory {
	return &Memory{users: map[string]*User{}, tickets: map[string]*Ticket{}}
}

func (m *Memory) Kind() string                 { return "memory" }
func (m *Memory) Ping(context.Context) error   { return nil }

func (m *Memory) CreateUser(_ context.Context, u *User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[u.Email]; ok {
		return ErrConflict
	}
	c := *u
	m.users[u.Email] = &c
	return nil
}

func (m *Memory) UserByEmail(_ context.Context, email string) (*User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, ok := m.users[email]
	if !ok {
		return nil, ErrNotFound
	}
	c := *u
	return &c, nil
}

func (m *Memory) CountUsers(context.Context) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.users), nil
}

func (m *Memory) CreateTicket(_ context.Context, t *Ticket) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := *t
	m.tickets[t.ID] = &c
	return nil
}

func (m *Memory) Ticket(_ context.Context, id string) (*Ticket, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tickets[id]
	if !ok {
		return nil, ErrNotFound
	}
	c := *t
	return &c, nil
}

func (m *Memory) TicketsByUser(_ context.Context, userID string) ([]Ticket, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []Ticket
	for _, t := range m.tickets {
		if t.UserID == userID {
			out = append(out, *t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (m *Memory) MarkTicketUsed(_ context.Context, id string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tickets[id]
	if !ok {
		return ErrNotFound
	}
	if t.Status == "USED" {
		return ErrAlreadyUsed
	}
	t.Status, t.UsedAt = "USED", &at
	return nil
}

func (m *Memory) CountTicketsSince(_ context.Context, since time.Time) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n := 0
	for _, t := range m.tickets {
		if !t.CreatedAt.Before(since) {
			n++
		}
	}
	return n, nil
}

func (m *Memory) CreateIncident(_ context.Context, i *Incident) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := *i
	m.incidents = append(m.incidents, &c)
	return nil
}

func (m *Memory) Incidents(_ context.Context, status string, limit int) ([]Incident, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []Incident
	for k := len(m.incidents) - 1; k >= 0 && len(out) < limit; k-- {
		if status == "" || m.incidents[k].Status == status {
			out = append(out, *m.incidents[k])
		}
	}
	return out, nil
}

func (m *Memory) SetIncidentStatus(_ context.Context, id, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, i := range m.incidents {
		if i.ID == id {
			i.Status, i.UpdatedAt = status, time.Now()
			return nil
		}
	}
	return ErrNotFound
}

func (m *Memory) RecordOccupancy(_ context.Context, recs []OccupancyRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.occ = append(m.occ, recs...)
	if len(m.occ) > 200_000 { // bound memory
		m.occ = append([]OccupancyRecord(nil), m.occ[len(m.occ)-100_000:]...)
	}
	return nil
}

func (m *Memory) OccupancyHistory(_ context.Context, stationID string, since time.Time) ([]OccupancyRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []OccupancyRecord
	for _, r := range m.occ {
		if r.StationID == stationID && !r.At.Before(since) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (m *Memory) Audit(_ context.Context, actor, action, detail string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.audit = append(m.audit, time.Now().Format(time.RFC3339)+" "+actor+" "+action+" "+detail)
	return nil
}
