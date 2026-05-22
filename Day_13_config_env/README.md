# Day 13 — Config via Env Vars

> **Goal:** stop sprinkling `os.Getenv("FOO")` through `main.go`. Put every configurable thing — DSN, HTTP timeouts, pool sizes — in one `Config` struct, loaded once at startup, validated up front. The rest of the app reads from typed fields, never strings.

---

## 1. Why this matters

Day 12's `main.go` has two `os.Getenv` calls and a bunch of hard-coded numbers:

```go
dsn := os.Getenv("DATABASE_URL")
db.SetMaxOpenConns(25)
db.SetMaxIdleConns(10)
db.SetConnMaxLifetime(5 * time.Minute)
srv.Addr = ":8080"
srv.ReadTimeout = 10 * time.Second
// ...
```

That works for one developer. It breaks the moment you:

- Run **staging** with a different pool size.
- Hand the project to a teammate who needs `:8001` instead of `:8080`.
- Deploy to two regions with different DB URLs.
- Audit for "what env vars does this app actually need?"

Today's pattern: **all config in one place, typed, defaulted, validated.**

```go
cfg, err := config.Load()
if err != nil {
    log.Fatal(err)
}
log.Printf("starting in %s on %s", cfg.Env, cfg.HTTP.Addr)
```

`cfg` is a struct. `cfg.HTTP.Addr` is a `string`. `cfg.DB.MaxOpenConns` is an `int`. `cfg.HTTP.ReadTimeout` is a `time.Duration`. No more `strconv.Atoi(os.Getenv(...))` everywhere.

---

## 2. The 12-Factor rule

The "Twelve-Factor App" methodology says: **"store config in the environment."** Not in YAML files, not in JSON, not in code. Environment variables, because:

- They're the lowest common denominator — every cloud platform, container, init system, and CI/CD tool can set them.
- They're per-instance — you can change them without rebuilding.
- They never accidentally get committed (if you also gitignore `.env`).

You'll see exceptions for *truly static* settings (e.g. feature flags in YAML). For credentials, hosts, ports, timeouts — env vars are correct.

---

## 3. Library choice — `caarlos0/env` (recommended)

Two libraries dominate Go config:

| Library | What it does | When |
| --- | --- | --- |
| **`github.com/caarlos0/env/v11`** | Reads env vars into a struct via field tags. ~700 LOC. | The right default for backend services that follow 12-factor. |
| **`github.com/spf13/viper`** | Env + YAML + JSON + flags + remote config (Consul/etcd). | CLI tools / installers needing config files. Overkill for services. |

We use `caarlos0/env`. It's tiny, idiomatic, and the *only* thing it does is "env → struct". When the source-of-truth is env vars, that's the right shape.

```powershell
go get github.com/caarlos0/env/v11
```

---

## 4. The `Config` struct

[internal/config/config.go](internal/config/config.go) defines it. Three sub-structs for three concerns:

```go
type Config struct {
    Env  string      `env:"APP_ENV" envDefault:"development"`
    HTTP HTTPConfig
    DB   DBConfig
}

type HTTPConfig struct {
    Addr         string        `env:"HTTP_ADDR" envDefault:":8080"`
    ReadTimeout  time.Duration `env:"HTTP_READ_TIMEOUT" envDefault:"10s"`
    WriteTimeout time.Duration `env:"HTTP_WRITE_TIMEOUT" envDefault:"10s"`
    IdleTimeout  time.Duration `env:"HTTP_IDLE_TIMEOUT" envDefault:"60s"`
}

type DBConfig struct {
    URL             string        `env:"DATABASE_URL,required"`
    MaxOpenConns    int           `env:"DB_MAX_OPEN_CONNS" envDefault:"25"`
    MaxIdleConns    int           `env:"DB_MAX_IDLE_CONNS" envDefault:"10"`
    ConnMaxLifetime time.Duration `env:"DB_CONN_MAX_LIFETIME" envDefault:"5m"`
    ConnMaxIdleTime time.Duration `env:"DB_CONN_MAX_IDLE_TIME" envDefault:"2m"`
    PingTimeout     time.Duration `env:"DB_PING_TIMEOUT" envDefault:"5s"`
}
```

Three tag features doing the work:

