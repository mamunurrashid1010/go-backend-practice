# Day 35 — Week 5 Polish Checklist

Week 5 closer. No new feature. The bar is **"would I clone this stranger's repo?"** — every task tips that answer toward yes.

> **Before you start:**
>
> ```powershell
> docker compose up -d
> go mod init day35
> go mod tidy
> go run .
> ```

Then open [http://localhost:8080/docs/](http://localhost:8080/docs/) — every "Try it out" round-trips against your local server.

---

## Part 1 — The README

- [ ] Open [README.md](README.md). Read it from a stranger's perspective. Can you tell in 60 seconds *what it is*, *how to run it*, and *what's in it*?
- [ ] Every curl example in the README — actually run it. Fix anything that returns something unexpected.
- [ ] The CI badge in the header is a placeholder. Leave it; Day 42 wires GitHub Actions.

---

## Part 2 — The refactor I did

- [ ] Diff against Day 34. The only code change is:
  - **new** `auth.RequireUserID(w, r) (int64, bool)` — writes 401 if the user is missing.
  - `notes/handler.go` and `audit/handler.go` (and now `auth/me`) call it instead of their own copies.
- [ ] Notice this is the third helper of its exact shape across the codebase — three uses tips over "extract it."
- [ ] Run `go test ./...` — everything passes; the shape didn't change.

---

## Part 3 — Refactor sweep (your turn)

- [ ] **Dead code / unused imports.** `go vet ./...` — fix anything flagged.
- [ ] **Magic numbers.** Anything obvious to lift into a named `const`? `defaultLimit = 20` and friends are already constants. Look at `IDEMPOTENCY_LEASE_TTL=60s` — should the default be a compile-time constant?
- [ ] **Package doc comments.** Skim `internal/*` — every package should start with `// Package foo — one-line summary.` Add any missing.
- [ ] **`writeAuthErr` vs `writeServiceErr`.** They have different signatures but similar shape. Not worth extracting — different error sets. Note the *reason* in your head so a future you doesn't try again.

---

## Part 4 — Repo hygiene

- [ ] [.gitignore](.gitignore) — `.env` excluded, `.env.example` committed. Confirm `docs/docs.go` + `docs/swagger.json` + `docs/swagger.yaml` are gitignored (they're swag scratch — we ship `internal/openapi/openapi.yaml`).
- [ ] [LICENSE](LICENSE) — MIT. Update the copyright name if needed.
- [ ] [Makefile](Makefile) — every target should work. Try `make redis-cli`, `make sqlc`, `make swagger`.
- [ ] [tools.go](tools.go) — `go install ./...` should install sqlc + swag + migrate. Try `make tools`.
- [ ] [.env.example](.env.example) — every env var the app reads has a sensible default here.

---

## Part 5 — Verify the cumulative product

End-to-end sanity check for Week 5:

- [ ] `docker compose up -d` then `go run .` — starts cleanly, prints `docs=http://localhost:8080/docs/`.
- [ ] Walk the README's full curl walkthrough. Every step returns what the README says it does.
- [ ] `go test ./...` — all tests pass.
- [ ] Idempotency: same key + same body → replay. Same key + different body → 422.
- [ ] Cache: `redis-cli MONITOR` in one terminal, GET the same note twice — see one `GET note:1:1` (miss) + `SET`, then one `GET` (hit).
- [ ] Rate limit: burst 70 healthchecks, see 60 x 200 + 429s.
- [ ] Transaction rollback: inject a temporary error in `audit.Log`, POST /notes → 500, confirm no `notes` row was created.
- [ ] `Ctrl+C` — "shutdown initiated" then "shutdown clean". No context-deadline-exceeded.

---

## Part 6 — Reflection (Week 5 summary)

Write 5 bullets on "what compounded this week?":

-
-
-
-
-

Then 5 on "what is the project NOW vs Day 28?":

-
-
-
-
-

(Hint: writes are atomic, reads are cached with singleflight, the limiter holds across replicas, retries are safe, and the whole surface is machine-readable at `/docs/openapi.yaml`.)

---

## Stretch — only if you're flying

- [ ] **CI**: add `.github/workflows/ci.yml` with `go test`, `go vet`, and `redocly lint internal/openapi/openapi.yaml`. Replace the placeholder CI badge.
- [ ] **Pre-commit hook**: `lefthook` or `pre-commit` running `go fmt` + `go vet` + `sqlc diff` before each commit.
- [ ] **`golangci-lint` config**: a minimal `.golangci.yml` enabling `errcheck`, `gosimple`, `govet`, `ineffassign`, `staticcheck`, `unused`.
- [ ] **Module name**: rename `day35` to `github.com/<you>/notes-api` if this is going to be its own repo. Update every import.
- [ ] **README pass**: read it out loud. Fix anything that sounds wrong.
- [ ] **Generate a TypeScript SDK** from `internal/openapi/openapi.yaml` and check that the top-level API methods look right (Day 34 Task 6).

---

## What I learned (Week 5 summary)

> 5 bullets in your own words. Focus on what *compounded* this week.

-
-
-
-
-
