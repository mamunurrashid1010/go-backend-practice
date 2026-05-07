// Day 1 — A tiny HTTP server using only the standard library.
//
// Routes:
//
//		GET  /          -> plain-text greeting
//		GET  /hello     -> greets ?name= (query param)
//		GET  /time      -> current server time, demonstrates setting headers
//		GET  /headers   -> echoes the request headers back as text
//		POST /echo      -> echoes the raw request body
//	 Practice task
//
// Run:   go run .
// Stop:  Ctrl+C
package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"
)

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/", rootHandler)
	mux.HandleFunc("/hello", helloHandler)
	mux.HandleFunc("/time", timeHandler)
	mux.HandleFunc("/headers", headersHandler)
	mux.HandleFunc("/echo", echoHandler)

	// task 1
	mux.HandleFunc("/goodbye", goodbyeHandler)
	// task 2
	mux.HandleFunc("/teapot", teapotHandler)
	mux.HandleFunc("/redirect", redirectHandle)
	// task 3
	mux.HandleFunc("/add", addHandler)
	// task 5
	mux.HandleFunc("/whoami", whoamiHandler)
	// task 6
	mux.HandleFunc("/me", meHandler)

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

// GET / -> simple plain text response.
func rootHandler(w http.ResponseWriter, r *http.Request) {
	// http.ServeMux's "/" pattern matches *every* unmatched path,
	// so reject anything that isn't exactly "/".
	if r.URL.Path != "/" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)

		fmt.Fprintf(w, "no route for %s %s", r.Method, r.URL.Path)
		return
	}

	// Only allow GET
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		// use anyone for error response
		//http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintln(w, "Hello, world! Try /hello?name=Mamun, /time, /headers, or POST /echo")
}

// GET /hello?name=Mamun
func helloHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		name = "stranger"
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "Hello, %s!\n", name)
}

// GET /time -> shows how to write status + headers + body explicitly.
func timeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Server", "day01")
	w.WriteHeader(http.StatusOK) // optional here; first Write would do it
	fmt.Fprintf(w, "server time: %s\n", time.Now().Format(time.RFC3339))
}

// GET /headers -> dump the incoming request headers.
func headersHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "%s %s %s\n", r.Method, r.URL.Path, r.Proto)
	for key, values := range r.Header {
		for _, v := range values {
			fmt.Fprintf(w, "%s: %s\n", key, v)
		}
	}
}

// POST /echo -> read the body and write it back.
func echoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()

	// Cap the body at 1 MiB so a huge upload can't exhaust memory.
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		http.Error(w, "could not read body", http.StatusBadRequest)
		return
	}

	// Reflect back whatever Content-Type the client sent (default to plain text).
	ct := r.Header.Get("Content-Type")
	if ct == "" {
		ct = "text/plain; charset=utf-8"
	}
	w.Header().Set("Content-Type", ct)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func goodbyeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		name = "name not found!"
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "Goodbye, %s\n", name)
}

func teapotHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	bodyMessage := "short and stout"

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusTeapot)
	fmt.Fprint(w, "I'm a teapot\n"+bodyMessage)
}

func redirectHandle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Location", "/")
	w.WriteHeader(http.StatusFound) // 302
}

func addHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	aStr := r.URL.Query().Get("a")
	bStr := r.URL.Query().Get("b")

	if aStr == "" || bStr == "" {
		http.Error(w, "Missing query parameters: a and b are required", http.StatusBadRequest)
		return
	}

	a, err := strconv.Atoi(aStr)
	if err != nil {
		http.Error(w, "Invalid value for 'a': must be a number", http.StatusBadRequest)
		return
	}

	b, err := strconv.Atoi(bStr)
	if err != nil {
		http.Error(w, "Invalid value for 'b': must be a number", http.StatusBadRequest)
		return
	}

	// Add numbers
	result := a + b

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, result)
}

func whoamiHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	agent := r.UserAgent()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "you are: %s", agent)
}

func meHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	jsonBody := fmt.Sprintf(
		`{"name":"%s","day":%d}`,
		"Mamun",
		1,
	)

	fmt.Fprint(w, jsonBody)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)

	fmt.Fprint(w, msg)
}
