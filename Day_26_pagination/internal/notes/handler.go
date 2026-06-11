package notes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"day26/internal/auth"
	"day26/internal/respond"
	"day26/internal/validate"
)

type Handler struct {
	Svc *Service
}

// ListResponse — wrapper around the page so we can add metadata without
// breaking the response shape later.
type ListResponse struct {
	Data       []Note `json:"data"`
	NextCursor string `json:"next_cursor,omitempty"`
}

const (
	defaultLimit = 20
	maxLimit     = 100
)

func (h *Handler) Router() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Get("/{id}", h.get)
	r.Put("/{id}", h.update)
	r.Patch("/{id}", h.patch)
	r.Delete("/{id}", h.delete)
	return r
}

func userIDFromCtx(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, ok := auth.GetUserID(r.Context())
	if !ok {
		respond.Unauthorized(w, "not authenticated")
		return 0, false
	}
	return id, true
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(w, r)
	if !ok {
		return
	}
	filter, ok := parseListFilter(w, r)
	if !ok {
		return
	}
	page, err := h.Svc.List(r.Context(), userID, filter)
	if err != nil {
		writeServiceErr(r.Context(), w, err)
		return
	}
	respond.JSON(w, http.StatusOK, ListResponse{
		Data:       page.Items,
		NextCursor: encodeCursor(page.NextID),
	})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	n, err := h.Svc.Get(r.Context(), userID, id)
	if err != nil {
		writeServiceErr(r.Context(), w, err)
		return
	}
	respond.JSON(w, http.StatusOK, n)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(w, r)
	if !ok {
		return
	}
	var in CreateRequest
	if !decodeJSON(w, r, &in) {
		return
	}
	if fields := validate.Struct(in); fields != nil {
		respond.ValidationFailed(w, fields)
		return
	}
	n, err := h.Svc.Create(r.Context(), userID, in)
	if err != nil {
		writeServiceErr(r.Context(), w, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/notes/%d", n.ID))
	respond.JSON(w, http.StatusCreated, n)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var in UpdateRequest
	if !decodeJSON(w, r, &in) {
		return
	}
	if fields := validate.Struct(in); fields != nil {
		respond.ValidationFailed(w, fields)
		return
	}
	n, err := h.Svc.Update(r.Context(), userID, id, in)
	if err != nil {
		writeServiceErr(r.Context(), w, err)
		return
	}
	respond.JSON(w, http.StatusOK, n)
}

func (h *Handler) patch(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var in PatchRequest
	if !decodeJSON(w, r, &in) {
		return
	}
	if fields := validate.Struct(in); fields != nil {
		respond.ValidationFailed(w, fields)
		return
	}
	n, err := h.Svc.Patch(r.Context(), userID, id, in)
	if err != nil {
		writeServiceErr(r.Context(), w, err)
		return
	}
	respond.JSON(w, http.StatusOK, n)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.Svc.Delete(r.Context(), userID, id); err != nil {
		writeServiceErr(r.Context(), w, err)
		return
	}
	respond.NoContent(w)
}

func writeServiceErr(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		respond.NotFound(w, err.Error())
	case errors.Is(err, ErrNothingToUpdate), errors.Is(err, ErrInvalidCursor):
		respond.BadRequest(w, err.Error())
	default:
		respond.Internal(ctx, w, err)
	}
}

func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		respond.BadRequest(w, "id must be a positive integer")
		return 0, false
	}
	return id, true
}

// parseListFilter — Day 26 handles ?search, ?limit, ?after (opaque cursor),
// and ?sort (desc|asc, default desc).
func parseListFilter(w http.ResponseWriter, r *http.Request) (ListFilter, bool) {
	q := r.URL.Query()
	f := ListFilter{Limit: defaultLimit, SortDesc: true}

	f.Search = q.Get("search")

	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxLimit {
			respond.BadRequest(w, fmt.Sprintf("limit must be an integer 1..%d", maxLimit))
			return f, false
		}
		f.Limit = n
	}

	if v := q.Get("after"); v != "" {
		id, err := decodeCursor(v)
		if err != nil {
			respond.BadRequest(w, "invalid after cursor")
			return f, false
		}
		f.AfterID = id
	}

	switch q.Get("sort") {
	case "", "desc":
		f.SortDesc = true
	case "asc":
		f.SortDesc = false
	default:
		respond.BadRequest(w, "sort must be 'asc' or 'desc'")
		return f, false
	}

	return f, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if ct := r.Header.Get("Content-Type"); ct != "" && ct != "application/json" {
		respond.UnsupportedMediaType(w, "Content-Type must be application/json")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		respond.BadRequest(w, decodeErrorMessage(err))
		return false
	}
	if dec.More() {
		respond.BadRequest(w, "body must contain a single JSON object")
		return false
	}
	return true
}

func decodeErrorMessage(err error) string {
	var (
		syntaxErr    *json.SyntaxError
		unmarshalErr *json.UnmarshalTypeError
		maxBytesErr  *http.MaxBytesError
	)
	switch {
	case errors.As(err, &syntaxErr):
		return fmt.Sprintf("malformed JSON at byte %d", syntaxErr.Offset)
	case errors.As(err, &unmarshalErr):
		return fmt.Sprintf("field %q has wrong type", unmarshalErr.Field)
	case errors.As(err, &maxBytesErr):
		return "request body too large"
	default:
		return "invalid JSON request body"
	}
}
