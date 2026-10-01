package reporting

import (
	"context"
	"fmt"
	"time"

	"gin/app/shared/db"
)

// Repository reads sales data for reporting.
type Repository interface {
	Aggregate(ctx context.Context, f OrderFilter, groupBy string) ([]Bucket, error)
	List(ctx context.Context, f OrderFilter) ([]Order, error)
}

// NewRepository selects the backend implementation.
func NewRepository(d *db.Database) Repository {
	if d.IsPostgres() {
		return &pgRepo{q: d.Pool}
	}
	return &memRepo{mem: d.Mem}
}

func periodLayout(groupBy string) string {
	switch groupBy {
	case GroupByMonth:
		return "2006-01"
	case GroupByWeek, GroupByDay:
		return "2006-01-02"
	default:
		return "2006-01-02"
	}
}

func inRange(t, from, to time.Time) bool {
	return !t.Before(from) && t.Before(to)
}

func newBucket(period string) Bucket { return Bucket{Period: period} }

// ---- memory ----

type memRepo struct{ mem *db.Memory }

func (r *memRepo) Aggregate(ctx context.Context, f OrderFilter, groupBy string) ([]Bucket, error) {
	layout := periodLayout(groupBy)
	buckets := map[string]*Bucket{}

	for _, o := range r.mem.OrderSnapshot() {
		if !inRange(o.CreatedAt, f.From, f.To) {
			continue
		}
		var key string
		switch groupBy {
		case GroupByWeek:
			// Align to the Monday that starts the week (UTC).
			day := time.Date(o.CreatedAt.Year(), o.CreatedAt.Month(), o.CreatedAt.Day(), 0, 0, 0, 0, time.UTC)
			offset := (int(day.Weekday()) + 6) % 7
			day = day.AddDate(0, 0, -offset)
			key = day.Format(layout)
		default:
			key = o.CreatedAt.UTC().Format(layout)
		}
		b := buckets[key]
		if b == nil {
			b = ptr(newBucket(key))
			buckets[key] = b
		}
		b.Orders++
		b.RevenueCents += o.AmountCents
	}
	return sortBuckets(buckets), nil
}

func (r *memRepo) List(ctx context.Context, f OrderFilter) ([]Order, error) {
	out := make([]Order, 0, 64)
	for _, o := range r.mem.OrderSnapshot() {
		if !inRange(o.CreatedAt, f.From, f.To) {
			continue
		}
		out = append(out, toOrder(o))
		if f.Limit > 0 && len(out) >= f.Limit {
			break
		}
	}
	return out, nil
}

func toOrder(o db.OrderRow) Order {
	return Order{
		ID:          o.ID,
		UserID:      o.UserID,
		Product:     o.Product,
		AmountCents: o.AmountCents,
		Amount:      float64(o.AmountCents) / 100,
		Currency:    o.Currency,
		Status:      o.Status,
		CreatedAt:   o.CreatedAt,
	}
}

func ptr[T any](v T) *T { return &v }

func sortBuckets(m map[string]*Bucket) []Bucket {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Keys are ISO-like dates, so lexical order == chronological order.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	out := make([]Bucket, 0, len(keys))
	for _, k := range keys {
		b := *m[k]
		b.Revenue = float64(b.RevenueCents) / 100
		out = append(out, b)
	}
	return out
}

// ---- postgres ----

type pgRepo struct{ q db.Pgx }

// groupBySQL maps the validated group_by value onto SQL literals.
// The values are whitelisted, never interpolated from raw user input.
func groupBySQL(groupBy string) (unit, layout string, err error) {
	switch groupBy {
	case GroupByDay:
		return "day", "YYYY-MM-DD", nil
	case GroupByWeek:
		return "week", "YYYY-MM-DD", nil
	case GroupByMonth:
		return "month", "YYYY-MM", nil
	default:
		return "", "", fmt.Errorf("unsupported group_by %q", groupBy)
	}
}

func (r *pgRepo) Aggregate(ctx context.Context, f OrderFilter, groupBy string) ([]Bucket, error) {
	unit, layout, err := groupBySQL(groupBy)
	if err != nil {
		return nil, err
	}
	sql := fmt.Sprintf(
		`SELECT to_char(date_trunc('%s', created_at AT TIME ZONE 'UTC'), '%s') AS period,
		        count(*), COALESCE(sum(amount_cents),0)
		 FROM orders
		 WHERE created_at >= $1 AND created_at < $2
		 GROUP BY period
		 ORDER BY period`, unit, layout)

	rows, err := r.q.Query(ctx, sql, f.From, f.To)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Bucket, 0, 32)
	for rows.Next() {
		var b Bucket
		if err := rows.Scan(&b.Period, &b.Orders, &b.RevenueCents); err != nil {
			return nil, err
		}
		b.Revenue = float64(b.RevenueCents) / 100
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *pgRepo) List(ctx context.Context, f OrderFilter) ([]Order, error) {
	limit := f.Limit
	if limit <= 0 || limit > 5000 {
		limit = 5000
	}
	rows, err := r.q.Query(ctx,
		`SELECT id, user_id, product, amount_cents, currency, status, created_at
		 FROM orders
		 WHERE created_at >= $1 AND created_at < $2
		 ORDER BY created_at DESC
		 LIMIT $3`, f.From, f.To, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Order, 0, limit)
	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.ID, &o.UserID, &o.Product, &o.AmountCents, &o.Currency, &o.Status, &o.CreatedAt); err != nil {
			return nil, err
		}
		o.Amount = float64(o.AmountCents) / 100
		out = append(out, o)
	}
	return out, rows.Err()
}
