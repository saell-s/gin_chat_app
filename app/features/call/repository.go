package call

import (
	"context"

	"gin/app/shared/db"
	"gin/app/shared/utils"
)

// Repository persists call records.
type Repository interface {
	Create(ctx context.Context, c *Call) error
	Get(ctx context.Context, id int64) (*Call, error)
	Update(ctx context.Context, c *Call) error
	ListForUser(ctx context.Context, userID int64, limit int) ([]Call, error)
}

// NewRepository selects the backend implementation.
func NewRepository(d *db.Database) Repository {
	if d.IsPostgres() {
		return &pgRepo{q: d.Pool}
	}
	return &memRepo{mem: d.Mem}
}

// ---- memory ----

type memRepo struct{ mem *db.Memory }

func (r *memRepo) Create(ctx context.Context, c *Call) error {
	row := r.mem.InsertCall(db.CallRow{
		CallerID:        c.CallerID,
		CalleeID:        c.CalleeID,
		ConversationID:  c.ConversationID,
		Kind:            c.Kind,
		Status:          c.Status,
		StartedAt:       c.StartedAt,
		AnsweredAt:      c.AnsweredAt,
		EndedAt:         c.EndedAt,
		DurationSeconds: c.DurationSeconds,
		CreatedAt:       c.CreatedAt,
	})
	c.ID = row.ID
	c.CreatedAt = row.CreatedAt
	c.StartedAt = row.StartedAt
	return nil
}

func (r *memRepo) Get(ctx context.Context, id int64) (*Call, error) {
	row, ok := r.mem.GetCall(id)
	if !ok {
		return nil, utils.ErrNotFound
	}
	return toCall(row), nil
}

func (r *memRepo) Update(ctx context.Context, c *Call) error {
	if _, ok := r.mem.UpdateCall(db.CallRow{
		ID:              c.ID,
		CallerID:        c.CallerID,
		CalleeID:        c.CalleeID,
		ConversationID:  c.ConversationID,
		Kind:            c.Kind,
		Status:          c.Status,
		StartedAt:       c.StartedAt,
		AnsweredAt:      c.AnsweredAt,
		EndedAt:         c.EndedAt,
		DurationSeconds: c.DurationSeconds,
		CreatedAt:       c.CreatedAt,
	}); !ok {
		return utils.ErrNotFound
	}
	return nil
}

func (r *memRepo) ListForUser(ctx context.Context, userID int64, limit int) ([]Call, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	all := r.mem.CallSnapshot()
	out := make([]Call, 0, limit)
	for i := len(all) - 1; i >= 0 && len(out) < limit; i-- {
		if all[i].CallerID == userID || all[i].CalleeID == userID {
			out = append(out, *toCall(all[i]))
		}
	}
	return out, nil
}

func toCall(row db.CallRow) *Call {
	return &Call{
		ID:              row.ID,
		CallerID:        row.CallerID,
		CalleeID:        row.CalleeID,
		ConversationID:  row.ConversationID,
		Kind:            row.Kind,
		Status:          row.Status,
		StartedAt:       row.StartedAt,
		AnsweredAt:      row.AnsweredAt,
		EndedAt:         row.EndedAt,
		DurationSeconds: row.DurationSeconds,
		CreatedAt:       row.CreatedAt,
	}
}

// ---- postgres ----

type pgRepo struct{ q db.Pgx }

const callColumns = `id, caller_id, callee_id, conversation_id, kind, status,
	started_at, answered_at, ended_at, duration_seconds, created_at`

func scanCall(row interface{ Scan(dest ...any) error }) (*Call, error) {
	c := &Call{}
	err := row.Scan(&c.ID, &c.CallerID, &c.CalleeID, &c.ConversationID, &c.Kind, &c.Status,
		&c.StartedAt, &c.AnsweredAt, &c.EndedAt, &c.DurationSeconds, &c.CreatedAt)
	if err != nil {
		if err == db.ErrNoRows {
			return nil, utils.ErrNotFound
		}
		return nil, err
	}
	return c, nil
}

func (r *pgRepo) Create(ctx context.Context, c *Call) error {
	return r.q.QueryRow(ctx,
		`INSERT INTO calls (caller_id, callee_id, conversation_id, kind, status, started_at, answered_at, ended_at, duration_seconds, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		 RETURNING id, created_at`,
		c.CallerID, c.CalleeID, c.ConversationID, c.Kind, c.Status,
		c.StartedAt, c.AnsweredAt, c.EndedAt, c.DurationSeconds, c.CreatedAt,
	).Scan(&c.ID, &c.CreatedAt)
}

func (r *pgRepo) Get(ctx context.Context, id int64) (*Call, error) {
	return scanCall(r.q.QueryRow(ctx, `SELECT `+callColumns+` FROM calls WHERE id = $1`, id))
}

func (r *pgRepo) Update(ctx context.Context, c *Call) error {
	_, err := r.q.Exec(ctx,
		`UPDATE calls SET status=$1, answered_at=$2, ended_at=$3, duration_seconds=$4 WHERE id=$5`,
		c.Status, c.AnsweredAt, c.EndedAt, c.DurationSeconds, c.ID)
	return err
}

func (r *pgRepo) ListForUser(ctx context.Context, userID int64, limit int) ([]Call, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := r.q.Query(ctx,
		`SELECT `+callColumns+` FROM calls
		 WHERE caller_id = $1 OR callee_id = $1
		 ORDER BY created_at DESC
		 LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Call, 0, limit)
	for rows.Next() {
		c := Call{}
		if err := rows.Scan(&c.ID, &c.CallerID, &c.CalleeID, &c.ConversationID, &c.Kind, &c.Status,
			&c.StartedAt, &c.AnsweredAt, &c.EndedAt, &c.DurationSeconds, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
