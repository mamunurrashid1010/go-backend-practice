// Day 3 — JSON encoding and decoding with encoding/json.
//
// Routes:
//
//	GET  /users         -> list all users (JSON array)
//	GET  /users/{id}    -> one user, or 404
//	POST /users         -> create a user from JSON body (201 + Location header)
//	GET  /me            -> Day 1's /me route, this time done properly
//
// Run:   go run .
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
)

// User is what we encode/decode.
//
// Note the tags:
//   - "id"          → JSON key "id"
//   - "name"        → required, validated by hand below
//   - "email,omitempty" → omit from output when empty
//   - "password" with `json:"-"` → never appears in JSON, in either direction
//   - "created_at"  → time.Time encodes to RFC 3339 automatically
type User struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email,omitempty"`
	Password  string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

// In-memory store. Same shape we'll use in Day 7's mini-project.
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
	u := User{
		ID:        s.nextID,
		Name:      name,
		Email:     email,
		CreatedAt: time.Now().UTC(),
	}
	s.items[s.nextID] = u
	s.nextID++
	return u
}

func (s *userStore) update(id int, name, email string) User {
	s.mu.Lock()
	defer s.mu.Unlock()
	// u := User{
	// 	Name:  name,
	// 	Email: email,
	// }
	u := s.items[id]

	// Update the fields
	u.Name = name
	u.Email = email

	// Save it back to the map (important since it's a value, not a pointer)
	s.items[id] = u

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
	// Seed a couple of users so GET /users isn't empty.
	store.create("Mamun", "mamun@example.dev")
	store.create("Ada", "ada@example.dev")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /users", listUsersHandler)
	mux.HandleFunc("GET /users/{id}", getUserHandler)
	mux.HandleFunc("POST /users", createUserHandler)
	mux.HandleFunc("GET /me", meHandler)
	// task 1
	mux.HandleFunc("PUT /users/{id}", updateUserHandler)
	// task 2
	mux.HandleFunc("DELETE /users/{id}", deleteUserHandler)

	srv := &http.Server{
		Addr:              ":8001",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Println("listening on http://localhost:8001")
	log.Fatal(srv.ListenAndServe())
}

// ---------- Handlers ----------------------------------------------------

// GET /users
func listUsersHandler(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, store.list())
}

// GET /users/{id}
func getUserHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		respondError(w, http.StatusBadRequest, "id must be a positive integer")
		return
	}
	u, ok := store.get(id)
	if !ok {
		respondError(w, http.StatusNotFound, "user not found")
		return
	}
	respondJSON(w, http.StatusOK, u)
}

// POST /users — accept JSON, validate, store, return 201.
func createUserHandler(w http.ResponseWriter, r *http.Request) {
	// 1. Content-Type guard.
	if ct := r.Header.Get("Content-Type"); ct != "" && ct != "application/json" {
		respondError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}

	// 2. Cap body size at 1 MiB.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	// 3. Strict decode.
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	// Use an input-only struct so the client can't smuggle in ID or CreatedAt.
	var in struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := dec.Decode(&in); err != nil {
		respondError(w, http.StatusBadRequest, decodeErrorMessage(err))
		return
	}

	// 4. Reject trailing garbage after the JSON object.
	if dec.More() {
		respondError(w, http.StatusBadRequest, "body must contain a single JSON object")
		return
	}

	// 5. Validate.
	if in.Name == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}

	u := store.create(in.Name, in.Email)
	w.Header().Set("Location", fmt.Sprintf("/users/%d", u.ID))
	respondJSON(w, http.StatusCreated, u)
}

// GET /me — Day 1 redo with real JSON encoding.
//
// In Day 1 we did: jsonBody := fmt.Sprintf(`{"name":"%s","day":%d}`, "Mamun", 1)
// That breaks if "name" contains a double quote. Below is the safe version.
func meHandler(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"name": `Bobby "Drop Tables" O'Brien`, // notice the quotes — encoder handles them
		"day":  3,
	})
}

func updateUserHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		respondError(w, http.StatusBadRequest, "id must be a positive integer")
		return
	}

	u, ok := store.get(id)
	if !ok {
		respondError(w, http.StatusNotFound, "user not found")
		return
	}

	// 1. Content-Type guard.
	if ct := r.Header.Get("Content-Type"); ct != "application/json" {
		respondError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}

	// 2. Cap body size at 1 MiB.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	// 3. Strict decode.
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	// Use an input-only struct so the client can't smuggle in ID or CreatedAt.
	var in struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := dec.Decode(&in); err != nil {
		respondError(w, http.StatusBadRequest, decodeErrorMessage(err))
		return
	}

	// 4. Reject trailing garbage after the JSON object.
	if dec.More() {
		respondError(w, http.StatusBadRequest, "body must contain a single JSON object")
		return
	}

	// 5. Validate.
	if in.Name == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}

	store.update(u.ID, in.Name, in.Email)
	w.Header().Set("Location", fmt.Sprintf("/users/%d", u.ID))
	respondJSON(w, http.StatusOK, u)
}

func deleteUserHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		respondError(w, http.StatusBadRequest, "id must be a positive integer")
		return
	}

	ok := store.delete(id)
	if !ok {
		respondError(w, http.StatusNotFound, "user not found")
		return
	}

	// 204 No Content: No headers like Content-Type and no body.
	w.WriteHeader(http.StatusNoContent)
}

// ---------- Helpers -----------------------------------------------------

// respondJSON writes v as JSON with the given status. Sneak preview of Day 4.
func respondJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Headers are already sent — log and move on.
		log.Printf("respondJSON encode error: %v", err)
	}
}

// respondError writes a small uniform error envelope.
func respondError(w http.ResponseWriter, status int, msg string) {
	respondJSON(w, status, map[string]string{"error": msg})
}

// decodeErrorMessage turns json.Decoder errors into something a client can read.
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
