// Day 4 — Error handling patterns with a real respond package.
//
// Routes:
//
//	GET    /users         -> list
//	GET    /users/{id}    -> one user (or 404)
//	POST   /users         -> create (returns 201 + Location)
//	DELETE /users/{id}    -> 204 No Content (or 404)
//	GET    /panic         -> deliberately panics, demonstrates recovery middleware
//
// Every response — successes AND errors — is JSON. Every error has the same
// envelope: { "error": { "code": "...", "message": "..." } }.
//
// Run:   go mod init day04 && go run .
// Stop:  Ctrl+C
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"day04/internal/respond"
)

type User struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type userStore struct {
	mu     sync.Mutex
	nextID int
	items  map[int]User
}

func newUserStore() *userStore {
	return &userStore{nextID: 1, items: make(map[int]User)}
}

func (s *userStore) list() []User {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]User, 0, len(s.items))
	for _, u := range s.items {
		out = append(out, u)
	}
	return out
}

func (s *userStore) get(id int) (User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.items[id]
	return u, ok
}

func (s *userStore) create(name, email string) User {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := User{ID: s.nextID, Name: name, Email: email, CreatedAt: time.Now().UTC()}
	s.items[s.nextID] = u
	s.nextID++
	return u
}

func (s *userStore) delete(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[id]; !ok {
		return false
	}
	delete(s.items, id)
	return true
}

var store = newUserStore()

func main() {
	store.create("Mamun", "mamun@example.dev")
	store.create("Ada", "ada@example.dev")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /users", listUsersHandler)
	mux.HandleFunc("GET /users/{id}", getUserHandler)
	mux.HandleFunc("POST /users", createUserHandler)
	mux.HandleFunc("DELETE /users/{id}", deleteUserHandler)
	mux.HandleFunc("GET /panic", panicHandler)

	// Catch-all: anything not matched above falls here and gets a JSON 404.
	mux.HandleFunc("/", notFoundHandler)

	// Wrap the whole mux in the panic-recovery middleware.
	handler := recoverPanic(mux)

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Println("listening on http://localhost:8080")
	log.Fatal(srv.ListenAndServe())
}

// ---------- Handlers ----------------------------------------------------

func listUsersHandler(w http.ResponseWriter, r *http.Request) {
	respond.JSON(w, http.StatusOK, store.list())
}

func getUserHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		respond.BadRequest(w, "id must be a positive integer")
		return
	}
	u, ok := store.get(id)
	if !ok {
		respond.NotFound(w, "user not found")
		return
	}
	respond.JSON(w, http.StatusOK, u)
}

func createUserHandler(w http.ResponseWriter, r *http.Request) {
	if ct := r.Header.Get("Content-Type"); ct != "" && ct != "application/json" {
		respond.UnsupportedMediaType(w, "Content-Type must be application/json")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var in struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := dec.Decode(&in); err != nil {
		respond.BadRequest(w, decodeErrorMessage(err))
		return
	}
	if dec.More() {
		respond.BadRequest(w, "body must contain a single JSON object")
		return
	}
	if in.Name == "" {
		respond.BadRequest(w, "name is required")
		return
	}

	u := store.create(in.Name, in.Email)
	w.Header().Set("Location", fmt.Sprintf("/users/%d", u.ID))
	respond.JSON(w, http.StatusCreated, u)
}

func deleteUserHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		respond.BadRequest(w, "id must be a positive integer")
		return
	}
	if !store.delete(id) {
		respond.NotFound(w, "user not found")
		return
	}
	respond.NoContent(w)
}

// GET /panic — deliberate panic to demonstrate the recovery middleware.
// In real code you'd never write this. The middleware catches it, logs it,
// and returns a 500 with the standard error envelope.
func panicHandler(w http.ResponseWriter, r *http.Request) {
	var p *User
	_ = p.Name // nil dereference → panic
}

// Catch-all for unmatched paths: JSON 404.
func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	respond.NotFound(w, "route not found: "+r.Method+" "+r.URL.Path)
}

// ---------- Middleware --------------------------------------------------

// recoverPanic is the safety net every long-lived server needs.
// Without it, one nil dereference would still complete (Go's http server
// recovers per-request) but the client gets a torn response with no body.
// With it, the client gets a clean JSON 500 and we log the cause.
func recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				respond.Internal(w, fmt.Errorf("panic: %v", rec))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// ---------- Helpers -----------------------------------------------------

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
		return "invalid JSON: " + err.Error()
	}
}
