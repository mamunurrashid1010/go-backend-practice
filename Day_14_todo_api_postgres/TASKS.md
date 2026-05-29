# Day 14 — Ship It

Today isn't a teaching day. It's a **shipping** day: the project as it stands is the closer for Week 2, and the goal is to push it to GitHub as a thing someone could clone and run.

> **Before you start:**
>
> ```powershell
> docker compose up -d     # Postgres on host port 5433
> go mod init day14
> go get github.com/go-chi/chi/v5
> go get github.com/jackc/pgx/v5/stdlib
> go get -tags 'postgres' github.com/golang-migrate/migrate/v4
> go get github.com/golang-migrate/migrate/v4/database/postgres
> go get github.com/golang-migrate/migrate/v4/source/file
> go get github.com/joho/godotenv
> go get github.com/caarlos0/env/v11
> go run .
> ```

---

## Smoke test — prove every endpoint works

- [ ] `curl.exe http://localhost:8080/healthz` → `{"status":"ok"}`.
- [ ] `POST /todos` with `{"title":"x"}` → 201 + `Location: /todos/1`.
- [ ] `GET /todos` → `[ {...} ]`.
- [ ] `PATCH /todos/1` with `{"done":true}` → 200, `done` now true, `title` unchanged.
- [ ] `DELETE /todos/1` → 204 (no body).
- [ ] `GET /todos/999` → 404 JSON envelope.
- [ ] `POST /todos` with `{"title":""}` → 400 `"title is required"`.

---

## Test graceful shutdown

- [ ] In one terminal, start the server (`go run .`).
- [ ] In another terminal, send a slow-ish request:
  ```powershell
  curl.exe -i http://localhost:8080/todos
  ```
- [ ] As soon as it returns, `Ctrl+C` the server. You should see:
  ```
  shutting down...
  bye
  ```
- [ ] Try harder: start a long-running curl (e.g. against `/healthz` in a loop) and `Ctrl+C` the server mid-flight. The in-flight request should complete before the server exits.

This is `srv.Shutdown` doing its job — `Day 40` of the plan covers it formally; you've got the simple version today.

---

## Push to GitHub

If you want this folder to live as its own repo (recommended — it makes a clean portfolio piece):

```powershell
# Copy this folder somewhere outside the practice repo
Copy-Item -Recurse Day_14_todo_api_postgres ..\todo-api-go
cd ..\todo-api-go
git init
git add .
git commit -m "initial commit: To-Do REST API in Go"

# If you have the gh CLI:
gh repo create todo-api-go --public --source=. --push

# Otherwise create the repo on GitHub manually and:
git remote add origin git@github.com:<you>/todo-api-go.git
git branch -M main
git push -u origin main
```

If you'd rather keep it inside the practice repo, just commit it like every other day.

### Polish the README before pushing

The auto-generated README is decent but personalise:

- [ ] Add a `## Author` section with your name + a link to your portfolio / LinkedIn.
- [ ] Take a screenshot of `curl.exe -i http://localhost:8080/todos` output and include it (drag onto a GitHub Issues/PR page to get a hosted URL).
- [ ] Add a "License: MIT" line and a `LICENSE` file.

---

## Polish ideas (pick a few)

These are optional sandpaper passes. Pick what feels worth your time before moving to Week 3.

- [ ] **Makefile**: `make up`, `make migrate-up`, `make migrate-down`, `make run`, `make test`. Day 39 will introduce it formally.
- [ ] **`.golangci.yml`**: add a `golangci-lint` config and run it. Catches dozens of small issues. Day 41 covers it; you can preview.
- [ ] **`Dockerfile`**: multi-stage build of the API as a distroless image. Day 37 covers it; the preview shows the API can ship as a container.
- [ ] **More migrations**: add `000002_index_todos_done.up.sql` with `CREATE INDEX idx_todos_done ON todos(done) WHERE done = false;` — the partial-index trick from Day 10.
- [ ] **`MarkAllDone` service method**: from Day 11 Task 3. Now backed by Postgres (one SQL UPDATE), trivially efficient.
- [ ] **Add a tiny test**: `internal/todo/service_test.go` that uses `InMemoryRepository` and confirms `Create` rejects an empty title. Day 22 covers it properly; doing one now will make that day feel like home.

---

## Reflection — Week 2 summary

Write 5 bullets answering: **"What's different about my code now vs. Day 7?"** Not "I learned X" — concrete differences in the code.

-
-
-
-
-

---

## What I learned (Week 2)

> 5 bullets in your own words. The compounding bits.

-
-
-
-
-
