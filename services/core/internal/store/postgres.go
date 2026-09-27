package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

//go:embed schema.sql
var schemaSQL string

type Postgres struct{ db *sql.DB }

func OpenPostgres(ctx context.Context, url string) (*Postgres, error) {
	db, err := sql.Open("postgres", url)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	var lastErr error
	for i := 0; i < 10; i++ { // wait for container start-up
		c, cancel := context.WithTimeout(ctx, 2*time.Second)
		lastErr = db.PingContext(c)
		cancel()
		if lastErr == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if lastErr != nil {
		db.Close()
		return nil, fmt.Errorf("postgres unreachable: %w", lastErr)
	}
	if _, err := db.ExecContext(ctx, schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Postgres{db: db}, nil
}

func (p *Postgres) Kind() string                   { return "postgres" }
func (p *Postgres) Ping(ctx context.Context) error { return p.db.PingContext(ctx) }

func isUnique(err error) bool {
	var pe *pq.Error
	return errors.As(err, &pe) && pe.Code == "23505"
}

func (p *Postgres) CreateUser(ctx context.Context, u *User) error {
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO users (id,email,name,password_hash,role,created_at) VALUES ($1,$2,$3,$4,$5,$6)`,
		u.ID, u.Email, u.Name, u.PasswordHash, u.Role, u.CreatedAt)
	if isUnique(err) {
		return ErrConflict
	}
	return err
}

func (p *Postgres) UserByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	err := p.db.QueryRowContext(ctx,
		`SELECT id,email,name,password_hash,role,created_at FROM users WHERE email=$1`, email).
		Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &u, err
}

func (p *Postgres) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := p.db.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&n)
	return n, err
}

const ticketCols = `id,user_id,from_station,to_station,profile,coach,fare,route_json,token,status,created_at,expires_at,used_at`

func scanTicket(r interface{ Scan(...any) error }) (*Ticket, error) {
	var t Ticket
	var used sql.NullTime
	err := r.Scan(&t.ID, &t.UserID, &t.FromStation, &t.ToStation, &t.Profile, &t.Coach, &t.Fare,
		&t.RouteJSON, &t.Token, &t.Status, &t.CreatedAt, &t.ExpiresAt, &used)
	if used.Valid {
		t.UsedAt = &used.Time
	}
	return &t, err
}

func (p *Postgres) CreateTicket(ctx context.Context, t *Ticket) error {
	_, err := p.db.ExecContext(ctx, `INSERT INTO tickets (`+ticketCols+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		t.ID, t.UserID, t.FromStation, t.ToStation, t.Profile, t.Coach, t.Fare, t.RouteJSON, t.Token, t.Status,
		t.CreatedAt, t.ExpiresAt, t.UsedAt)
	return err
}

func (p *Postgres) Ticket(ctx context.Context, id string) (*Ticket, error) {
	t, err := scanTicket(p.db.QueryRowContext(ctx, `SELECT `+ticketCols+` FROM tickets WHERE id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

func (p *Postgres) TicketsByUser(ctx context.Context, userID string) ([]Ticket, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT `+ticketCols+` FROM tickets WHERE user_id=$1 ORDER BY created_at DESC LIMIT 100`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Ticket
	for rows.Next() {
		t, err := scanTicket(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// MarkTicketUsed is atomic: the conditional UPDATE prevents double validation.
func (p *Postgres) MarkTicketUsed(ctx context.Context, id string, at time.Time) error {
	res, err := p.db.ExecContext(ctx, `UPDATE tickets SET status='USED', used_at=$2 WHERE id=$1 AND status='ACTIVE'`, id, at)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 1 {
		return nil
	}
	if _, err := p.Ticket(ctx, id); err != nil {
		return err
	}
	return ErrAlreadyUsed
}

func (p *Postgres) CountTicketsSince(ctx context.Context, since time.Time) (int, error) {
	var n int
	err := p.db.QueryRowContext(ctx, `SELECT count(*) FROM tickets WHERE created_at >= $1`, since).Scan(&n)
	return n, err
}

func (p *Postgres) CreateIncident(ctx context.Context, i *Incident) error {
	_, err := p.db.ExecContext(ctx, `INSERT INTO incidents (id,type,severity,station_id,line_id,title,detail,source,status,created_at,updated_at)
		VALUES ($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,$7,$8,$9,$10,$11)`,
		i.ID, i.Type, i.Severity, i.StationID, i.LineID, i.Title, i.Detail, i.Source, i.Status, i.CreatedAt, i.UpdatedAt)
	return err
}

func (p *Postgres) Incidents(ctx context.Context, status string, limit int) ([]Incident, error) {
	q := `SELECT id,type,severity,COALESCE(station_id,''),COALESCE(line_id,''),title,detail,source,status,created_at,updated_at FROM incidents`
	args := []any{}
	if status != "" {
		q += ` WHERE status=$1`
		args = append(args, status)
	}
	q += fmt.Sprintf(` ORDER BY created_at DESC LIMIT %d`, limit)
	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Incident
	for rows.Next() {
		var i Incident
		if err := rows.Scan(&i.ID, &i.Type, &i.Severity, &i.StationID, &i.LineID, &i.Title, &i.Detail, &i.Source, &i.Status, &i.CreatedAt, &i.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (p *Postgres) SetIncidentStatus(ctx context.Context, id, status string) error {
	res, err := p.db.ExecContext(ctx, `UPDATE incidents SET status=$2, updated_at=now() WHERE id=$1`, id, status)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Postgres) RecordOccupancy(ctx context.Context, recs []OccupancyRecord) error {
	if len(recs) == 0 {
		return nil
	}
	var sb strings.Builder
	sb.WriteString(`INSERT INTO occupancy_records (station_id,at,density,expected,source) VALUES `)
	args := make([]any, 0, len(recs)*5)
	for i, r := range recs {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, "($%d,$%d,$%d,$%d,$%d)", i*5+1, i*5+2, i*5+3, i*5+4, i*5+5)
		args = append(args, r.StationID, r.At, r.Density, r.Expected, r.Source)
	}
	_, err := p.db.ExecContext(ctx, sb.String(), args...)
	return err
}

func (p *Postgres) OccupancyHistory(ctx context.Context, stationID string, since time.Time) ([]OccupancyRecord, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT station_id,at,density,expected,source FROM occupancy_records
		WHERE station_id=$1 AND at >= $2 ORDER BY at`, stationID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OccupancyRecord
	for rows.Next() {
		var r OccupancyRecord
		if err := rows.Scan(&r.StationID, &r.At, &r.Density, &r.Expected, &r.Source); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (p *Postgres) Audit(ctx context.Context, actor, action, detail string) error {
	_, err := p.db.ExecContext(ctx, `INSERT INTO audit_logs (actor,action,detail) VALUES ($1,$2,$3)`, actor, action, detail)
	return err
}

// SeedNetwork upserts lines and stations so SQL analytics can join on them.
func (p *Postgres) SeedNetwork(ctx context.Context, lines []LineRow, stations []StationRow) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, s := range stations {
		if _, err := tx.ExecContext(ctx, `INSERT INTO stations (id,name,weight,platform_capacity) VALUES ($1,$2,$3,$4)
			ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name, weight=EXCLUDED.weight, platform_capacity=EXCLUDED.platform_capacity`,
			s.ID, s.Name, s.Weight, s.Capacity); err != nil {
			return err
		}
	}
	for _, l := range lines {
		if _, err := tx.ExecContext(ctx, `INSERT INTO lines (id,name,color,coaches,headway_min) VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name, color=EXCLUDED.color, coaches=EXCLUDED.coaches, headway_min=EXCLUDED.headway_min`,
			l.ID, l.Name, l.Color, l.Coaches, l.HeadwayMin); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM line_stations WHERE line_id=$1`, l.ID); err != nil {
			return err
		}
		for i, sid := range l.Stations {
			if _, err := tx.ExecContext(ctx, `INSERT INTO line_stations (line_id,station_id,seq) VALUES ($1,$2,$3)`, l.ID, sid, i); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

type LineRow struct {
	ID, Name, Color string
	Coaches         int
	HeadwayMin      float64
	Stations        []string
}

type StationRow struct {
	ID, Name string
	Weight   float64
	Capacity int
}
