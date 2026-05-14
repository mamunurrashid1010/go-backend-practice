// Day 6 — Middleware: logging, recovery, request ID.
//
// Same users API as Day 5, now with a middleware stack.
//
// Routes:
//
//	GET    /users         -> list
//	POST   /users         -> create
//	GET    /users/{id}    -> one user
//	PUT    /users/{id}    -> replace
//	DELETE /users/{id}    -> 204
//	GET    /panic         -> deliberately panics (shows Recover in action)
//
// Run:
//
//	go mod init day06
//	go get github.com/go-chi/chi/v5
//	go run .
//
// Stop: Ctrl+C
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

	"github.com/go-chi/chi/v5"

	mw "day06/internal/middleware"
	"day06/internal/respond"
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

func (s *userStore) update(id int, name, email string) (User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.items[id]
	if !ok {
		return User{}, false
	}
	u.Name = name
	u.Email = email
	s.items[id] = u
	return u, true
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

	r := chi.NewRouter()

	// --- The middleware stack -------------------------------------------
	//
	// Order matters:
	//   Recover    — outermost: catches panics from anything below
	//   RequestID  — early: so Logger and handlers can read the ID
	//   Logger     — after RequestID so the log line includes it
	r.Use(mw.Recover)
	r.Use(mw.RequestID)
	r.Use(mw.Logger)

	// --- Routes ---------------------------------------------------------
	r.Route("/users", func(r chi.Router) {
		r.Get("/", listUsersHandler)
		r.Post("/", createUserHandler)
		r.Get("/{id}", getUserHandler)
		r.Put("/{id}", updateUserHandler)
		r.Delete("/{id}", deleteUserHandler)
	})

	// Deliberately panics — useful for verifying the Recover middleware.
	r.Get("/panic", func(w http.ResponseWriter, r *http.Request) {
		var p *User
		_ = p.Name // nil dereference
	})

	// --- Custom JSON 404 / 405 ------------------------------------------
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		respond.NotFound(w, "route not found: "+r.Method+" "+r.URL.Path)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		respond.MethodNotAllowed(w, "")
	})

	srv := &http.Server{
		Addr:              ":8001",
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Println("listening on http://localhost:8001")
	log.Fatal(srv.ListenAndServe())
}

// ---------- Handlers ----------------------------------------------------

func listUsersHandler(w http.ResponseWriter, r *http.Request) {
	respond.JSON(w, http.StatusOK, store.list())
}

func getUserHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
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
	in, ok := decodeUserInput(w, r)
	if !ok {
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

func updateUserHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	in, ok := decodeUserInput(w, r)
	if !ok {
		return
	}
	if in.Name == "" {
		respond.BadRequest(w, "name is required")
		return
	}
	u, ok := store.update(id, in.Name, in.Email)
	if !ok {
		respond.NotFound(w, "user not found")
		return
	}
	respond.JSON(w, http.StatusOK, u)
}

func deleteUserHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if !store.delete(id) {
		respond.NotFound(w, "user not found")
		return
	}
	respond.NoContent(w)
}

// ---------- Small handler helpers ---------------------------------------

func parseID(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		respond.BadRequest(w, "id must be a positive integer")
		return 0, false
	}
	return id, true
}

type userInput struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func decodeUserInput(w http.ResponseWriter, r *http.Request) (userInput, bool) {
	var in userInput
	if ct := r.Header.Get("Content-Type"); ct != "" && ct != "application/json" {
		respond.UnsupportedMediaType(w, "Content-Type must be application/json")
		return in, false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(&in); err != nil {
		respond.BadRequest(w, decodeErrorMessage(err))
		return in, false
	}
	if dec.More() {
		respond.BadRequest(w, "body must contain a single JSON object")
		return in, false
	}
	return in, true
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
