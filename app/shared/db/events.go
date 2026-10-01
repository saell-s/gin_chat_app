package db

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Event is an auditable activity record shared by all features.
type Event struct {
	ID        int64     `json:"id"`
	Type      string    `json:"type"`
	Actor     string    `json:"actor"`
	Payload   any       `json:"payload,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// EventRepository appends and reads activity events.
type EventRepository interface {
	Append(ctx context.Context, typ, actor string, payload any) error
	List(ctx context.Context, limit int) ([]Event, error)
}

// NewEventRepository picks the backend implementation.
func NewEventRepository(d *Database) EventRepository {
	if d.IsPostgres() {
		return &pgEventRepo{pool: d.Pool}
	}
	return &memEventRepo{mem: d.Mem}
}

type pgEventRepo struct{ pool Pgx }

func (r *pgEventRepo) Append(ctx context.Context, typ, actor string, payload any) error {
	raw := "{}"
	if payload != nil {
		b, err := json.Marshal(payload)
		if err == nil {
			raw = string(b)
		}
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO events (type, actor, payload) VALUES ($1,$2,$3)`, typ, actor, raw)
	return err
}

func (r *pgEventRepo) List(ctx context.Context, limit int) ([]Event, error) {
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, type, actor, payload, created_at FROM events ORDER BY created_at DESC, id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Event, 0, limit)
	for rows.Next() {
		var e Event
		var raw string
		if err := rows.Scan(&e.ID, &e.Type, &e.Actor, &raw, &e.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(raw), &e.Payload)
		out = append(out, e)
	}
	return out, rows.Err()
}

type memEventRepo struct{ mem *Memory }

func (r *memEventRepo) Append(ctx context.Context, typ, actor string, payload any) error {
	raw := "{}"
	if payload != nil {
		if b, err := json.Marshal(payload); err == nil {
			raw = string(b)
		}
	}
	r.mem.InsertEvent(EventRow{Type: typ, Actor: actor, Payload: raw})
	return nil
}

func (r *memEventRepo) List(ctx context.Context, limit int) ([]Event, error) {
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	snapshot := r.mem.EventSnapshot()
	out := make([]Event, 0, limit)
	for i := len(snapshot) - 1; i >= 0 && len(out) < limit; i-- {
		var payload any
		_ = json.Unmarshal([]byte(snapshot[i].Payload), &payload)
		out = append(out, Event{
			ID:        snapshot[i].ID,
			Type:      snapshot[i].Type,
			Actor:     snapshot[i].Actor,
			Payload:   payload,
			CreatedAt: snapshot[i].CreatedAt,
		})
	}
	return out, nil
}

// Pgx is the subset of pgx operations the repositories need, so both the
// pool and a plain transaction can satisfy it.
type Pgx interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// ErrNoRows re-exports the pgx sentinel so feature packages can compare.
var ErrNoRows = pgx.ErrNoRows
