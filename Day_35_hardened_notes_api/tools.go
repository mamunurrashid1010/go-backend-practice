//go:build tools

// Package tools pins developer CLIs as build-tagged imports so
// `go install ./...` and `go mod tidy` keep them in sync with the
// codebase without dragging their code into the production binary.
//
// Run: go install github.com/sqlc-dev/sqlc/cmd/sqlc ...
//      go install github.com/swaggo/swag/cmd/swag ...
//      go install github.com/golang-migrate/migrate/v4/cmd/migrate ...
//
// Or `make tools`.
package tools

import (
	_ "github.com/golang-migrate/migrate/v4/cmd/migrate"
	_ "github.com/sqlc-dev/sqlc/cmd/sqlc"
	_ "github.com/swaggo/swag/cmd/swag"
)
