# Day 25 — Practice Tasks

> **Before you start:**
>
> ```powershell
> docker compose up -d
> go mod init day25
> go get github.com/go-chi/chi/v5
> go get github.com/jackc/pgx/v5/stdlib
> go get github.com/jackc/pgx/v5/pgconn
> go get -tags 'postgres' github.com/golang-migrate/migrate/v4
> go get github.com/golang-migrate/migrate/v4/database/postgres
> go get github.com/golang-migrate/migrate/v4/source/file
> go get github.com/joho/godotenv
> go get github.com/caarlos0/env/v11
> go get github.com/go-playground/validator/v10
> go get golang.org/x/crypto/bcrypt
> go get github.com/golang-jwt/jwt/v5
> go run .
> ```

---

## Warm-up — observe the structured logs

- [ ] Hit `GET /healthz` and look at your server stdout. You should see **one** structured line:
  ```
  time=... level=INFO msg="http_request" rid=... method=GET path=/healthz status=200 duration=... size=...
  ```
- [ ] Register + login + hit `GET /auth/me` with the Bearer token. The `/auth/me` line should also have `user_id=1` — the field added by `RequireAuth`.
- [ ] Now set `APP_ENV=production` (or just `LOG_LEVEL=warn`) and restart. JSON output. Compare to the text format.

---

## Task 1 — Log from a handler with the request logger

Inside `notes.Handler.create`, after a successful create, log a line:

```go
import "day25/internal/logging"

// ...inside create, after the respond.JSON line:
logging.From(r.Context()).Info("note created",
    slog.Int64("note_id", n.ID),
    slog.Int("body_len", len(in.Body)),
)
```

- [ ] Restart, create a note, watch the log line. It should carry `rid`, `method`, `path`, `user_id`, AND `note_id`, `body_len` — all in one structured line.
- [ ] Revert the change. Lesson: anywhere in the request chain, `logging.From(ctx).Info(...)` Just Works.

---

## Task 2 — Tune the log level

- [ ] Set `LOG_LEVEL=debug` and add a debug line in the service:
  ```go
  logging.From(ctx).Debug("notes.Service.Create called",
      slog.Int64("user_id", userID),
      slog.String("title", in.Title),
  )
  ```
- [ ] Hit `POST /notes`. The debug line appears in dev. Restart with `LOG_LEVEL=info`. Debug line gone.
- [ ] **Why this matters:** prod gets `info`; dev can go to `debug` without code changes.

---

## Task 3 — Replace `slog.Any("err", err)` with a richer attr

`slog.Any` uses reflection and stringifies `err.Error()`. For known typed errors, you can do better:

- [ ] Build a helper:
  ```go
  func errAttr(err error) slog.Attr {
      attrs := []any{slog.String("msg", err.Error())}
      var conflict *auth.ConflictError
      if errors.As(err, &conflict) {
          attrs = append(attrs, slog.String("conflict_field", conflict.Field), slog.String("conflict_value", conflict.Value))
      }
      return slog.Group("err", attrs...)
  }
  ```
- [ ] Trigger a duplicate-email register → log should now have a typed nested `err` group with `conflict_field=email`.
- [ ] Lesson: structured logging shines when you **decompose** errors instead of stringifying them.

---

## Task 4 — Don't log secrets

The login handler's request body has `password`. If you ever decide to log "what request came in", that body must NOT be logged.

- [ ] Search your handlers for any spot that logs request bodies. Today there are none — confirm.
- [ ] Decide a rule: **never log request bodies on auth endpoints.** Write 2 lines in your "What I learned" section about why.

---

## Task 5 — A custom log attribute group: `slog.Group`

`slog.Group("user", slog.Int64("id", 42), slog.String("email", "a@b.dev"))` nests fields. In JSON:

```json
"user": {"id": 42, "email": "a@b.dev"}
```

- [ ] In `auth/middleware.go` `RequireAuth`, change:
  ```go
  logging.From(ctx).With(slog.Int64("user_id", claims.UserID))
  ```
  to:
  ```go
  logging.From(ctx).With(slog.Group("user", slog.Int64("id", claims.UserID), slog.String("email", claims.Email)))
  ```
- [ ] Hit `/auth/me`. The log line now has a nested `user` object. Cleaner in JSON; slightly verbose in text mode.

---

## Task 6 — Log slow requests as `WARN`

The current `mw.Logger` always logs `INFO`. Promote slow requests:

- [ ] In `middleware.Logger`, after measuring duration:
  ```go
  lvl := slog.LevelInfo
  if time.Since(start) > 500*time.Millisecond {
      lvl = slog.LevelWarn
  }
  reqLog.LogAttrs(ctx, lvl, "http_request", ...)
  ```
- [ ] Test: add a fake `/slow` handler that sleeps 700ms. Hit it. The log line should be `level=WARN`.
- [ ] Lesson: severity ≠ "the call failed". A 200 that took 5s can be a worse incident than a 404.

---

## Stretch — only if you're flying

- [ ] Read the `log/slog` package doc — it's short: <https://pkg.go.dev/log/slog>.
- [ ] Make your own `slog.Handler` that drops `password` and `token` fields no matter who sets them. ("Secret-scrubbing" middleware at the log layer is a defense-in-depth pattern.)
- [ ] Pipe logs to a real aggregator: run `docker run --rm -p 3100:3100 grafana/loki`, send your stdout to it via `promtail`. Overkill for a learning project, but you'll feel why structured logs matter when you do.

---

## What I learned (fill at end of day)

> 3–5 bullets in your own words.

-
-
-
-
-
