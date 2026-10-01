package dashboard

import (
	"context"
	"time"

	"gin/app/shared/db"
)

// Service assembles the dashboard payload.
type Service struct {
	stats  StatsRepository
	events db.EventRepository
}

func NewService(stats StatsRepository, events db.EventRepository) *Service {
	return &Service{stats: stats, events: events}
}

// Summary combines user, order and product aggregates.
func (s *Service) Summary(ctx context.Context) (*Summary, error) {
	users, err := s.stats.UserStats(ctx)
	if err != nil {
		return nil, err
	}
	orders, err := s.stats.OrderStats(ctx)
	if err != nil {
		return nil, err
	}
	top, err := s.stats.TopProducts(ctx, 5)
	if err != nil {
		return nil, err
	}
	return &Summary{
		Users:       users,
		Orders:      orders,
		TopProducts: top,
		GeneratedAt: time.Now().UTC(),
	}, nil
}

// Activity returns the most recent recorded events.
func (s *Service) Activity(ctx context.Context, limit int) (*ActivityPage, error) {
	events, err := s.events.List(ctx, limit)
	if err != nil {
		return nil, err
	}
	return &ActivityPage{Events: events}, nil
}
