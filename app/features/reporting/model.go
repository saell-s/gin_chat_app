package reporting

import "time"

// Grouping options for time series reports.
const (
	GroupByDay   = "day"
	GroupByWeek  = "week"
	GroupByMonth = "month"
)

// Order is a single sales record.
type Order struct {
	ID          int64     `json:"id"`
	UserID      int64     `json:"user_id"`
	Product     string    `json:"product"`
	AmountCents int64     `json:"amount_cents"`
	Amount      float64   `json:"amount"`
	Currency    string    `json:"currency"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

// OrderFilter scopes order queries.
type OrderFilter struct {
	From  time.Time
	To    time.Time
	Limit int
}

// Bucket is one aggregated period inside a sales report.
type Bucket struct {
	Period       string  `json:"period"`
	Orders       int64   `json:"orders"`
	RevenueCents int64   `json:"revenue_cents"`
	Revenue      float64 `json:"revenue"`
}

// SalesReport is the time series payload.
type SalesReport struct {
	From         time.Time `json:"from"`
	To           time.Time `json:"to"`
	GroupBy      string    `json:"group_by"`
	Currency     string    `json:"currency"`
	Buckets      []Bucket  `json:"buckets"`
	TotalOrders  int64     `json:"total_orders"`
	TotalCents   int64     `json:"total_revenue_cents"`
	TotalRevenue float64   `json:"total_revenue"`
	GeneratedAt  time.Time `json:"generated_at"`
}
