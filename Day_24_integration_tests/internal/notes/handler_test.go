package notes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"day24/internal/auth"
)

func newAuthedRequest(method, target, body string, userID int64) *http.Request {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	return r.WithContext(auth.WithUserID(r.Context(), userID))
}

func readBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v (body: %s)", err, rec.Body.String())
	}
	return out
}

func newHandler(repo *fakeRepository) *Handler {
	return &Handler{Svc: NewService(repo)}
}

func TestHandler_Get_OK(t *testing.T) {
	repo := &fakeRepository{GetNote: Note{ID: 7, UserID: 1, Title: "hello"}}
	router := newHandler(repo).Router()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newAuthedRequest("GET", "/7", "", 1))
	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestHandler_Get_NotFound(t *testing.T) {
	router := newHandler(&fakeRepository{GetErr: ErrNotFound}).Router()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newAuthedRequest("GET", "/99", "", 1))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestHandler_Get_Unauthenticated(t *testing.T) {
	router := newHandler(&fakeRepository{}).Router()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/7", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestHandler_Create_201(t *testing.T) {
	repo := &fakeRepository{CreateNote: Note{ID: 1, UserID: 1, Title: "x"}}
	router := newHandler(repo).Router()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newAuthedRequest("POST", "/", `{"title":"x"}`, 1))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status: want 201, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/notes/1" {
		t.Errorf("Location: want /notes/1, got %q", loc)
	}
}

func TestHandler_Create_ValidationFails(t *testing.T) {
	router := newHandler(&fakeRepository{}).Router()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newAuthedRequest("POST", "/", `{"title":""}`, 1))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", rec.Code)
	}
}

func TestHandler_Patch_NothingToUpdate(t *testing.T) {
	router := newHandler(&fakeRepository{}).Router()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newAuthedRequest("PATCH", "/7", `{}`, 1))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestHandler_Delete_204(t *testing.T) {
	router := newHandler(&fakeRepository{}).Router()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newAuthedRequest("DELETE", "/7", "", 1))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("204 body should be empty, got %q", rec.Body.String())
	}
}
