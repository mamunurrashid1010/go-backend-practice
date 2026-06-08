package notes

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"day23/internal/auth"
)

// ---------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------

// newAuthedRequest creates a request whose context has the given userID,
// bypassing RequireAuth. We're testing the handler, not the middleware.
func newAuthedRequest(method, target, body string, userID int64) *http.Request {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	ctx := auth.WithUserID(r.Context(), userID)
	return r.WithContext(ctx)
}

// readBody decodes a JSON response body into a map for ad-hoc inspection.
func readBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v (body: %s)", err, rec.Body.String())
	}
	return out
}

// errorCode pulls error.code out of the JSON envelope: {"error":{"code":"..."}}.
func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	body := readBody(t, rec)
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("response has no .error object: %s", rec.Body.String())
	}
	code, _ := errObj["code"].(string)
	return code
}

// newHandler builds a real *Handler whose service is backed by the given
// fakeRepository. One line per test.
func newHandler(repo *fakeRepository) *Handler {
	return &Handler{Svc: NewService(repo)}
}

// ---------------------------------------------------------------------
// GET /notes/{id}
// ---------------------------------------------------------------------

func TestHandler_Get_OK(t *testing.T) {
	repo := &fakeRepository{GetNote: Note{ID: 7, UserID: 1, Title: "hello"}}
	router := newHandler(repo).Router()

	rec := httptest.NewRecorder()
	req := newAuthedRequest("GET", "/7", "", 1)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if repo.LastGetUserID != 1 || repo.LastGetID != 7 {
		t.Errorf("repo args: want (1, 7), got (%d, %d)", repo.LastGetUserID, repo.LastGetID)
	}
	body := readBody(t, rec)
	if int(body["id"].(float64)) != 7 {
		t.Errorf("id: want 7, got %v", body["id"])
	}
}

func TestHandler_Get_NotFound(t *testing.T) {
	repo := &fakeRepository{GetErr: ErrNotFound}
	router := newHandler(repo).Router()

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newAuthedRequest("GET", "/99", "", 1))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: want 404, got %d", rec.Code)
	}
	if got := errorCode(t, rec); got != "NOT_FOUND" {
		t.Errorf("error code: want NOT_FOUND, got %q", got)
	}
}

func TestHandler_Get_BadID(t *testing.T) {
	router := newHandler(&fakeRepository{}).Router()

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newAuthedRequest("GET", "/abc", "", 1))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: want 400, got %d", rec.Code)
	}
}

func TestHandler_Get_Unauthenticated(t *testing.T) {
	router := newHandler(&fakeRepository{}).Router()

	rec := httptest.NewRecorder()
	// Bare httptest.NewRequest — no auth.WithUserID on the context.
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/7", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: want 401, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------
// GET /notes (list)
// ---------------------------------------------------------------------

func TestHandler_List_OK(t *testing.T) {
	repo := &fakeRepository{ListNotes: []Note{{ID: 1, Title: "a"}, {ID: 2, Title: "b"}}}
	router := newHandler(repo).Router()

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newAuthedRequest("GET", "/?search=he&limit=10", "", 1))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	if repo.LastListFilter.Search != "he" || repo.LastListFilter.Limit != 10 {
		t.Errorf("filter not propagated: %+v", repo.LastListFilter)
	}
}

func TestHandler_List_BadLimit(t *testing.T) {
	router := newHandler(&fakeRepository{}).Router()

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newAuthedRequest("GET", "/?limit=999", "", 1))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: want 400, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------
// POST /notes
// ---------------------------------------------------------------------

func TestHandler_Create_201(t *testing.T) {
	repo := &fakeRepository{CreateNote: Note{ID: 1, UserID: 1, Title: "x"}}
	router := newHandler(repo).Router()

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newAuthedRequest("POST", "/", `{"title":"x","body":"b"}`, 1))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status: want 201, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/notes/1" {
		t.Errorf("Location header: want /notes/1, got %q", loc)
	}
}

func TestHandler_Create_ValidationFails(t *testing.T) {
	router := newHandler(&fakeRepository{}).Router()

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newAuthedRequest("POST", "/", `{"title":""}`, 1))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status: want 422, got %d", rec.Code)
	}
	if got := errorCode(t, rec); got != "VALIDATION" {
		t.Errorf("error code: want VALIDATION, got %q", got)
	}
}

func TestHandler_Create_MalformedJSON(t *testing.T) {
	router := newHandler(&fakeRepository{}).Router()

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newAuthedRequest("POST", "/", `{ "title": `, 1))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: want 400, got %d", rec.Code)
	}
}

func TestHandler_Create_WrongContentType(t *testing.T) {
	router := newHandler(&fakeRepository{}).Router()

	// Build by hand: a body but a non-JSON Content-Type.
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"title":"x"}`))
	req.Header.Set("Content-Type", "text/plain")
	req = req.WithContext(auth.WithUserID(req.Context(), 1))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status: want 415, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------
// PUT / PATCH / DELETE
// ---------------------------------------------------------------------

func TestHandler_Put_OK(t *testing.T) {
	repo := &fakeRepository{UpdateNote: Note{ID: 7, UserID: 1, Title: "new"}}
	router := newHandler(repo).Router()

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newAuthedRequest("PUT", "/7", `{"title":"new","body":"b"}`, 1))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestHandler_Patch_NothingToUpdate(t *testing.T) {
	router := newHandler(&fakeRepository{}).Router()

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newAuthedRequest("PATCH", "/7", `{}`, 1))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: want 400, got %d", rec.Code)
	}
	if got := errorCode(t, rec); got != "BAD_REQUEST" {
		t.Errorf("error code: want BAD_REQUEST, got %q", got)
	}
}

func TestHandler_Patch_OK(t *testing.T) {
	repo := &fakeRepository{PatchNote: Note{ID: 7, UserID: 1, Title: "renamed"}}
	router := newHandler(repo).Router()

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newAuthedRequest("PATCH", "/7", `{"title":"renamed"}`, 1))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	if repo.LastPatchUserID != 1 || repo.LastPatchID != 7 {
		t.Errorf("repo args: want (1, 7), got (%d, %d)", repo.LastPatchUserID, repo.LastPatchID)
	}
}

func TestHandler_Delete_204(t *testing.T) {
	router := newHandler(&fakeRepository{}).Router()

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newAuthedRequest("DELETE", "/7", "", 1))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status: want 204, got %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("204 must have empty body, got %q", rec.Body.String())
	}
}

func TestHandler_Delete_NotFound(t *testing.T) {
	router := newHandler(&fakeRepository{DeleteErr: ErrNotFound}).Router()

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newAuthedRequest("DELETE", "/99", "", 1))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: want 404, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------
// A sentinel error from the repo bubbles up through service to handler
// — proves the error-mapping chain end-to-end inside this package.
// ---------------------------------------------------------------------

func TestHandler_Get_WrappedErrNotFoundStillMaps(t *testing.T) {
	wrapped := errors.New("get failed: " + ErrNotFound.Error()) // not a real wrap
	repo := &fakeRepository{GetErr: wrapped}
	router := newHandler(repo).Router()

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newAuthedRequest("GET", "/7", "", 1))

	// Because wrapped is NOT actually wrapping ErrNotFound (no %w), it falls
	// through to respond.Internal -> 500. This is the negative test:
	// "string contains" is not the same as "errors.Is". Use %w in repos.
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: want 500 (proves Is/As needed wrap), got %d", rec.Code)
	}
}
