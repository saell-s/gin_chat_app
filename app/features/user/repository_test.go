package user

import (
	"context"
	"errors"
	"testing"

	"gin/app/shared/db"
	"gin/app/shared/utils"
)

func testRepo() Repository {
	return NewRepository(&db.Database{Mode: db.ModeMemory, Mem: db.NewMemory()})
}

func TestCreateAndGet(t *testing.T) {
	repo := testRepo()
	ctx := context.Background()

	u := &User{Email: "a@example.com", PasswordHash: "hash", Name: "A", Role: RoleUser, Status: StatusActive}
	if err := repo.Create(ctx, u); err != nil {
		t.Fatalf("create: %v", err)
	}
	if u.ID == 0 {
		t.Fatal("expected generated id")
	}

	got, err := repo.GetByEmail(ctx, "A@Example.com")
	if err != nil {
		t.Fatalf("get by email: %v", err)
	}
	if got.ID != u.ID || got.Name != "A" {
		t.Errorf("unexpected user: %+v", got)
	}

	if err := repo.Create(ctx, &User{Email: "a@example.com", Role: RoleUser, Status: StatusActive}); !errors.Is(err, utils.ErrConflict) {
		t.Errorf("expected conflict on duplicate email, got %v", err)
	}
}

func TestListFiltersAndPagination(t *testing.T) {
	repo := testRepo()
	ctx := context.Background()

	seed := []User{
		{Email: "admin@example.com", Name: "Ada", Role: RoleAdmin, Status: StatusActive},
		{Email: "bob@example.com", Name: "Bob", Role: RoleUser, Status: StatusActive},
		{Email: "cat@example.com", Name: "Cat", Role: RoleUser, Status: StatusSuspended},
	}
	for i := range seed {
		seed[i].PasswordHash = "x"
		if err := repo.Create(ctx, &seed[i]); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	all, total, err := repo.List(ctx, ListFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 3 || len(all) != 3 {
		t.Fatalf("expected 3 users, got total=%d len=%d", total, len(all))
	}

	users, total, err := repo.List(ctx, ListFilter{Role: RoleUser, Status: StatusActive})
	if err != nil {
		t.Fatalf("list filtered: %v", err)
	}
	if total != 1 || len(users) != 1 || users[0].Email != "bob@example.com" {
		t.Errorf("unexpected filtered result: total=%d %+v", total, users)
	}

	_, total, err = repo.List(ctx, ListFilter{Query: "cat"})
	if err != nil || total != 1 {
		t.Errorf("search should match by name/email, got total=%d err=%v", total, err)
	}

	page2, total, err := repo.List(ctx, ListFilter{Page: 2, PageSize: 2})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if total != 3 || len(page2) != 1 {
		t.Errorf("expected 1 user on page 2, got %d (total %d)", len(page2), total)
	}
}

func TestUpdateAndDelete(t *testing.T) {
	repo := testRepo()
	ctx := context.Background()

	u := &User{Email: "x@example.com", PasswordHash: "h", Name: "X", Role: RoleUser, Status: StatusActive}
	if err := repo.Create(ctx, u); err != nil {
		t.Fatalf("create: %v", err)
	}

	u.Name = "Renamed"
	u.Role = RoleEditor
	if err := repo.Update(ctx, u); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := repo.GetByID(ctx, u.ID)
	if err != nil || got.Name != "Renamed" || got.Role != RoleEditor {
		t.Fatalf("update not persisted: %+v err=%v", got, err)
	}

	if err := repo.SetPassword(ctx, u.ID, "newhash"); err != nil {
		t.Fatalf("set password: %v", err)
	}
	got, _ = repo.GetByID(ctx, u.ID)
	if got.PasswordHash != "newhash" {
		t.Errorf("password not updated: %q", got.PasswordHash)
	}

	if err := repo.Delete(ctx, u.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.GetByID(ctx, u.ID); !errors.Is(err, utils.ErrNotFound) {
		t.Errorf("expected not found after delete, got %v", err)
	}
	if err := repo.Delete(ctx, u.ID); !errors.Is(err, utils.ErrNotFound) {
		t.Errorf("expected not found on second delete, got %v", err)
	}
}
