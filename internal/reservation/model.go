package reservation

import (
	"strings"
	"time"
	"unicode/utf8"
)

// MinGap is the minimum time between two bookings on the same table.
const MinGap = 2 * time.Hour

// Reservation is a table booking.
type Reservation struct {
	ID          int64     `json:"id"`
	GuestName   string    `json:"guest_name"`
	PartySize   int       `json:"party_size"`
	TableNumber int       `json:"table_number"`
	StartsAt    time.Time `json:"starts_at"`
	Notes       string    `json:"notes"`
	CreatedAt   time.Time `json:"created_at"`
}

// ValidationError describes invalid client input.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

func invalid(msg string) error { return &ValidationError{Message: msg} }

// Validate checks the client-supplied fields and normalises them in place.
// now is passed in so that the "must be in the future" rule is testable.
func (r *Reservation) Validate(now time.Time) error {
	r.GuestName = strings.TrimSpace(r.GuestName)
	if n := utf8.RuneCountInString(r.GuestName); n < 2 || n > 60 {
		return invalid("guest_name must be 2-60 characters")
	}
	if r.PartySize < 1 || r.PartySize > 20 {
		return invalid("party_size must be 1-20")
	}
	if r.TableNumber < 1 || r.TableNumber > 30 {
		return invalid("table_number must be 1-30")
	}
	if r.StartsAt.IsZero() {
		return invalid("starts_at is required (RFC 3339)")
	}
	if !r.StartsAt.After(now) {
		return invalid("starts_at must be in the future")
	}
	if utf8.RuneCountInString(r.Notes) > 200 {
		return invalid("notes must be at most 200 characters")
	}
	return nil
}
