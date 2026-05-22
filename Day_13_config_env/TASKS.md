# Day 13 — Practice Tasks

Configuration is one of those topics that's easy to learn and easy to *not* do well. These tasks force the small habits.

> **Before you start:**
>
> ```powershell
> cd ..\Day_08_postgres_sql_basics
> docker compose ps          # "healthy"
>
> cd ..\Day_13_config_env
> go mod init day13
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

## Warm-up

- [ ] Run the server. The first log line should look like:
  ```
  config: env=development addr=:8080 db_pool=25/10
  ```
  Confirm the values match your `.env`.
- [ ] Override at the shell:
  ```powershell
  $env:HTTP_ADDR = ":9000"
  go run .
  ```
  Server now binds `:9000`. No code change.
- [ ] Curl `http://localhost:9000/healthz` — works.
- [ ] Reset: `Remove-Item Env:HTTP_ADDR`.

---

## Task 1 — Make misconfig fail loudly

- [ ] Comment out the `DATABASE_URL=...` line in `.env`.
- [ ] `go run .` — expect:
  ```
  config: required env DATABASE_URL is not set
  exit status 1
  ```
- [ ] The server **doesn't start with a broken config**. That's the win — bad state caught at the door.
- [ ] Restore `.env`.

---

## Task 2 — Add a `LOG_LEVEL` setting

A throwaway field that previews Day 25's slog work.

- [ ] In [config.go](internal/config/config.go), add:
  ```go
  LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
  ```
  on the top-level `Config`.
- [ ] Add a case in `validate()`: must be one of `debug | info | warn | error`.
- [ ] Print it in main's startup line.
- [ ] Add it to `.env.example` and `.env`.
- [ ] Test: set `LOG_LEVEL=banana` — expect a clean validation error at startup.

**Why:** muscle memory for "add a knob = 4 small files." Once you do it twice, it's reflexive.

---

## Task 3 — Add a `CORS_ORIGINS` slice

A list of allowed origins, comma-separated in the env var:

- [ ] In `HTTPConfig`:
  ```go
  AllowedOrigins []string `env:"CORS_ORIGINS" envSeparator:"," envDefault:"http://localhost:3000"`
  ```
- [ ] Set `CORS_ORIGINS=http://localhost:3000,https://my-app.example` in `.env`.
- [ ] Print `cfg.HTTP.AllowedOrigins` at startup.
- [ ] **Don't** wire CORS middleware yet — Day 27 does that properly. Today's win is just the typed parse.

**Why:** slices are the most common "uh how do I do that with env vars" question. `envSeparator` is the answer.

---

## Task 4 — Per-env defaults via Make / scripts (no code)

`caarlos0/env` doesn't have "profiles". You pick defaults via the shell.

- [ ] Create `.env.production.example` next to your existing `.env.example`. Put values you'd want in production:
  ```
  APP_ENV=production
  HTTP_READ_TIMEOUT=5s
  DB_MAX_OPEN_CONNS=50
  ```
- [ ] To test: `cp .env.production.example .env.production`, then
  ```powershell
  Get-Content .env.production | foreach { ... }    # load it
  go run .
  ```
  (Or `godotenv.Load(".env.production")` if you want to inline-switch.)

**Why:** in real apps, your CI/CD or platform sets env vars based on the deploy target. There's no "config file per env" — there's one struct, many sources.

---

## Task 5 — Stop logging the DSN

The startup log currently prints `db_pool=25/10` but **not** the DSN, which is correct. Confirm:

- [ ] Search [main.go](main.go) for `cfg.DB.URL` outside the `sql.Open` and `runMigrations` calls. It should appear nowhere in `log.Printf`.
- [ ] As an exercise, deliberately add `log.Printf("dsn=%s", cfg.DB.URL)` and run with a real password. See the password in the log.
- [ ] Now write a helper `redactDSN(s string) string` that masks the password (regex on the `:password@` part). Use it in your debug logs.
- [ ] Remove the debug log. The helper stays — you'll need it again.

**Why:** secrets in logs is the #1 way credentials leak. This is the literal one-function fix.

---

## Task 6 — Add a `flag` override on top of env (advanced)

The 12-factor preference is env vars, but local dev sometimes wants a flag override.

- [ ] In `main()`, after `config.Load()`, add:
  ```go
  addr := flag.String("addr", "", "override HTTP_ADDR")
  flag.Parse()
  if *addr != "" {
      cfg.HTTP.Addr = *addr
  }
  ```
- [ ] Test: `go run . -addr :9090` — server on 9090, ignoring env.
- [ ] Trade-off: flags are great for local; env is universal. Keep flags as **overrides**, not the source of truth.

**Why:** the real-world override pattern. Flag → env → default, last-wins.

---

## Task 7 — Group config differently and feel the seam

The `Config.HTTP.Addr` access pattern is fine for small services. As a service grows you have FAQs like:

- "Should I pass the full `Config` to constructors?" → No. Pass only the sub-struct each layer needs.
- "Should the DB package import the config package?" → No. The DB package shouldn't know about config — it just wants a `*sql.DB` and `int`s. The wiring (main.go) bridges the two.

- [ ] Confirm in this codebase: `internal/todo/postgres_repository.go` imports `config`? **No.** It takes a `*sql.DB`.
- [ ] If you wanted to make the repo configurable (say, query timeouts), would you:
  - (a) add a `*config.Config` parameter to the repo, or
  - (b) add a `timeout time.Duration` field on the repo struct?

  (b) is the right answer. main.go reads `cfg.DB.QueryTimeout` and passes the duration to `NewPostgresRepository(db, cfg.DB.QueryTimeout)`. The repo doesn't depend on the config package.

This is a thinking task. Write 3 lines in your "What I learned" section.

---

## Stretch — only if you're flying

- [ ] Try **viper** instead of `caarlos0/env` for one file. Notice the boilerplate: `viper.SetEnvPrefix(...)`, `viper.BindEnv(...)`, `viper.AutomaticEnv()`. Decide which you'd ship.
- [ ] Read the caarlos0/env README's "expand env vars" section. You can write `HTTP_ADDR=${PORT:-:8080}` and get nested expansion — useful for Heroku / Cloud Run style platforms that set `$PORT`.
- [ ] Make a `Config.String()` method that returns a **redacted** debug dump for use in a `GET /admin/config` route. (Mask the DSN password, mask any future API keys.) Useful for ops, never logs the raw values.

---

## What I learned (fill at end of day)

> 3–5 bullets in your own words.

-
-
-
-
-
