package auth

import (
	"context"
	"errors"
	"time"

	"gin/app/shared/db"
	"gin/app/shared/utils"
)

// TokenRepository persists refresh tokens.
type TokenRepository interface {
	Save(ctx context.Context, t *RefreshToken) error
	FindByHash(ctx context.Context, hash string) (*RefreshToken, error)
	Revoke(ctx context.Context, id int64) error
	RevokeAllForUser(ctx context.Context, userID int64) (int64, error)
}

// NewTokenRepository selects the backend implementation.
func NewTokenRepository(d *db.Database) TokenRepository {
	if d.IsPostgres() {
		return &pgTokenRepo{q: d.Pool}
	}
	return &memTokenRepo{mem: d.Mem}
}

// ---- memory ----

type memTokenRepo struct{ mem *db.Memory }

func (r *memTokenRepo) Save(ctx context.Context, t *RefreshToken) error {
	row := r.mem.InsertToken(db.TokenRow{
		UserID:    t.UserID,
		TokenHash: t.TokenHash,
		ExpiresAt: t.ExpiresAt,
		CreatedAt: t.CreatedAt,
		RevokedAt: t.RevokedAt,
	})
	t.ID = row.ID
	t.CreatedAt = row.CreatedAt
	return nil
}

func (r *memTokenRepo) FindByHash(ctx context.Context, hash string) (*RefreshToken, error) {
	row, ok := r.mem.GetTokenByHash(hash)
	if !ok {
		return nil, utils.ErrNotFound
	}
	return &RefreshToken{
		ID:        row.ID,
		UserID:    row.UserID,
		TokenHash: row.TokenHash,
		ExpiresAt: row.ExpiresAt,
		CreatedAt: row.CreatedAt,
		RevokedAt: row.RevokedAt,
	}, nil
}

func (r *memTokenRepo) Revoke(ctx context.Context, id int64) error {
	if !r.mem.RevokeToken(id) {
		return utils.ErrNotFound
	}
	return nil
}

func (r *memTokenRepo) RevokeAllForUser(ctx context.Context, userID int64) (int64, error) {
	return r.mem.RevokeUserTokens(userID), nil
}

// ---- postgres ----

type pgTokenRepo struct{ q db.Pgx }

func (r *pgTokenRepo) Save(ctx context.Context, t *RefreshToken) error {
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}
	return r.q.QueryRow(ctx,
		`INSERT INTO refresh_tokens (user_id, token_hash, expires_at, created_at)
		 VALUES ($1,$2,$3,$4) RETURNING id`,
		t.UserID, t.TokenHash, t.ExpiresAt, t.CreatedAt,
	).Scan(&t.ID)
}

func (r *pgTokenRepo) FindByHash(ctx context.Context, hash string) (*RefreshToken, error) {
	t := &RefreshToken{}
	err := r.q.QueryRow(ctx,
		`SELECT id, user_id, token_hash, expires_at, created_at, revoked_at
		 FROM refresh_tokens WHERE token_hash = $1`, hash,
	).Scan(&t.ID, &t.UserID, &t.TokenHash, &t.ExpiresAt, &t.CreatedAt, &t.RevokedAt)
	if err != nil {
		if errors.Is(err, db.ErrNoRows) {
			return nil, utils.ErrNotFound
		}
		return nil, err
	}
	return t, nil
}

func (r *pgTokenRepo) Revoke(ctx context.Context, id int64) error {
	tag, err := r.q.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return utils.ErrNotFound
	}
	return nil
}

func (r *pgTokenRepo) RevokeAllForUser(ctx context.Context, userID int64) (int64, error) {
	tag, err := r.q.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
