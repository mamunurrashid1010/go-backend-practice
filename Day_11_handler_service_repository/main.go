// Day 11 — handler / service / repository layering.
//
// Same endpoints, same JSON, same behaviour as Day 7 — only the internal
// structure changes. main.go's job is now the smallest "wire it together"
// thing it can be:
//
//   1. build the repository
//   2. build the service from it
//   3. build the handler from the service
//   4. mount on chi, add middleware, start the server
//
// Day 12 will replace line (1) with NewPostgresRepository(db) and that's
// the whole change.
//
// Run:
//   go mod init day11
//   go get github.com/go-chi/chi/v5
//   go run .
package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	mw "day11/internal/middleware"
	"day11/internal/respond"
	"day11/internal/todo"
)

func main() {
	// ----------------------------------------------------------
	// 1. Wire the layers from the inside out.
	// ----------------------------------------------------------
	repo := todo.NewInMemoryRepository()
	svc := todo.NewService(repo)
	h := &todo.Handler{Svc: svc}

	// Seed two rows so GET /todos isn't empty on first run.
	// Note: we go through the service so seed inputs get the same validation.
	_, _ = svc.Create(context.Background(), todo.CreateRequest{Title: "ship Day 11"})
	_, _ = svc.Create(context.Background(), todo.CreateRequest{Title: "compare to Day 7's handler.go", Done: true})

	// ----------------------------------------------------------
	// 2. HTTP router + middleware stack.
	// ----------------------------------------------------------
	r := chi.NewRouter()
	r.Use(mw.Recover, mw.RequestID, mw.Logger)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		respond.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Mount("/todos", h.Router())

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		respond.NotFound(w, "route not found: "+r.Method+" "+r.URL.Path)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		respond.MethodNotAllowed(w, "")
	})

	// ----------------------------------------------------------
	// 3. Start the server.
	// ----------------------------------------------------------
	srv := &http.Server{
		Addr:              ":8080",
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Println("listening on http://localhost:8080")
	log.Fatal(srv.ListenAndServe())
}
