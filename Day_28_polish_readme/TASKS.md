# Day 28 — Week 4 Polish Checklist

Week 4 closer. No new feature. The bar is **"would I clone this stranger's repo?"** — every task below tips that answer toward yes.

> **Before you start:**
>
> ```powershell
> docker compose up -d
> go mod init day28
> go mod tidy
> go run .
> go test ./...
> ```

---

## Part 1 — The README

- [ ] Open [README.md](README.md) and skim from a stranger's perspective. Can you tell in 60 seconds: what it does, how to run it, what's in it?
- [ ] If anything in the README doesn't actually work, fix it. The CI badge is a placeholder — that's fine; broken curl examples aren't.
- [ ] Add a screenshot of a working request/response under the curl walkthrough if you feel like it. Optional, not required.

---

## Part 2 — The refactor I did

- [ ] Diff against Day 27. The only code change is:
  - **new** `internal/httpjson/decode.go` + `decode_test.go`
  - `internal/auth/handler.go` and `internal/notes/handler.go` now call `httpjson.DecodeJSON` instead of carrying their own copies.
- [ ] Notice that `decodeJSON` was duplicated *twice* across Day 22 → Day 27. Two copies is borderline; three would be malpractice. Extracting it here is timed to feel earned, not premature.
- [ ] Run `go test ./...` — every test from Day 22 onwards still passes (the test files for auth and notes carry forward; only the helper they call moved).

---

## Part 3 — Refactor sweep (your turn)

Walk the code with fresh eyes. Each of these is a small win:

- [ ] **Dead code.** `go vet ./...` and `staticcheck ./...` (if installed) — fix every flagged thing or write down why you're keeping it.
- [ ] **Unused imports / variables.** Should be none, but verify.
- [ ] **Magic numbers.** `1<<20` for body limit lives in `httpjson` now; are any others scattered? `5*time.Second` for read header timeout in `main.go` is fine — it's used once.
- [ ] **Package comments.** Every package now has a top doc comment (see `// Package ...` lines). Add one anywhere it's missing.
- [ ] **Top-of-file comments.** `main.go` documents the startup sequence. `internal/notes/cursor.go` could have one too if you want.

---

## Part 4 — Repo hygiene

- [ ] [.gitignore](.gitignore) — confirms `.env` is excluded but `.env.example` is committed.
- [ ] [LICENSE](LICENSE) — MIT. Update the copyright holder name if needed.
- [ ] [Makefile](Makefile) — `make up`, `make test`, `make migrate`, etc. Verify each target works.
- [ ] [.env.example](.env.example) — every env var the app reads has a sensible default here.

---

## Part 5 — Verify the cumulative product

End-to-end sanity check that Week 4 ships:

- [ ] `docker compose up -d` then `go run .` — server starts cleanly.
- [ ] Walk the curl sequence in the README — every step returns what the README says it does.
- [ ] `go test ./...` — all unit + handler tests pass.
- [ ] Burst 60 requests at `/healthz` — see the rate limit kick in.
- [ ] CORS preflight from an allowed origin — 204 + headers. From a disallowed origin — 204 with no Allow-Origin.
- [ ] `Ctrl+C` — see "shutdown initiated" then "shutdown clean", not "context deadline exceeded" or a panic.

---

## Part 6 — The reflection (Week 4 summary)

Write 5 bullets answering "what compounded for me this week?":

-
-
-
-
-

Then 5 bullets on "what is the project NOW, vs. Day 14?":

-
-
-
-
-

(Hint: tests at every layer, structured logs, cursor pagination, rate limiting, CORS — and the layering that lets you add those without rewriting anything.)

---

## Stretch — only if you're flying

- [ ] **CI**: add `.github/workflows/ci.yml` with `go test`, `go vet`, `golangci-lint`. Replace the placeholder badge.
- [ ] **Pre-commit hook** with `lefthook` or `pre-commit` running `go fmt` + `go vet` before each commit.
- [ ] **`golangci-lint` config**: a minimal `.golangci.yml` enabling the linters you actually care about (`errcheck`, `gosimple`, `govet`, `ineffassign`, `staticcheck`, `unused`).
- [ ] **Module name**: rename `day28` to something durable (`github.com/<you>/notes-api`) if this is going to be its own repo. Update every import.
- [ ] **One README pass through Grammarly** or just read it out loud. Hobby READMEs are riddled with typos because no one proofreads them.

---

## What I learned (Week 4 summary)

> 5 bullets in your own words. Focus on what *compounded* this week.

-
-
-
-
-
