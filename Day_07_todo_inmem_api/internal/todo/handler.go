package todo

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"day07/internal/respond"
)

// Handler bundles the HTTP handlers for /todos.
// It holds its dependency (Store) so main.go can inject one.
//
// On Day 22 you'll mock Store with an interface to unit-test these handlers.
type Handler struct {
	Store *Store
}

// Router returns a chi sub-router that handles all /todos routes.
// main.go mounts it at /todos.
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

// ---- Handlers ---------------------------------------------------------

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	filter, ok := parseListFilter(w, r)
	if !ok {
		return
	}
	respond.JSON(w, http.StatusOK, h.Store.List(filter))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	t, ok := h.Store.Get(id)
	if !ok {
		respond.NotFound(w, "todo not found")
		return
	}
	respond.JSON(w, http.StatusOK, t)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in CreateRequest
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Title == "" {
		respond.BadRequest(w, "title is required")
		return
	}
	t := h.Store.Create(in)
	w.Header().Set("Location", fmt.Sprintf("/todos/%d", t.ID))
	respond.JSON(w, http.StatusCreated, t)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var in UpdateRequest
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Title == "" {
		respond.BadRequest(w, "title is required")
		return
	}
	t, ok := h.Store.Update(id, in)
	if !ok {
		respond.NotFound(w, "todo not found")
		return
	}
	respond.JSON(w, http.StatusOK, t)
}

func (h *Handler) patch(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var in PatchRequest
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Title == nil && in.Done == nil {
		respond.BadRequest(w, "nothing to update")
		return
	}
	if in.Title != nil && *in.Title == "" {
		respond.BadRequest(w, "title cannot be empty")
		return
	}
	t, ok := h.Store.Patch(id, in)
	if !ok {
		respond.NotFound(w, "todo not found")
		return
	}
	respond.JSON(w, http.StatusOK, t)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if !h.Store.Delete(id) {
		respond.NotFound(w, "todo not found")
		return
	}
	respond.NoContent(w)
}

// ---- Small helpers ----------------------------------------------------

func parseID(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		respond.BadRequest(w, "id must be a positive integer")
		return 0, false
	}
	return id, true
}

// parseListFilter reads ?done=true&search=...&limit=10 from the query string.
func parseListFilter(w http.ResponseWriter, r *http.Request) (ListFilter, bool) {
	var f ListFilter
	q := r.URL.Query()

	if v := q.Get("done"); v != "" {
		switch v {
		case "true":
			t := true
			f.Done = &t
		case "false":
			fb := false
			f.Done = &fb
		default:
			respond.BadRequest(w, "done must be true or false")
			return f, false
		}
	}

	f.Search = q.Get("search")

	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 100 {
			respond.BadRequest(w, "limit must be an integer 1..100")
			return f, false
		}
		f.Limit = n
	}
	return f, true
}

// decodeJSON is the Day 3/4 strict-decode pipeline, factored out.
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
