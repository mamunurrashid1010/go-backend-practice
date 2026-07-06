.PHONY: up down logs psql redis-cli redis-monitor migrate sqlc swagger run test test-int tidy fmt vet lint tools help

help:
	@echo "make up             - docker compose up -d (postgres + redis)"
	@echo "make down           - docker compose down"
	@echo "make logs           - tail postgres + redis logs"
	@echo "make psql           - open psql in the running postgres"
	@echo "make redis-cli      - open redis-cli in the running redis"
	@echo "make redis-monitor  - stream every Redis command"
	@echo "make migrate        - run migrations against \$$DATABASE_URL"
	@echo "make sqlc           - regenerate internal/db/ from queries/"
	@echo "make swagger        - lint the openapi.yaml with redocly"
	@echo "make run            - go run ."
	@echo "make test           - go test ./..."
	@echo "make test-int       - integration tests (testcontainers)"
	@echo "make tools          - go install all pinned dev tools"
	@echo "make tidy fmt vet lint"

# ---------------------------------------------------------------- infra
up:
	docker compose up -d

down:
	docker compose down

logs:
	docker compose logs -f postgres redis

psql:
	docker exec -it day35-pg psql -U app -d appdb

redis-cli:
	docker exec -it day35-redis redis-cli

redis-monitor:
	docker exec -it day35-redis redis-cli MONITOR

# ---------------------------------------------------------------- codegen
migrate:
	go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate \
	  -path migrations -database "$$DATABASE_URL" up

sqlc:
	go run github.com/sqlc-dev/sqlc/cmd/sqlc generate

# npx path so no global install is assumed
swagger:
	npx --yes @redocly/cli@latest lint internal/openapi/openapi.yaml

# ---------------------------------------------------------------- build
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

tools:
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	go install github.com/swaggo/swag/cmd/swag@latest
	go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
