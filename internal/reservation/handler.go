package reservation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
)

// Repository is what the handlers need from storage.
type Repository interface {
	Create(ctx context.Context, r *Reservation) error
	Get(ctx context.Context, id int64) (*Reservation, error)
	List(ctx context.Context, day *time.Time) ([]Reservation, error)
	Update(ctx context.Context, r *Reservation) error
	Delete(ctx context.Context, id int64) error
}

// Handler serves the reservation API.
type Handler struct {
	repo Repository
	now  func() time.Time
}

// NewHandler builds a Handler. Pass nil for now to use the wall clock.
func NewHandler(repo Repository, now func() time.Time) *Handler {
	if now == nil {
		now = time.Now
	}
	return &Handler{repo: repo, now: now}
}

// Routes returns the HTTP routes of the API.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", h.health)
	mux.HandleFunc("GET /reservations", h.list)
	mux.HandleFunc("GET /reservations/{id}", h.get)
	mux.HandleFunc("POST /reservations", h.create)
	mux.HandleFunc("PUT /reservations/{id}", h.update)
	mux.HandleFunc("DELETE /reservations/{id}", h.delete)
	return mux
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	var day *time.Time
	if s := r.URL.Query().Get("date"); s != "" {
		d, err := time.Parse("2006-01-02", s)
		if err != nil {
			writeError(w, invalid("date must be YYYY-MM-DD"))
			return
		}
		day = &d
	}
	items, err := h.repo.List(r.Context(), day)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	res, err := h.repo.Get(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	res, err := h.decodeAndValidate(w, r)
	if err != nil {
		writeError(w, err)
		return
	}
	res.CreatedAt = h.now().UTC().Truncate(time.Second)
	if err := h.repo.Create(r.Context(), res); err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/reservations/%d", res.ID))
	writeJSON(w, http.StatusCreated, res)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	res, err := h.decodeAndValidate(w, r)
	if err != nil {
		writeError(w, err)
		return
	}
	res.ID = id
	if err := h.repo.Update(r.Context(), res); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := h.repo.Delete(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// decodeAndValidate reads a JSON body into a Reservation and validates it.
// Server-owned fields (id, created_at) are discarded.
func (h *Handler) decodeAndValidate(w http.ResponseWriter, r *http.Request) (*Reservation, error) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var res Reservation
	if err := dec.Decode(&res); err != nil {
		return nil, invalid("invalid JSON body: " + err.Error())
	}
	res.ID = 0
	res.CreatedAt = time.Time{}
	if err := res.Validate(h.now()); err != nil {
		return nil, err
	}
	res.StartsAt = res.StartsAt.UTC().Truncate(time.Second)
	return &res, nil
}

func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, invalid("id must be a positive integer")
	}
	return id, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}

// writeError maps domain errors to HTTP status codes.
func writeError(w http.ResponseWriter, err error) {
	var verr *ValidationError
	switch {
	case errors.As(err, &verr):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": verr.Message})
	case errors.Is(err, ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	default:
		log.Printf("internal error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
}
