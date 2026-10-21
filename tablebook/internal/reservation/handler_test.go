package reservation_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/d4b0mb/tablebook/internal/reservation"
)

var fixedNow = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

func newTestServer(t *testing.T) http.Handler {
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
	h := reservation.NewHandler(reservation.NewStore(db), func() time.Time { return fixedNow })
	return h.Routes()
}

func body(name string, party, table int, startsAt string) string {
	return fmt.Sprintf(`{"guest_name":%q,"party_size":%d,"table_number":%d,"starts_at":%q}`,
		name, party, table, startsAt)
}

func do(h http.Handler, method, path, reqBody string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(reqBody))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func errorMessage(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var e struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("response is not a JSON error: %q", rec.Body.String())
	}
	return e.Error
}

func TestCreate(t *testing.T) {
	const evening = "2026-11-07T19:00:00Z"

	tests := []struct {
		name       string
		seed       []string // bodies POSTed first
		body       string
		wantStatus int
		wantError  string // substring of the error message
	}{
		{
			name:       "valid",
			body:       body("Maria Rossi", 4, 7, evening),
			wantStatus: http.StatusCreated,
		},
		{
			name:       "guest name too short",
			body:       body("A", 4, 7, evening),
			wantStatus: http.StatusBadRequest,
			wantError:  "guest_name must be 2-60",
		},
		{
			name:       "party size too large",
			body:       body("Maria Rossi", 21, 7, evening),
			wantStatus: http.StatusBadRequest,
			wantError:  "party_size must be 1-20",
		},
		{
			name:       "table number out of range",
			body:       body("Maria Rossi", 4, 31, evening),
			wantStatus: http.StatusBadRequest,
			wantError:  "table_number must be 1-30",
		},
		{
			name:       "start in the past",
			body:       body("Maria Rossi", 4, 7, "2026-10-01T19:00:00Z"),
			wantStatus: http.StatusBadRequest,
			wantError:  "starts_at must be in the future",
		},
		{
			name:       "notes too long",
			body:       fmt.Sprintf(`{"guest_name":"Maria Rossi","party_size":2,"table_number":1,"starts_at":%q,"notes":%q}`, evening, strings.Repeat("x", 201)),
			wantStatus: http.StatusBadRequest,
			wantError:  "notes must be at most 200",
		},
		{
			name:       "malformed JSON",
			body:       `{"guest_name":`,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid JSON",
		},
		{
			name:       "bad timestamp",
			body:       `{"guest_name":"Maria Rossi","party_size":2,"table_number":1,"starts_at":"tomorrow"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid JSON",
		},
		{
			name:       "same table 90 minutes later conflicts",
			seed:       []string{body("Luca Bianchi", 2, 7, evening)},
			body:       body("Maria Rossi", 4, 7, "2026-11-07T20:30:00Z"),
			wantStatus: http.StatusConflict,
			wantError:  "already booked",
		},
		{
			name:       "same table 90 minutes earlier conflicts",
			seed:       []string{body("Luca Bianchi", 2, 7, evening)},
			body:       body("Maria Rossi", 4, 7, "2026-11-07T17:30:00Z"),
			wantStatus: http.StatusConflict,
			wantError:  "already booked",
		},
		{
			name:       "same table exactly 2 hours later is allowed",
			seed:       []string{body("Luca Bianchi", 2, 7, evening)},
			body:       body("Maria Rossi", 4, 7, "2026-11-07T21:00:00Z"),
			wantStatus: http.StatusCreated,
		},
		{
			name:       "different table at same time is allowed",
			seed:       []string{body("Luca Bianchi", 2, 7, evening)},
			body:       body("Maria Rossi", 4, 8, evening),
			wantStatus: http.StatusCreated,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestServer(t)
			for _, s := range tc.seed {
				if rec := do(h, "POST", "/reservations", s); rec.Code != http.StatusCreated {
					t.Fatalf("seed failed: %d %s", rec.Code, rec.Body)
				}
			}

			rec := do(h, "POST", "/reservations", tc.body)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tc.wantStatus, rec.Body)
			}
			if tc.wantError != "" {
				if msg := errorMessage(t, rec); !strings.Contains(msg, tc.wantError) {
					t.Errorf("error = %q, want it to contain %q", msg, tc.wantError)
				}
			}
			if tc.wantStatus == http.StatusCreated {
				var got reservation.Reservation
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if got.ID == 0 {
					t.Error("created reservation has no id")
				}
				if want := fmt.Sprintf("/reservations/%d", got.ID); rec.Header().Get("Location") != want {
					t.Errorf("Location = %q, want %q", rec.Header().Get("Location"), want)
				}
				if !got.CreatedAt.Equal(fixedNow) {
					t.Errorf("created_at = %v, want %v", got.CreatedAt, fixedNow)
				}
			}
		})
	}
}

func TestGetAndList(t *testing.T) {
	h := newTestServer(t)
	do(h, "POST", "/reservations", body("Maria Rossi", 4, 1, "2026-11-07T19:00:00Z"))
	do(h, "POST", "/reservations", body("Luca Bianchi", 2, 2, "2026-11-07T12:00:00Z"))
	do(h, "POST", "/reservations", body("Anna Verdi", 6, 1, "2026-11-08T19:00:00Z"))

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantNames  []string // for list endpoints, in order
	}{
		{"get existing", "/reservations/1", http.StatusOK, nil},
		{"get missing", "/reservations/99", http.StatusNotFound, nil},
		{"get non-numeric id", "/reservations/abc", http.StatusBadRequest, nil},
		{"list all ordered by start", "/reservations", http.StatusOK, []string{"Luca Bianchi", "Maria Rossi", "Anna Verdi"}},
		{"list filtered by date", "/reservations?date=2026-11-07", http.StatusOK, []string{"Luca Bianchi", "Maria Rossi"}},
		{"list empty day", "/reservations?date=2026-12-25", http.StatusOK, []string{}},
		{"list bad date", "/reservations?date=07-11-2026", http.StatusBadRequest, nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(h, "GET", tc.path, "")
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tc.wantStatus, rec.Body)
			}
			if tc.wantNames == nil {
				return
			}
			var got []reservation.Reservation
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.wantNames) {
				t.Fatalf("got %d reservations, want %d", len(got), len(tc.wantNames))
			}
			for i, name := range tc.wantNames {
				if got[i].GuestName != name {
					t.Errorf("item %d = %q, want %q", i, got[i].GuestName, name)
				}
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		body       string
		wantStatus int
	}{
		{"valid", "/reservations/1", body("Maria Rossi", 5, 1, "2026-11-07T19:30:00Z"), http.StatusOK},
		{"invalid input", "/reservations/1", body("Maria Rossi", 0, 1, "2026-11-07T19:00:00Z"), http.StatusBadRequest},
		{"missing id", "/reservations/99", body("Maria Rossi", 4, 1, "2026-11-07T19:00:00Z"), http.StatusNotFound},
		{"clashes with another booking", "/reservations/1", body("Maria Rossi", 4, 2, "2026-11-07T19:30:00Z"), http.StatusConflict},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestServer(t)
			do(h, "POST", "/reservations", body("Maria Rossi", 4, 1, "2026-11-07T19:00:00Z"))
			do(h, "POST", "/reservations", body("Luca Bianchi", 2, 2, "2026-11-07T19:00:00Z"))

			rec := do(h, "PUT", tc.path, tc.body)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tc.wantStatus, rec.Body)
			}
			if tc.wantStatus == http.StatusOK {
				var got reservation.Reservation
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if got.ID != 1 || got.PartySize != 5 || !got.CreatedAt.Equal(fixedNow) {
					t.Errorf("unexpected updated reservation: %+v", got)
				}
			}
		})
	}
}

func TestUpdateIgnoresOwnBooking(t *testing.T) {
	h := newTestServer(t)
	do(h, "POST", "/reservations", body("Maria Rossi", 4, 1, "2026-11-07T19:00:00Z"))

	// Moving a booking 30 minutes must not conflict with its own old slot.
	rec := do(h, "PUT", "/reservations/1", body("Maria Rossi", 4, 1, "2026-11-07T19:30:00Z"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body)
	}
}

func TestDelete(t *testing.T) {
	h := newTestServer(t)
	do(h, "POST", "/reservations", body("Maria Rossi", 4, 1, "2026-11-07T19:00:00Z"))

	steps := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{"delete existing", "DELETE", "/reservations/1", http.StatusNoContent},
		{"gone afterwards", "GET", "/reservations/1", http.StatusNotFound},
		{"delete again", "DELETE", "/reservations/1", http.StatusNotFound},
	}
	for _, s := range steps {
		if rec := do(h, s.method, s.path, ""); rec.Code != s.wantStatus {
			t.Fatalf("%s: status = %d, want %d", s.name, rec.Code, s.wantStatus)
		}
	}

	// A cancelled slot can be booked again.
	rec := do(h, "POST", "/reservations", body("Luca Bianchi", 2, 1, "2026-11-07T19:00:00Z"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("rebooking status = %d, want 201", rec.Code)
	}
}

func TestHealth(t *testing.T) {
	rec := do(newTestServer(t), "GET", "/health", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}