- **`env:"NAME"`** — which env var to read.
- **`envDefault:"…"`** — used when the var is unset (parsed using the field's type, not just a string).
- **`,required`** — fail to start if missing. Catch misconfiguration at the door, not the first request.

`time.Duration` parses `10s`, `5m`, `1h30m`. `int` parses `25`. `bool` parses `true`/`false`. You can have **slices** via comma-separated env values, and nested struct prefixes if you want, but we'll keep it flat today.

---

## 5. The `Load()` function

```go
func Load() (Config, error) {
    var cfg Config
    if err := env.Parse(&cfg); err != nil {
        return Config{}, fmt.Errorf("config: %w", err)
    }
    if err := cfg.validate(); err != nil {
        return Config{}, err
    }
    return cfg, nil
}
```

Two phases:
1. **Parse** — read env vars into struct fields.
2. **Validate** — semantic checks the type system can't enforce (e.g. "`APP_ENV` must be development/staging/production").

Both phases return errors **before** the HTTP server starts. A bad config kills the app at startup, not at midnight on a Sunday when the first request hits.

---

## 6. What changed in `main.go`

Before:

```go
dsn := os.Getenv("DATABASE_URL")
if dsn == "" { log.Fatal("DATABASE_URL not set") }
// ...
db.SetMaxOpenConns(25)
db.SetMaxIdleConns(10)
// ...
srv := &http.Server{Addr: ":8080", ReadTimeout: 10 * time.Second, ...}
```

After:

```go
cfg, err := config.Load()
if err != nil { log.Fatal(err) }
// ...
db.SetMaxOpenConns(cfg.DB.MaxOpenConns)
db.SetMaxIdleConns(cfg.DB.MaxIdleConns)
// ...
srv := &http.Server{
    Addr:         cfg.HTTP.Addr,
    ReadTimeout:  cfg.HTTP.ReadTimeout,
    WriteTimeout: cfg.HTTP.WriteTimeout,
    IdleTimeout:  cfg.HTTP.IdleTimeout,
}
```

Same behaviour. Hardcoded numbers became reads from `cfg`. New knobs are now turnable without code changes — set `HTTP_ADDR=:9000` in `.env` and the server uses 9000.

---

## 7. The `.env` / `.env.example` discipline

| File | Tracked by git? | Purpose |
| --- | --- | --- |
| `.env.example` | ✅ committed | Template. Every var the app knows about, with a sample value. |
| `.env` | ❌ ignored | Your real values. Don't commit. |

Two rules:

1. **Every var goes in `.env.example`.** If you add `STRIPE_API_KEY` to `Config`, add it to `.env.example` the same minute with a placeholder. Teammates clone, copy `.env.example` to `.env`, fill in their values, and the app starts.
2. **Secrets stay in `.env`.** The example has placeholders only (`STRIPE_API_KEY=sk_test_replace_me`), never real keys.

The root `.gitignore` already enforces this:

```
.env
.env.*
!.env.example
```

---

## 8. The "log the config" trick

Useful debugging habit, with one caveat:

```go
log.Printf("config: env=%s addr=%s db_pool=%d/%d",
    cfg.Env, cfg.HTTP.Addr, cfg.DB.MaxOpenConns, cfg.DB.MaxIdleConns)
```

**Never log the DSN directly.** It contains the password. Either:

- Print only `host:port/db` (extract from the URL).
- Print `cfg.DB.URL` only when `cfg.Env == "development"`.
- Or just print "DATABASE_URL set".

The general rule: when you log a config dump, treat it like a public document and exclude anything you'd hide from the public.

---

## 9. Run it

```powershell
cd Day_13_config_env
go mod init day13
go get github.com/go-chi/chi/v5
go get github.com/jackc/pgx/v5/stdlib
go get -tags 'postgres' github.com/golang-migrate/migrate/v4
go get github.com/golang-migrate/migrate/v4/database/postgres
go get github.com/golang-migrate/migrate/v4/source/file
go get github.com/joho/godotenv
go get github.com/caarlos0/env/v11
go run .
```

Expected:

```
loaded .env
config: env=development addr=:8080 db_pool=25/10
migrations up: 1
connected to postgres
listening on http://localhost:8080
```

Now try overriding:

```powershell
$env:HTTP_ADDR = ":9000"
$env:DB_MAX_OPEN_CONNS = "5"
go run .
```

Server starts on 9000 with a pool of 5. Zero code changes.

Try misconfig:

```powershell
$env:DATABASE_URL = ""
go run .
# fails immediately: required env DATABASE_URL is not set
```

That's the value of `required`. The app refuses to start broken.

---

## 10. What's next

**Day 14** is the Week 2 mini-project: ship the To-Do API with Postgres + migrations + clean layering + this config package, all in one repo, pushed to GitHub. It's the consolidation day.

After today:
- One `Config` struct is the only place env vars get read.
- Misconfiguration fails fast at startup with a clear error.
- Every magic number is now a turnable knob with a default.
- Your `main.go` reads like a sentence.
