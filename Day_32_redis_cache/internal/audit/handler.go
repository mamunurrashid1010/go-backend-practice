package audit

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"day32/internal/auth"
	"day32/internal/respond"
)

type Handler struct {
	Repo Repository
}

type ListResponse struct {
	Data []Entry `json:"data"`
}
type ListWithNotesResponse struct {
	Data     []EntryWithNote `json:"data"`
	Strategy string          `json:"strategy"`
}

const (
	defaultLimit = 50
	maxLimit     = 200
)

func (h *Handler) Router() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.list)
	return r
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.GetUserID(r.Context())
	if !ok {
		respond.Unauthorized(w, "not authenticated")
		return
	}
	limit, ok := parseLimit(w, r)
	if !ok {
		return
	}
	if r.URL.Query().Get("include") != "notes" {
		entries, err := h.Repo.List(r.Context(), userID, limit)
		if err != nil {
			respond.Internal(r.Context(), w, err)
			return
		}
		respond.JSON(w, http.StatusOK, ListResponse{Data: entries})
		return
	}

	strategy := r.URL.Query().Get("strategy")
	if strategy == "" {
		strategy = "join"
	}
	fn, ok := strategyFn(h.Repo, strategy)
	if !ok {
		respond.BadRequest(w, "strategy must be join|in_batch|naive")
		return
	}
	entries, err := fn(r.Context(), userID, limit)
	if err != nil {
		respond.Internal(r.Context(), w, err)
		return
	}
	respond.JSON(w, http.StatusOK, ListWithNotesResponse{Data: entries, Strategy: strategy})
}

type withNotesFn func(ctx context.Context, userID int64, limit int) ([]EntryWithNote, error)

func strategyFn(repo Repository, name string) (withNotesFn, bool) {
	switch name {
	case "join":
		return repo.ListWithNotesJoin, true
	case "in_batch":
		return repo.ListWithNotesInBatch, true
	case "naive":
		return repo.ListWithNotesNaive, true
	default:
		return nil, false
	}
}

func parseLimit(w http.ResponseWriter, r *http.Request) (int, bool) {
	v := r.URL.Query().Get("limit")
	if v == "" {
		return defaultLimit, true
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > maxLimit {
		respond.BadRequest(w, "limit must be an integer 1..200")
		return 0, false
	}
	return n, true
}
