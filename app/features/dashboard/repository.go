package dashboard

import (
	"context"
	"sort"

	"gin/app/shared/db"
)

// StatsRepository provides pre-aggregated counters for the dashboard.
type StatsRepository interface {
	UserStats(ctx context.Context) (UserStats, error)
	OrderStats(ctx context.Context) (OrderStats, error)
	TopProducts(ctx context.Context, limit int) ([]TopProduct, error)
}

// NewStatsRepository selects the backend implementation.
func NewStatsRepository(d *db.Database) StatsRepository {
	if d.IsPostgres() {
		return &pgStatsRepo{q: d.Pool}
	}
	return &memStatsRepo{mem: d.Mem}
}

// ---- memory ----

type memStatsRepo struct{ mem *db.Memory }

func (r *memStatsRepo) UserStats(ctx context.Context) (UserStats, error) {
	stats := UserStats{ByRole: map[string]int64{}}
	for _, u := range r.mem.UserSnapshot() {
		stats.Total++
		stats.ByRole[u.Role]++
		switch u.Status {
		case "active":
			stats.Active++
		case "suspended":
			stats.Suspended++
		}
	}
	return stats, nil
}

func (r *memStatsRepo) OrderStats(ctx context.Context) (OrderStats, error) {
	stats := OrderStats{ByStatus: map[string]int64{}, Currency: "USD"}
	for _, o := range r.mem.OrderSnapshot() {
		stats.Total++
		stats.RevenueCents += o.AmountCents
		stats.ByStatus[o.Status]++
	}
	return stats, nil
}

func (r *memStatsRepo) TopProducts(ctx context.Context, limit int) ([]TopProduct, error) {
	agg := map[string]*TopProduct{}
	for _, o := range r.mem.OrderSnapshot() {
		t := agg[o.Product]
		if t == nil {
			t = &TopProduct{Product: o.Product}
			agg[o.Product] = t
		}
		t.Orders++
		if o.Status == "completed" {
			t.RevenueCents += o.AmountCents
		}
	}
	out := make([]TopProduct, 0, len(agg))
	for _, t := range agg {
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].RevenueCents != out[j].RevenueCents {
			return out[i].RevenueCents > out[j].RevenueCents
		}
		return out[i].Product < out[j].Product
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ---- postgres ----

type pgStatsRepo struct{ q db.Pgx }

func (r *pgStatsRepo) UserStats(ctx context.Context) (UserStats, error) {
	rows, err := r.q.Query(ctx, `SELECT role, status, count(*) FROM users GROUP BY role, status`)
	if err != nil {
		return UserStats{}, err
	}
	defer rows.Close()

	stats := UserStats{ByRole: map[string]int64{}}
	for rows.Next() {
		var role, status string
		var n int64
		if err := rows.Scan(&role, &status, &n); err != nil {
			return UserStats{}, err
		}
		stats.Total += n
		stats.ByRole[role] += n
		switch status {
		case "active":
			stats.Active += n
		case "suspended":
			stats.Suspended += n
		}
	}
	return stats, rows.Err()
}

func (r *pgStatsRepo) OrderStats(ctx context.Context) (OrderStats, error) {
	rows, err := r.q.Query(ctx,
		`SELECT status, count(*), COALESCE(sum(amount_cents),0) FROM orders GROUP BY status`)
	if err != nil {
		return OrderStats{}, err
	}
	defer rows.Close()

	stats := OrderStats{ByStatus: map[string]int64{}, Currency: "USD"}
	for rows.Next() {
		var status string
		var n, cents int64
		if err := rows.Scan(&status, &n, &cents); err != nil {
			return OrderStats{}, err
		}
		stats.Total += n
		stats.ByStatus[status] += n
		if status == "completed" {
			stats.RevenueCents += cents
		}
	}
	return stats, rows.Err()
}

func (r *pgStatsRepo) TopProducts(ctx context.Context, limit int) ([]TopProduct, error) {
	if limit <= 0 {
		limit = 5
	}
	rows, err := r.q.Query(ctx,
		`SELECT product, count(*), COALESCE(sum(amount_cents) FILTER (WHERE status = 'completed'),0)
		 FROM orders GROUP BY product
		 ORDER BY 3 DESC, 1 ASC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]TopProduct, 0, limit)
	for rows.Next() {
		var t TopProduct
		if err := rows.Scan(&t.Product, &t.Orders, &t.RevenueCents); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
