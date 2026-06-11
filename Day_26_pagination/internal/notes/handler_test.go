package notes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"day26/internal/auth"
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

func TestHandler_Get_Unauthenticated(t *testing.T) {
	router := newHandler(&fakeRepository{}).Router()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/7", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestHandler_List_WrapsResponse(t *testing.T) {
	repo := &fakeRepository{
		ListPage: ListPage{
			Items:  []Note{{ID: 5, Title: "a"}, {ID: 4, Title: "b"}},
			NextID: 3,
		},
	}
	router := newHandler(repo).Router()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newAuthedRequest("GET", "/?limit=2", "", 1))
	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}

	var body struct {
		Data       []Note `json:"data"`
		NextCursor string `json:"next_cursor"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (body: %s)", err, rec.Body.String())
	}
	if len(body.Data) != 2 {
		t.Errorf("data len: want 2, got %d", len(body.Data))
	}
	if body.NextCursor == "" {
		t.Errorf("next_cursor should be set, got empty")
	}
	// Verify the cursor round-trips back to id=3.
	id, err := decodeCursor(body.NextCursor)
	if err != nil || id != 3 {
		t.Errorf("decoded cursor: %d, err: %v", id, err)
	}
}

func TestHandler_List_NoNextCursor_WhenNoMore(t *testing.T) {
	repo := &fakeRepository{
		ListPage: ListPage{
			Items:  []Note{{ID: 1, Title: "only"}},
			NextID: 0, // 0 = no more
		},
	}
	router := newHandler(repo).Router()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newAuthedRequest("GET", "/", "", 1))

	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if _, hasCursor := body["next_cursor"]; hasCursor {
		t.Errorf("next_cursor should be omitted when there's no next page; body=%s", rec.Body.String())
	}
}

func TestHandler_List_DecodesAfter(t *testing.T) {
	repo := &fakeRepository{ListPage: ListPage{Items: []Note{}}}
	router := newHandler(repo).Router()
	cursor := encodeCursor(42)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newAuthedRequest("GET", "/?after="+cursor, "", 1))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	if repo.LastListFilter.AfterID != 42 {
		t.Errorf("AfterID: want 42, got %d", repo.LastListFilter.AfterID)
	}
}

func TestHandler_List_BadCursor(t *testing.T) {
	router := newHandler(&fakeRepository{}).Router()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newAuthedRequest("GET", "/?after=not-a-cursor!!!", "", 1))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestHandler_List_BadSort(t *testing.T) {
	router := newHandler(&fakeRepository{}).Router()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newAuthedRequest("GET", "/?sort=banana", "", 1))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
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

func TestHandler_Delete_204(t *testing.T) {
	router := newHandler(&fakeRepository{}).Router()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newAuthedRequest("DELETE", "/7", "", 1))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", rec.Code)
	}
}
