package todo

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"day15/internal/respond"
	"day15/internal/validate"
)

type Handler struct {
	Svc *Service
}

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

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	filter, ok := parseListFilter(w, r)
	if !ok {
		return
	}
	todos, err := h.Svc.List(r.Context(), filter)
	if err != nil {
		writeServiceErr(w, err)
		return
	}
	respond.JSON(w, http.StatusOK, todos)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	t, err := h.Svc.Get(r.Context(), id)
	if err != nil {
		writeServiceErr(w, err)
		return
	}
	respond.JSON(w, http.StatusOK, t)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in CreateRequest
	if !decodeJSON(w, r, &in) {
		return
	}
	// Format validation at the boundary. Field-level 422 on failure.
	if fields := validate.Struct(in); fields != nil {
		respond.ValidationFailed(w, fields)
		return
	}
	t, err := h.Svc.Create(r.Context(), in)
	if err != nil {
		writeServiceErr(w, err)
		return
	}
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
	if fields := validate.Struct(in); fields != nil {
		respond.ValidationFailed(w, fields)
		return
	}
	t, err := h.Svc.Update(r.Context(), id, in)
	if err != nil {
		writeServiceErr(w, err)
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
	if fields := validate.Struct(in); fields != nil {
		respond.ValidationFailed(w, fields)
		return
	}
	t, err := h.Svc.Patch(r.Context(), id, in)
	if err != nil {
		writeServiceErr(w, err)
		return
	}
	respond.JSON(w, http.StatusOK, t)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.Svc.Delete(r.Context(), id); err != nil {
		writeServiceErr(w, err)
		return
	}
	respond.NoContent(w)
}

// writeServiceErr maps domain errors to HTTP codes. Validation errors are
// handled separately (via respond.ValidationFailed) before the service runs.
func writeServiceErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		respond.NotFound(w, err.Error())
	case errors.Is(err, ErrNothingToUpdate):
		respond.BadRequest(w, err.Error())
	default:
		respond.Internal(w, err)
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
