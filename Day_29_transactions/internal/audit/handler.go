package audit

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"day29/internal/auth"
	"day29/internal/respond"
)

type Handler struct {
	Repo Repository
}

type ListResponse struct {
	Data []Entry `json:"data"`
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

	limit := defaultLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxLimit {
			respond.BadRequest(w, "limit must be an integer 1..200")
			return
		}
		limit = n
	}

	entries, err := h.Repo.List(r.Context(), userID, limit)
	if err != nil {
		respond.Internal(r.Context(), w, err)
		return
	}
	respond.JSON(w, http.StatusOK, ListResponse{Data: entries})
}
