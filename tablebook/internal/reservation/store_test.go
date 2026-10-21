package reservation_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/d4b0mb/tablebook/internal/reservation"
)

// storeBase is the reference start time used by the store tests.
var storeBase = time.Date(2027, 3, 6, 19, 0, 0, 0, time.UTC)

// openTestStore returns a Store backed by a fresh in-memory SQLite database.
func openTestStore(t *testing.T) *reservation.Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1) // each connection to :memory: is its own database
	t.Cleanup(func() { db.Close() })
	if err := reservation.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return reservation.NewStore(db)
}

// newBooking builds a valid reservation for the given table and start time.
func newBooking(table int, startsAt time.Time) *reservation.Reservation {
	return &reservation.Reservation{
		GuestName:   "Test Guest",
		PartySize:   2,
		TableNumber: table,
		StartsAt:    startsAt,
		CreatedAt:   storeBase.Add(-24 * time.Hour),
	}
}

func TestStoreCreateConflictWindow(t *testing.T) {
	tests := []struct {
		name    string
		table   int
		offset  time.Duration // relative to the seeded booking on table 7
		wantErr error
	}{
		{"same instant conflicts", 7, 0, reservation.ErrConflict},
		{"119m59s later conflicts", 7, 119*time.Minute + 59*time.Second, reservation.ErrConflict},
		{"119m59s earlier conflicts", 7, -(119*time.Minute + 59*time.Second), reservation.ErrConflict},
		{"exactly 2h later is allowed", 7, 2 * time.Hour, nil},
		{"exactly 2h earlier is allowed", 7, -2 * time.Hour, nil},
		{"other table at same time is allowed", 8, 0, nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			s := openTestStore(t)
			if err := s.Create(ctx, newBooking(7, storeBase)); err != nil {
				t.Fatalf("seed: %v", err)
			}

			err := s.Create(ctx, newBooking(tc.table, storeBase.Add(tc.offset)))
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Create error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestStoreUpdate(t *testing.T) {
	tests := []struct {
		name    string
		missing bool          // update an id that does not exist
		table   int           // table to move the first seeded booking to
		offset  time.Duration // relative to storeBase
		wantErr error
	}{
		{"moving own row 30m ignores itself", false, 1, 30 * time.Minute, nil},
		{"same slot unchanged ignores itself", false, 1, 0, nil},
		{"moving onto another booking conflicts", false, 2, 30 * time.Minute, reservation.ErrConflict},
		{"exactly 2h from another booking is allowed", false, 2, 2 * time.Hour, nil},
		{"missing id", true, 1, 0, reservation.ErrNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			s := openTestStore(t)
			a := newBooking(1, storeBase)
			b := newBooking(2, storeBase)
			for _, r := range []*reservation.Reservation{a, b} {
				if err := s.Create(ctx, r); err != nil {
					t.Fatalf("seed: %v", err)
				}
			}

			upd := newBooking(tc.table, storeBase.Add(tc.offset))
			upd.ID = a.ID
			if tc.missing {
				upd.ID = 9999
			}
			upd.CreatedAt = time.Time{} // Update must fill this from the stored row

			err := s.Update(ctx, upd)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Update error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				return
			}
			if !upd.CreatedAt.Equal(a.CreatedAt) {
				t.Errorf("CreatedAt = %v, want %v", upd.CreatedAt, a.CreatedAt)
			}
			got, err := s.Get(ctx, upd.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.TableNumber != tc.table || !got.StartsAt.Equal(upd.StartsAt) {
				t.Errorf("stored row = table %d at %v, want table %d at %v",
					got.TableNumber, got.StartsAt, tc.table, upd.StartsAt)
			}
		})
	}
}

func TestStoreDelete(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	r := newBooking(1, storeBase)
	if err := s.Create(ctx, r); err != nil {
		t.Fatal(err)
	}

	steps := []struct {
		name    string
		id      int64
		wantErr error
	}{
		{"missing id", 9999, reservation.ErrNotFound},
		{"existing id", r.ID, nil},
		{"same id again", r.ID, reservation.ErrNotFound},
	}
	for _, step := range steps {
		if err := s.Delete(ctx, step.id); !errors.Is(err, step.wantErr) {
			t.Fatalf("%s: Delete error = %v, want %v", step.name, err, step.wantErr)
		}
	}
	if _, err := s.Get(ctx, r.ID); !errors.Is(err, reservation.ErrNotFound) {
		t.Errorf("Get after delete error = %v, want ErrNotFound", err)
	}
}

func TestStoreListDateFilter(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	day := time.Date(2027, 3, 6, 0, 0, 0, 0, time.UTC)
	// Distinct tables so the seeds never conflict with each other.
	seeds := []struct {
		table int
		at    time.Time
	}{
		{1, day.Add(-time.Second)},               // last second of the day before
		{2, day},                                 // first second of the day
		{3, day.Add(24*time.Hour - time.Second)}, // last second of the day
		{4, day.Add(24 * time.Hour)},             // first second of the day after
	}
	for _, seed := range seeds {
		if err := s.Create(ctx, newBooking(seed.table, seed.at)); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	nextDay := day.Add(24 * time.Hour)
	emptyDay := time.Date(2027, 12, 25, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		day        *time.Time
		wantTables []int // in start order
	}{
		{"no filter returns all", nil, []int{1, 2, 3, 4}},
		{"day includes both edges only", &day, []int{2, 3}},
		{"next day", &nextDay, []int{4}},
		{"empty day", &emptyDay, []int{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := s.List(ctx, tc.day)
			if err != nil {
				t.Fatal(err)
			}
			if got == nil {
				t.Fatal("List returned nil, want a non-nil slice")
			}
			if len(got) != len(tc.wantTables) {
				t.Fatalf("got %d reservations, want %d", len(got), len(tc.wantTables))
			}
			for i, table := range tc.wantTables {
				if got[i].TableNumber != table {
					t.Errorf("item %d on table %d, want table %d", i, got[i].TableNumber, table)
				}
			}
		})
	}
}
