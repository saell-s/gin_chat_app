package reporting

import (
	"context"
	"testing"
	"time"

	"gin/app/shared/configs"
	"gin/app/shared/db"
)

func testDB() *db.Database {
	return &db.Database{Mode: db.ModeMemory, Mem: db.NewMemory()}
}

func addOrder(t *testing.T, mem *db.Memory, at time.Time, cents int64) {
	t.Helper()
	mem.InsertOrder(db.OrderRow{
		UserID:      1,
		Product:     "Pro Plan",
		AmountCents: cents,
		Currency:    "USD",
		Status:      "completed",
		CreatedAt:   at.UTC(),
	})
}

func TestAggregateByDay(t *testing.T) {
	d := testDB()
	mem := d.Mem
	base := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	addOrder(t, mem, base, 1000)
	addOrder(t, mem, base.Add(2*time.Hour), 2500)
	addOrder(t, mem, base.AddDate(0, 0, 1), 500)
	addOrder(t, mem, base.AddDate(0, 0, 7), 700) // outside window

	repo := NewRepository(d)
	buckets, err := repo.Aggregate(context.Background(), OrderFilter{
		From: base,
		To:   base.AddDate(0, 0, 7),
	}, GroupByDay)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if len(buckets) != 2 {
		t.Fatalf("expected 2 buckets, got %d: %+v", len(buckets), buckets)
	}
	if buckets[0].Period != "2026-09-10" || buckets[0].Orders != 2 || buckets[0].RevenueCents != 3500 {
		t.Errorf("unexpected first bucket: %+v", buckets[0])
	}
	if buckets[1].Period != "2026-09-11" || buckets[1].RevenueCents != 500 {
		t.Errorf("unexpected second bucket: %+v", buckets[1])
	}
	if buckets[0].Revenue != 35 {
		t.Errorf("expected revenue in major units, got %v", buckets[0].Revenue)
	}
}

func TestAggregateWeekAlignsToMonday(t *testing.T) {
	d := testDB()
	// 2026-09-16 is a Wednesday; its week starts Monday 2026-09-14.
	addOrder(t, d.Mem, time.Date(2026, 9, 16, 9, 0, 0, 0, time.UTC), 100)
	addOrder(t, d.Mem, time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC), 200)
	addOrder(t, d.Mem, time.Date(2026, 9, 13, 23, 0, 0, 0, time.UTC), 400) // previous week

	repo := NewRepository(d)
	buckets, err := repo.Aggregate(context.Background(), OrderFilter{
		From: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	}, GroupByWeek)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if len(buckets) != 2 {
		t.Fatalf("expected 2 week buckets, got %d: %+v", len(buckets), buckets)
	}
	if buckets[0].Period != "2026-09-07" || buckets[0].RevenueCents != 400 {
		t.Errorf("unexpected week 1: %+v", buckets[0])
	}
	if buckets[1].Period != "2026-09-14" || buckets[1].RevenueCents != 300 {
		t.Errorf("unexpected week 2: %+v", buckets[1])
	}
}

func TestNormalizeValidation(t *testing.T) {
	to := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	gotFrom, gotTo, group, err := normalize(from, to, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !gotFrom.Equal(from) || !gotTo.Equal(to) || group != GroupByDay {
		t.Errorf("unexpected normalize output: %v %v %s", gotFrom, gotTo, group)
	}

	if _, _, _, err := normalize(to, from, GroupByDay); err == nil {
		t.Error("expected error when from is after to")
	}
	if _, _, _, err := normalize(from, to, "hour"); err == nil {
		t.Error("expected error for unsupported group_by")
	}
	if _, _, _, gErr := normalize(time.Time{}, time.Time{}, GroupByMonth); gErr != nil {
		t.Errorf("zero range should default, got %v", gErr)
	}
}

func TestParseDate(t *testing.T) {
	if _, err := ParseDate(""); err != nil {
		t.Errorf("empty date should be allowed: %v", err)
	}
	if _, err := ParseDate("2026-09-01"); err != nil {
		t.Errorf("YYYY-MM-DD should parse: %v", err)
	}
	if _, err := ParseDate("2026-09-01T00:00:00Z"); err != nil {
		t.Errorf("RFC3339 should parse: %v", err)
	}
	if _, err := ParseDate("banana"); err == nil {
		t.Error("invalid date should be rejected")
	}
}

func TestMemoryBackedByConfigDefaults(t *testing.T) {
	cfg := configs.Load()
	if cfg.Database.Mode == "" {
		t.Error("database mode must have a default")
	}
}
