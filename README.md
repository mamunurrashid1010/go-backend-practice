# go-backend-practice

A 15-week, day-by-day journey from Go REST basics to production-grade microservices with Postgres, Redis, Kafka, Temporal, Kubernetes, and a real cloud deploy.

This repo is my public learning log. Each day lives in its own folder with notes, runnable code, and practice tasks I work through myself.

> Full curriculum: **[LEARNING_PLAN.md](LEARNING_PLAN.md)** (105 days, 15 weeks)

---

## Why this repo exists

Most Go tutorials stop at "hello world + chi router". I wanted a structured path that ends with skills a real backend role expects in 2026: clean layering, transactions, caching, observability, async messaging, workflows, container orchestration, and a deployed product.

So I'm building it one day at a time — and committing every day, even when it's messy. If you're learning the same path, feel free to fork, copy structure, or open issues with feedback.

---

## What gets covered

| Phase | Weeks | Topics |
| ----- | ----- | ------ |
| 1 — REST foundations         | 1–2  | `net/http`, chi, middleware, JSON, Postgres, migrations, clean layering |
| 2 — Real-world API features  | 3–4  | Validation, typed errors + wrapping, JWT auth, refresh tokens, tests, slog, pagination, rate limit, CORS |
| 3 — Production fundamentals  | 5–6  | Transactions, indexes, sqlc, Redis cache, idempotency, OpenAPI, concurrency patterns, pprof, k6, golangci-lint, GitHub Actions CI |
| 4 — Frameworks & Docker      | 7–8  | Gin, Echo, multi-stage Dockerfile, docker-compose stack, graceful shutdown, health checks |
| 5 — Backend essentials       | 9    | File uploads, S3/MinIO, WebSockets, SSE, webhooks (HMAC), background jobs (asynq/river), cron |
| 6 — Async messaging          | 10–11| RabbitMQ, Kafka, outbox pattern, idempotent consumers |
| 7 — Workflows & microservices| 12–13| Temporal, gRPC, Saga, OpenTelemetry, Prometheus + Grafana, Sentry |
| 8 — Kubernetes & cloud       | 14   | kind / minikube, Helm, secrets management, real cloud deploy (fly.io / Cloud Run / ECS) |
| 9 — Capstone                 | 15   | One polished project that exercises everything above |

---

## Repo layout

Each day is a self-contained Go module so you can `cd` in and run it without affecting siblings.

```
go-backend-practice/
├── LEARNING_PLAN.md                          # the full 15-week plan
├── README.md
├── Day_01_http_basics_hello_server/
│   ├── README.md                             # day's notes
│   ├── TASKS.md                              # practice tasks + reflection
│   └── main.go                               # runnable example
├── Day_02_servemux_query_path_params/
│   └── ...
└── Day_NN_topic/
    └── ...
```

Inside every day folder you'll find:

- **`README.md`** — concept notes (request/response, status codes, etc.)
- **`TASKS.md`** — graded practice tasks I work through, ending with a "what I learned" reflection
- **`main.go`** (or more) — runnable code that ties the concepts together

---

## Running any day

Each day is its own Go module. From the repo root:

```bash
cd Day_01_http_basics_hello_server
go mod init day01    # only first time
go run .
```

Most days run on `:8080` (or `:8001` etc. — check the day's `main.go`). Hit endpoints with `curl`:

```bash
curl -v http://localhost:8080/hello?name=world
```

> **Windows / PowerShell tip:** PowerShell aliases `curl` to `Invoke-WebRequest`. Use `curl.exe` explicitly to behave like real curl.

---

## Progress

Live progress lives at the bottom of [LEARNING_PLAN.md](LEARNING_PLAN.md#progress-tracker). I tick days off as I finish them.

---

## Ground rules I'm holding myself to

- **Commit daily**, even if messy.
- **Type the code, don't paste it.** Friction is the point.
- **Don't skip mini-projects** — that's where the learning sticks.
- **One framework at a time** — no mixing Gin + Echo + chi in one project.
- **If a "day" takes two real days, that's fine.** The plan is a guide, not a deadline.

---

## License

MIT — do whatever you like with this. If it helps you learn, that's the win.
