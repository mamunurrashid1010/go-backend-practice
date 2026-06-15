.PHONY: up down logs psql migrate run test test-int tidy fmt vet lint help

# Default target — show what's available
help:
	@echo "make up        - docker compose up -d"
	@echo "make down      - docker compose down"
	@echo "make logs      - tail postgres logs"
	@echo "make psql      - open psql in the running postgres container"
	@echo "make run       - go run ."
	@echo "make test      - go test ./..."
	@echo "make test-int  - integration tests (testcontainers)"
	@echo "make tidy      - go mod tidy"
	@echo "make fmt       - go fmt ./..."
	@echo "make vet       - go vet ./..."
	@echo "make lint      - run golangci-lint if installed"

up:
	docker compose up -d

down:
	docker compose down

logs:
	docker compose logs -f postgres

psql:
	docker exec -it day28-pg psql -U app -d appdb

run:
	go run .

test:
	go test ./...

test-int:
	go test -tags integration ./...

tidy:
	go mod tidy

fmt:
	go fmt ./...

vet:
	go vet ./...

lint:
	@which golangci-lint > /dev/null 2>&1 || (echo "golangci-lint not installed"; exit 1)
	golangci-lint run
