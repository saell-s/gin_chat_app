package user

import (
	"context"
	"errors"
	"strings"
	"time"

	"gin/app/shared/db"
	"gin/app/shared/utils"
)

// Repository persists users on either backend.
type Repository interface {
	Create(ctx context.Context, u *User) error
	GetByID(ctx context.Context, id int64) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	List(ctx context.Context, f ListFilter) ([]User, int64, error)
	Update(ctx context.Context, u *User) error
	SetPassword(ctx context.Context, id int64, hash string) error
	Delete(ctx context.Context, id int64) error
}

// NewRepository selects the implementation for the configured backend.
func NewRepository(d *db.Database) Repository {
	if d.IsPostgres() {
		return &pgRepo{q: d.Pool}
	}
	return &memRepo{mem: d.Mem}
}

// ---- memory implementation ----

type memRepo struct{ mem *db.Memory }

func toUser(r db.UserRow) *User {
	return &User{
		ID:           r.ID,
		Email:        r.Email,
		PasswordHash: r.PasswordHash,
		Name:         r.Name,
		Role:         r.Role,
		Status:       r.Status,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
	}
}

func fromUser(u *User) db.UserRow {
	return db.UserRow{
		ID:           u.ID,
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
		Name:         u.Name,
		Role:         u.Role,
		Status:       u.Status,
		CreatedAt:    u.CreatedAt,
		UpdatedAt:    u.UpdatedAt,
	}
}

func (r *memRepo) Create(ctx context.Context, u *User) error {
	if _, ok := r.mem.GetUserByEmail(u.Email); ok {
		return utils.ErrConflict
	}
	row := r.mem.InsertUser(fromUser(u))
	*u = *toUser(row)
	return nil
}

func (r *memRepo) GetByID(ctx context.Context, id int64) (*User, error) {
	row, ok := r.mem.GetUser(id)
	if !ok {
		return nil, utils.ErrNotFound
	}
	return toUser(row), nil
}

func (r *memRepo) GetByEmail(ctx context.Context, email string) (*User, error) {
	row, ok := r.mem.GetUserByEmail(strings.ToLower(strings.TrimSpace(email)))
	if !ok {
		return nil, utils.ErrNotFound
	}
	return toUser(row), nil
}

func (r *memRepo) List(ctx context.Context, f ListFilter) ([]User, int64, error) {
	q := strings.ToLower(strings.TrimSpace(f.Query))
	match := make([]db.UserRow, 0)
	for _, row := range r.mem.UserSnapshot() {
		if f.Role != "" && row.Role != f.Role {
			continue
		}
		if f.Status != "" && row.Status != f.Status {
			continue
		}
		if q != "" &&
			!strings.Contains(strings.ToLower(row.Email), q) &&
			!strings.Contains(strings.ToLower(row.Name), q) {
			continue
		}
		match = append(match, row)
	}
	total := int64(len(match))

	page, size := f.Page, f.PageSize
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = utils.DefaultPageSize
	}
	start := (page - 1) * size
	if start > len(match) {
		start = len(match)
	}
	end := start + size
	if end > len(match) {
		end = len(match)
	}

	out := make([]User, 0, end-start)
	for _, row := range match[start:end] {
		out = append(out, *toUser(row))
	}
	return out, total, nil
}

func (r *memRepo) Update(ctx context.Context, u *User) error {
	row, ok := r.mem.UpdateUser(fromUser(u))
	if !ok {
		return utils.ErrNotFound
	}
	*u = *toUser(row)
	return nil
}

func (r *memRepo) SetPassword(ctx context.Context, id int64, hash string) error {
	row, ok := r.mem.GetUser(id)
	if !ok {
		return utils.ErrNotFound
	}
	row.PasswordHash = hash
	_, ok = r.mem.UpdateUser(row)
	if !ok {
		return utils.ErrNotFound
	}
	return nil
}

func (r *memRepo) Delete(ctx context.Context, id int64) error {
	if !r.mem.DeleteUser(id) {
		return utils.ErrNotFound
	}
	return nil
}

// ---- postgres implementation ----

type pgRepo struct{ q db.Pgx }

const userColumns = `id, email, password_hash, name, role, status, created_at, updated_at`

func scanUser(row interface{ Scan(dest ...any) error }) (*User, error) {
	u := &User{}
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.Role, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, db.ErrNoRows) {
			return nil, utils.ErrNotFound
		}
		return nil, err
	}
	return u, nil
}

func (r *pgRepo) Create(ctx context.Context, u *User) error {
	err := r.q.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, name, role, status)
		 VALUES ($1,$2,$3,$4,$5)
		 ON CONFLICT (email) DO NOTHING
		 RETURNING `+userColumns,
		strings.ToLower(strings.TrimSpace(u.Email)), u.PasswordHash, u.Name, u.Role, u.Status,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.Role, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, db.ErrNoRows) {
		return utils.ErrConflict
	}
	return err
}

func (r *pgRepo) GetByID(ctx context.Context, id int64) (*User, error) {
	return scanUser(r.q.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id))
}

func (r *pgRepo) GetByEmail(ctx context.Context, email string) (*User, error) {
	return scanUser(r.q.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE email = $1`,
		strings.ToLower(strings.TrimSpace(email))))
}

func (r *pgRepo) List(ctx context.Context, f ListFilter) ([]User, int64, error) {
	page, size := f.Page, f.PageSize
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = utils.DefaultPageSize
	}

	const where = `WHERE ($1 = '' OR role = $1)
		AND ($2 = '' OR status = $2)
		AND ($3 = '' OR email ILIKE '%'||$3||'%' OR name ILIKE '%'||$3||'%')`

	var total int64
	if err := r.q.QueryRow(ctx,
		`SELECT count(*) FROM users `+where,
		f.Role, f.Status, strings.TrimSpace(f.Query),
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.q.Query(ctx,
		`SELECT `+userColumns+` FROM users `+where+`
		 ORDER BY created_at DESC, id DESC
		 LIMIT $4 OFFSET $5`,
		f.Role, f.Status, strings.TrimSpace(f.Query), size, (page-1)*size)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]User, 0, size)
	for rows.Next() {
		u := User{}
		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.Role, &u.Status, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, u)
	}
	return out, total, rows.Err()
}

func (r *pgRepo) Update(ctx context.Context, u *User) error {
	tag, err := r.q.Exec(ctx,
		`UPDATE users SET email=$1, name=$2, role=$3, status=$4, updated_at=$5 WHERE id=$6`,
		strings.ToLower(strings.TrimSpace(u.Email)), u.Name, u.Role, u.Status, time.Now().UTC(), u.ID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return utils.ErrNotFound
	}
	return nil
}

func (r *pgRepo) SetPassword(ctx context.Context, id int64, hash string) error {
	tag, err := r.q.Exec(ctx,
		`UPDATE users SET password_hash=$1, updated_at=$2 WHERE id=$3`,
		hash, time.Now().UTC(), id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return utils.ErrNotFound
	}
	return nil
}

func (r *pgRepo) Delete(ctx context.Context, id int64) error {
	tag, err := r.q.Exec(ctx, `DELETE FROM users WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return utils.ErrNotFound
	}
	return nil
}
