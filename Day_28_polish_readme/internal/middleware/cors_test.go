package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newCORS() func(http.Handler) http.Handler {
	return CORS(CORSConfig{
		AllowedOrigins:   []string{"http://localhost:3000", "https://app.example.com"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		MaxAge:           5 * time.Minute,
		AllowCredentials: true,
	})
}

func nextOK() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
}

func TestCORS_AllowedOrigin_GetsHeaders(t *testing.T) {
	h := newCORS()(nextOK())
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/notes", nil)
	r.Header.Set("Origin", "http://localhost:3000")
	h.ServeHTTP(rec, r)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Errorf("Allow-Origin: want echo, got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("Allow-Credentials: want true, got %q", got)
	}
	if !contains(rec.Header().Values("Vary"), "Origin") {
		t.Errorf("Vary missing Origin: %v", rec.Header().Values("Vary"))
	}
}

func TestCORS_UnknownOrigin_NoAllowHeader(t *testing.T) {
	h := newCORS()(nextOK())
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/notes", nil)
	r.Header.Set("Origin", "https://evil.example.com")
	h.ServeHTTP(rec, r)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin should be empty for disallowed origin, got %q", got)
	}
}

func TestCORS_Preflight_204_NoNext(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	h := newCORS()(next)

	rec := httptest.NewRecorder()
	r := httptest.NewRequest("OPTIONS", "/notes", nil)
	r.Header.Set("Origin", "http://localhost:3000")
	r.Header.Set("Access-Control-Request-Method", "PATCH")
	h.ServeHTTP(rec, r)

	if called {
		t.Errorf("preflight should short-circuit, but next was called")
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("preflight status: want 204, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Errorf("Allow-Methods missing")
	}
	if got := rec.Header().Get("Access-Control-Max-Age"); got != "300" {
		t.Errorf("Max-Age: want 300, got %q", got)
	}
}

func TestCORS_PlainOptions_FallsThrough(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	h := newCORS()(next)

	rec := httptest.NewRecorder()
	r := httptest.NewRequest("OPTIONS", "/notes", nil)
	r.Header.Set("Origin", "http://localhost:3000")
	h.ServeHTTP(rec, r)

	if !called {
		t.Errorf("plain OPTIONS should pass through to next handler")
	}
}

func contains(ss []string, target string) bool {
	for _, s := range ss {
		if s == target {
			return true
		}
	}
	return false
}
