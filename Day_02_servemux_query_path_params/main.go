// Day 2 — http.HandlerFunc, ServeMux, query params, and path params.
//
// Routes (manual / pre-1.22 style):
//
//	GET  /ping             -> "pong"
//	GET  /echo?msg=...     -> echoes msg, 400 if empty/missing
//	GET  /search?q=...     -> demonstrates multi-value query params
//	GET  /users/{id}       -> manual path-parameter parsing (subtree pattern)
//
// Routes (Go 1.22+ pattern-aware ServeMux):
//
//	GET  /v2/users/{id}    -> uses r.PathValue("id")
//	GET  /v2/users/{id}/posts/{postID}
//
// Run:   go run .
// Stop:  Ctrl+C
package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func main() {
	mux := http.NewServeMux()

	// --- Pre-1.22 / "manual" style ---------------------------------------
	mux.HandleFunc("/ping", pingHandler)
	mux.HandleFunc("/echo", echoHandler)
	mux.HandleFunc("/search", searchHandler)

	// Subtree pattern (note the trailing slash) — needed for manual parsing.
	mux.HandleFunc("/users/", usersHandler)

	// --- Go 1.22+ pattern-aware style ------------------------------------
	// The "GET " prefix means non-GET requests get an automatic 405 + Allow.
	// {id} captures the segment; read it with r.PathValue("id").
	mux.HandleFunc("GET /v2/users/{id}", v2UserHandler)
	mux.HandleFunc("GET /v2/users/{id}/posts/{postID}", v2UserPostHandler)

	// task 1
	mux.HandleFunc("GET /v2/greet", greethandler)
	// task 2
	mux.HandleFunc("GET /v2/sum", sumHandler)
	// task 4
	mux.HandleFunc("GET /v2/products/{id}", productHandler)

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

// GET /ping -> plain text "pong".
func pingHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintln(w, "pong")
}

// GET /echo?msg=...
//
// Demonstrates: reading a query param, distinguishing missing vs empty,
// returning 400 with a useful message.
func echoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()
	if !q.Has("msg") {
		http.Error(w, "missing required query param: msg", http.StatusBadRequest)
		return
	}
	msg := q.Get("msg")
	if msg == "" {
		http.Error(w, "msg cannot be empty", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintln(w, msg)
}

// GET /search?q=golang&tag=web&tag=api
//
// Demonstrates: multi-value query params, optional params with defaults.
func searchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()
	query := q.Get("q")
	tags := q["tag"] // []string — could be empty

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "q=%q\n", query)
	if len(tags) == 0 {
		fmt.Fprintln(w, "tags=(none)")
	} else {
		fmt.Fprintf(w, "tags=%s\n", strings.Join(tags, ","))
	}
}

// GET /users/, /users/42, /users/42/posts -> manual path-param parsing.
//
// We registered the pattern "/users/" (trailing slash = subtree match), so
// THIS handler runs for any path that starts with "/users/". We do the
// dispatch ourselves.
func usersHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Strip the prefix to get whatever's after "/users/".
	rest := strings.TrimPrefix(r.URL.Path, "/users/")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	if rest == "" {
		// /users/ -> list
		fmt.Fprintln(w, "list of users (manual route)")
		return
	}

	parts := strings.Split(rest, "/")
	switch {
	case len(parts) == 1:
		// /users/42
		fmt.Fprintf(w, "user id=%s (manual route)\n", parts[0])
	case len(parts) == 2 && parts[1] == "posts":
		// /users/42/posts
		fmt.Fprintf(w, "posts for user id=%s (manual route)\n", parts[0])
	default:
		http.NotFound(w, r)
	}
}

// GET /v2/users/{id} -> Go 1.22+ ServeMux pattern parameters.
func v2UserHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "user id=%s (1.22 route)\n", id)
}

// GET /v2/users/{id}/posts/{postID}
func v2UserPostHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	postID := r.PathValue("postID")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "user=%s post=%s (1.22 route)\n", id, postID)
}

func greethandler(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	lang := r.URL.Query().Get("lang")

	if name == "" {
		http.Error(w, "Missing query parameters: name is required!", http.StatusBadRequest)
		return
	}

	if lang == "" {
		lang = "en"
	}

	if lang != "en" && lang != "bn" {
		http.Error(w, "Language does not support. Supported lang is en or bn", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if lang == "bn" {
		fmt.Fprintf(w, "Salam, %s!", name)
	} else {
		fmt.Fprintf(w, "Hello, %s!", name)
	}
}

func sumHandler(w http.ResponseWriter, r *http.Request) {
	nums := r.URL.Query()["n"]

	if len(nums) == 0 {
		http.Error(w, "Missing query parameter: n is required!", http.StatusBadRequest)
		return
	}

	total := 0
	for _, value := range nums {
		n, err := strconv.Atoi(value)
		if err != nil {
			// Return 400 and name the offending value if not an integer
			msg := fmt.Sprintf("Invalid value: '%s' is not an integer", value)
			http.Error(w, msg, http.StatusBadRequest)
			return
		}
		total += n
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "%d", total)
}

func productHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve the wildcard {id} from the path
	idStr := r.PathValue("id")

	// Convert string to integer
	id, err := strconv.Atoi(idStr)
	
	// Validate: must be an integer AND greater than 0
	if err != nil || id <= 0 {
		http.Error(w, "Invalid ID: Please provide a positive integer", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "Product ID: %d", id)
}
