// Day 7 — In-Memory To-Do REST API (Week 1 mini-project).
//
// Wiring file only. The interesting code lives in:
//   - internal/todo        (types + store + handlers)
//   - internal/middleware  (Recover, RequestID, Logger)
//   - internal/respond     (consistent JSON envelope)
//
// Run:
//
//	go mod init day07
//	go get github.com/go-chi/chi/v5
//	go run .
package main

import (
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	mw "day07/internal/middleware"
	"day07/internal/respond"
	"day07/internal/todo"
)

func main() {
	store := todo.NewStore()

	// Seed a couple of items so GET /todos isn't empty on first run.
	store.Create(todo.CreateRequest{Title: "ship Day 7"})
	store.Create(todo.CreateRequest{Title: "review Day 1–6", Done: true})

	h := &todo.Handler{Store: store}

	r := chi.NewRouter()

	// Middleware chain: outermost catches everything below.
	r.Use(mw.Recover)
	r.Use(mw.RequestID)
	r.Use(mw.Logger)

	// Routes.
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		respond.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Mount("/todos", h.Router())

	// JSON 404 / 405.
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
