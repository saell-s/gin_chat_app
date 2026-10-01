package dashboard

import (
	"time"

	"gin/app/shared/db"
)

// UserStats aggregates account counts.
type UserStats struct {
	Total     int64            `json:"total"`
	Active    int64            `json:"active"`
	Suspended int64            `json:"suspended"`
	ByRole    map[string]int64 `json:"by_role"`
}

// OrderStats aggregates order counts and revenue.
type OrderStats struct {
	Total        int64            `json:"total"`
	RevenueCents int64            `json:"revenue_cents"`
	Currency     string           `json:"currency"`
	ByStatus     map[string]int64 `json:"by_status"`
}

// Summary is the dashboard payload.
type Summary struct {
	Users       UserStats    `json:"users"`
	Orders      OrderStats   `json:"orders"`
	TopProducts []TopProduct `json:"top_products"`
	GeneratedAt time.Time    `json:"generated_at"`
}

// TopProduct ranks products by revenue.
type TopProduct struct {
	Product      string `json:"product"`
	Orders       int64  `json:"orders"`
	RevenueCents int64  `json:"revenue_cents"`
}

// ActivityPage is a list of recent events.
type ActivityPage struct {
	Events []db.Event `json:"events"`
}
