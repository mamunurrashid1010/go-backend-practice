// Day 5 — Same users API as Day 4, but with the chi router.
//
// Routes:
//
//	GET    /users         -> list
//	POST   /users         -> create (201 + Location)
//	GET    /users/{id}    -> one user (or 404)
//	PUT    /users/{id}    -> replace
//	DELETE /users/{id}    -> 204
//
// Plus, to demonstrate sub-routers, the same set is also mounted under
// /api/v1/users. Same handlers, two paths.
//
// Run:   go mod init day05 && go get github.com/go-chi/chi/v5 && go run .
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

	"github.com/go-chi/chi/v5"

	"day05/internal/respond"
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

	// --- Method-based registration on the root router --------------------
	r.Get("/users", listUsersHandler)
	r.Post("/users", createUserHandler)
	r.Get("/users/{id}", getUserHandler)
	r.Put("/users/{id}", updateUserHandler)
	r.Delete("/users/{id}", deleteUserHandler)

	// --- The same routes, mounted under /api/v1 with a sub-router --------
	// Defined as a function so it could live in its own package later
	// (handler/service/repository — Day 11).
	r.Mount("/api/v1/users", usersSubrouter())
	// Mount the v2 sub-router
	r.Mount("/api/v2/users", usersV2Subrouter())

	// --- Custom JSON 404 / 405 -------------------------------------------
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

// usersSubrouter returns a chi.Router that handles the same shapes as the
// root /users routes. Mount it anywhere — including under a versioned prefix.
func usersSubrouter() chi.Router {
	r := chi.NewRouter()
	r.Get("/", listUsersHandler)
	r.Post("/", createUserHandler)
	r.Get("/{id}", getUserHandler)
	r.Put("/{id}", updateUserHandler)
	r.Delete("/{id}", deleteUserHandler)
	return r
}

func usersV2Subrouter() chi.Router {
	r := chi.NewRouter()

	// Middleware to inject the fake v2 header into every response
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-API-Version", "v2")
			next.ServeHTTP(w, r)
		})
	})

	// Register routes relative to the mount point "/api/v2/users"
	r.Get("/", listUsersHandler)         // GET /api/v2/users
	r.Post("/", createUserHandler)       // POST /api/v2/users
	r.Get("/{id}", getUserHandler)       // GET /api/v2/users/{id}
	r.Put("/{id}", updateUserHandler)    // PUT /api/v2/users/{id}
	r.Delete("/{id}", deleteUserHandler) // DELETE /api/v2/users/{id}

	return r
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

// parseID reads the {id} URL param and returns it as an int.
// On failure it writes a 400 and returns ok=false.
func parseID(w http.ResponseWriter, r *http.Request) (int, bool) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
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

// decodeUserInput is the Day 3/4 strict-decode pipeline, factored out.
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
