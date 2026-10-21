package reservation

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/d4b0mb/tablebook/migrations"
)

var (
	// ErrNotFound is returned when a reservation id does not exist.
	ErrNotFound = errors.New("reservation not found")
	// ErrConflict is returned when the table is already booked too close to the requested time.
	ErrConflict = errors.New("table is already booked within 2 hours of that time")
)

// Store persists reservations in SQL.
type Store struct {
	db *sql.DB
}

// NewStore wraps an open database.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// Migrate creates the schema if it does not exist yet.
func Migrate(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, migrations.Init)
	return err
}

const columns = "id, guest_name, party_size, table_number, starts_at, notes, created_at"

type scanner interface{ Scan(dest ...any) error }

func scan(s scanner) (*Reservation, error) {
	var r Reservation
	var starts, created int64
	if err := s.Scan(&r.ID, &r.GuestName, &r.PartySize, &r.TableNumber, &starts, &r.Notes, &created); err != nil {
		return nil, err
	}
	r.StartsAt = time.Unix(starts, 0).UTC()
	r.CreatedAt = time.Unix(created, 0).UTC()
	return &r, nil
}

// hasConflict reports whether another booking on the same table starts less
// than MinGap from r. excludeID lets an update ignore its own row.
func hasConflict(ctx context.Context, tx *sql.Tx, r *Reservation, excludeID int64) (bool, error) {
	gap := int64(MinGap / time.Second)
	start := r.StartsAt.Unix()
	var exists bool
	err := tx.QueryRowContext(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM reservations
			WHERE table_number = ? AND id != ? AND starts_at > ? AND starts_at < ?
		)`, r.TableNumber, excludeID, start-gap, start+gap).Scan(&exists)
	return exists, err
}

// Create inserts r, setting r.ID. It returns ErrConflict on a double-booking.
func (s *Store) Create(ctx context.Context, r *Reservation) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if conflict, err := hasConflict(ctx, tx, r, 0); err != nil {
		return err
	} else if conflict {
		return ErrConflict
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO reservations (guest_name, party_size, table_number, starts_at, notes, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		r.GuestName, r.PartySize, r.TableNumber, r.StartsAt.Unix(), r.Notes, r.CreatedAt.Unix())
	if err != nil {
		return err
	}
	if r.ID, err = res.LastInsertId(); err != nil {
		return err
	}
	return tx.Commit()
}

// Get returns one reservation or ErrNotFound.
func (s *Store) Get(ctx context.Context, id int64) (*Reservation, error) {
	r, err := scan(s.db.QueryRowContext(ctx, "SELECT "+columns+" FROM reservations WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return r, err
}

// List returns reservations ordered by start time. If day is non-nil only
// bookings starting on that UTC calendar day are returned.
func (s *Store) List(ctx context.Context, day *time.Time) ([]Reservation, error) {
	query := "SELECT " + columns + " FROM reservations"
	var args []any
	if day != nil {
		from := day.UTC().Truncate(24 * time.Hour)
		query += " WHERE starts_at >= ? AND starts_at < ?"
		args = append(args, from.Unix(), from.Add(24*time.Hour).Unix())
	}
	query += " ORDER BY starts_at, id"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Reservation{}
	for rows.Next() {
		r, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// Update replaces the editable fields of r.ID. It returns ErrNotFound or ErrConflict.
// r.CreatedAt is filled in from the stored row.
func (s *Store) Update(ctx context.Context, r *Reservation) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var created int64
	err = tx.QueryRowContext(ctx, "SELECT created_at FROM reservations WHERE id = ?", r.ID).Scan(&created)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if conflict, err := hasConflict(ctx, tx, r, r.ID); err != nil {
		return err
	} else if conflict {
		return ErrConflict
	}
	_, err = tx.ExecContext(ctx,
		`UPDATE reservations
		 SET guest_name = ?, party_size = ?, table_number = ?, starts_at = ?, notes = ?
		 WHERE id = ?`,
		r.GuestName, r.PartySize, r.TableNumber, r.StartsAt.Unix(), r.Notes, r.ID)
	if err != nil {
		return err
	}
	r.CreatedAt = time.Unix(created, 0).UTC()
	return tx.Commit()
}

// Delete removes a reservation or returns ErrNotFound.
func (s *Store) Delete(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM reservations WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
