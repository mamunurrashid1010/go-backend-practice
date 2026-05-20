// Day 10 — Run golang-migrate from Go code (library mode).
//
// Same migrations you'd run from the `migrate` CLI, but invoked from the
// program itself — exactly how Day 12's HTTP server will bootstrap on
// startup.
//
// Usage:
//
//	go run .                # apply all pending migrations
//	go run . down 1         # roll back one step
//	go run . version        # print current version
//	go run . force 2        # force version (for unsticking a "dirty" DB)
//
// Env:
//
//	DATABASE_URL = "postgres://app:app@localhost:5433/appdb?sslmode=disable"
package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://app:app@localhost:5433/appdb?sslmode=disable"
		log.Println("DATABASE_URL not set, using default:", dsn)
	}

	// file://migrations resolves relative to the current working directory.
	m, err := migrate.New("file://migrations", dsn)
	if err != nil {
		log.Fatalf("migrate.New: %v", err)
	}
	defer func() {
		// Both connections must be closed to avoid a leaked goroutine.
		srcErr, dbErr := m.Close()
		if srcErr != nil {
			log.Printf("close source: %v", srcErr)
		}
		if dbErr != nil {
			log.Printf("close db: %v", dbErr)
		}
	}()

	// Dispatch on first arg.
	cmd := "up"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "up":
		err := m.Up()
		report(m, err, "up")

	case "down":
		// `down` with no count rolls back ALL migrations — dangerous.
		// Require an explicit step count so a typo can't wipe the schema.
		if len(os.Args) < 3 {
			log.Fatalf("usage: go run . down <steps>   (e.g. 'down 1')")
		}
		n, err := strconv.Atoi(os.Args[2])
		if err != nil || n <= 0 {
			log.Fatalf("down: steps must be a positive integer, got %q", os.Args[2])
		}
		err = m.Steps(-n)
		report(m, err, fmt.Sprintf("down %d", n))

	case "version":
		v, dirty, err := m.Version()
		if errors.Is(err, migrate.ErrNilVersion) {
			fmt.Println("version: (no migrations applied yet)")
			return
		}
		if err != nil {
			log.Fatalf("version: %v", err)
		}
		fmt.Printf("version: %d  dirty: %t\n", v, dirty)

	case "force":
		if len(os.Args) < 3 {
			log.Fatalf("usage: go run . force <version>")
		}
		v, err := strconv.Atoi(os.Args[2])
		if err != nil {
			log.Fatalf("force: version must be an integer, got %q", os.Args[2])
		}
		if err := m.Force(v); err != nil {
			log.Fatalf("force %d: %v", v, err)
		}
		fmt.Printf("forced to version %d (dirty flag cleared)\n", v)

	default:
		log.Fatalf("unknown command %q (try: up | down N | version | force N)", cmd)
	}
}

// report prints the post-migration state, treating "no change" as a normal
// outcome rather than an error.
func report(m *migrate.Migrate, err error, action string) {
	switch {
	case errors.Is(err, migrate.ErrNoChange):
		fmt.Printf("%s: nothing to do (already at latest)\n", action)
	case err != nil:
		log.Fatalf("%s: %v", action, err)
	default:
		fmt.Printf("%s: done\n", action)
	}
	v, dirty, vErr := m.Version()
	if errors.Is(vErr, migrate.ErrNilVersion) {
		fmt.Println("current version: (none)")
		return
	}
	if vErr == nil {
		fmt.Printf("current version: %d  dirty: %t\n", v, dirty)
	}
}
