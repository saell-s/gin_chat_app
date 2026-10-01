package reporting

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"strconv"
	"time"

	"gin/app/shared/db"
	"gin/app/shared/kafka"
	"gin/app/shared/utils"
)

const defaultRangeDays = 30

// Service implements report use cases.
type Service struct {
	repo   Repository
	events db.EventRepository
	pub    kafka.Publisher
}

func NewService(repo Repository, events db.EventRepository, pub kafka.Publisher) *Service {
	return &Service{repo: repo, events: events, pub: pub}
}

// SalesReport builds an aggregated time series.
func (s *Service) SalesReport(ctx context.Context, from, to time.Time, groupBy string) (*SalesReport, error) {
	from, to, groupBy, err := normalize(from, to, groupBy)
	if err != nil {
		return nil, err
	}
	buckets, err := s.repo.Aggregate(ctx, OrderFilter{From: from, To: to}, groupBy)
	if err != nil {
		return nil, utils.Internal(err)
	}
	report := &SalesReport{
		From:        from,
		To:          to,
		GroupBy:     groupBy,
		Currency:    "USD",
		Buckets:     buckets,
		GeneratedAt: time.Now().UTC(),
	}
	for _, b := range buckets {
		report.TotalOrders += b.Orders
		report.TotalCents += b.RevenueCents
	}
	report.TotalRevenue = float64(report.TotalCents) / 100
	return report, nil
}

// Generate records a report generation for auditing/eventing.
func (s *Service) Generate(ctx context.Context, actor string, from, to time.Time, groupBy string) (*SalesReport, error) {
	report, err := s.SalesReport(ctx, from, to, groupBy)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"from":         report.From.Format(time.RFC3339),
		"to":           report.To.Format(time.RFC3339),
		"group_by":     report.GroupBy,
		"total_orders": report.TotalOrders,
		"total_cents":  report.TotalCents,
	}
	_ = s.events.Append(ctx, "report.generated", actor, payload)
	kafka.PublishSafe(ctx, s.pub, "report.generated", payload)
	return report, nil
}

// CSV exports raw orders inside the range as a CSV document.
func (s *Service) CSV(ctx context.Context, from, to time.Time) ([]byte, string, error) {
	from, to, _, err := normalize(from, to, GroupByDay)
	if err != nil {
		return nil, "", err
	}
	orders, err := s.repo.List(ctx, OrderFilter{From: from, To: to, Limit: 5000})
	if err != nil {
		return nil, "", utils.Internal(err)
	}

	buf := &bytes.Buffer{}
	w := csv.NewWriter(buf)
	_ = w.Write([]string{"id", "created_at", "user_id", "product", "amount", "currency", "status"})
	for _, o := range orders {
		_ = w.Write([]string{
			strconv.FormatInt(o.ID, 10),
			o.CreatedAt.UTC().Format(time.RFC3339),
			strconv.FormatInt(o.UserID, 10),
			o.Product,
			fmt.Sprintf("%.2f", o.Amount),
			o.Currency,
			o.Status,
		})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, "", utils.Internal(err)
	}

	filename := fmt.Sprintf("orders_%s_%s.csv",
		from.Format("20060102"), to.Format("20060102"))
	return buf.Bytes(), filename, nil
}

// normalize applies defaults and validates the reporting window.
func normalize(from, to time.Time, groupBy string) (time.Time, time.Time, string, error) {
	if to.IsZero() {
		to = time.Now().UTC().Add(time.Hour).Truncate(time.Hour)
	} else {
		to = to.UTC()
	}
	if from.IsZero() {
		from = to.AddDate(0, 0, -defaultRangeDays)
	} else {
		from = from.UTC()
	}
	if !from.Before(to) {
		return time.Time{}, time.Time{}, "", utils.BadRequest("'from' must be before 'to'")
	}
	if to.Sub(from) > 5*365*24*time.Hour {
		return time.Time{}, time.Time{}, "", utils.BadRequest("date range is too large")
	}
	if groupBy == "" {
		groupBy = GroupByDay
	}
	switch groupBy {
	case GroupByDay, GroupByWeek, GroupByMonth:
	default:
		return time.Time{}, time.Time{}, "", utils.BadRequest("group_by must be one of day, week, month")
	}
	return from, to, groupBy, nil
}

// ParseDate accepts RFC3339 or YYYY-MM-DD query values.
func ParseDate(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", value); err == nil {
		return t, nil
	}
	return time.Time{}, utils.BadRequest("date must be RFC3339 or YYYY-MM-DD")
}
