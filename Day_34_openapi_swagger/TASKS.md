# Day 34 — Practice Tasks

The code ships a working Swagger UI at `/docs/` rendered against a
hand-curated OpenAPI 3.0 spec. The tasks make you exercise the spec,
generate one from annotations with `swag`, then close the loop on the
manual-vs-codegen tradeoff.

> **Before you start:**
>
> ```powershell
> docker compose up -d
> go mod init day34
> go mod tidy
> go run .
> ```
>
> Browser: <http://localhost:8080/docs/>

---

## Warm-up — drive every endpoint from Swagger UI

- [ ] Visit `/docs/`. Confirm the page loads, the four tags (health,
      auth, notes, audit) appear, and each operation is expandable.
- [ ] **Try it out** on `POST /auth/register`, fill the body, Execute.
- [ ] Same on `POST /auth/login` — copy the `access_token` from the
      response.
- [ ] Click **Authorize** at the top right, paste `Bearer <token>`.
- [ ] Now try `POST /notes` → 201. Hit `GET /notes` → see it.
- [ ] Try a deliberately invalid request (e.g. `password: "short"` on
      register). Confirm Swagger UI shows the 422 response with the
      validation error envelope.

---

## Task 1 — Install `swag` and generate the alternative

- [ ] Install:
  ```powershell
  go install github.com/swaggo/swag/cmd/swag@latest
  swag --version   # 1.x
  ```
- [ ] At the top of `main.go`, add the swag general-API annotation:
  ```go
  // @title       Notes API
  // @version     1.0
  // @description Personal notes API across Days 1–34 of the learning plan.
  // @host        localhost:8080
  // @BasePath    /
  // @securityDefinitions.apikey BearerAuth
  // @in   header
  // @name Authorization
  ```
- [ ] Run `swag init` from the day folder. It writes `docs/docs.go`,
      `docs/swagger.json`, `docs/swagger.yaml`.
- [ ] Open `docs/swagger.yaml` and diff it against
      `internal/openapi/openapi.yaml`. Things you'll notice:
  - `swag` emits **Swagger 2.0** — no `oneOf`, no `allOf`, no
    `nullable: true`.
  - Errors are flattened (no shared `Error` schema unless you annotate
    every `@Failure` with the same type).
  - Query enums + min/max are best-effort from your `Enums(...)` and
    field tags.
- [ ] Write 3 lines in "What I learned" about which output you'd ship.

---

## Task 2 — Make Swagger UI hit your local server

- [ ] In Swagger UI, click **Servers** (top dropdown).
- [ ] Confirm `http://localhost:8080` is selected.
- [ ] If you change the dev port, edit `servers:` in `openapi.yaml`
      and restart. Swagger UI updates on reload — no rebuild needed.

---

## Task 3 — Drift hunt

The hand-curated spec can lie. Find a discrepancy:

- [ ] In the spec, `PatchNoteRequest` says "at least one of title/body
      must be present." Try `PATCH /notes/1` with `{}` in Swagger UI.
      Expect 400. Confirm the spec **doesn't** model this — there's no
      400 listed on the PATCH operation.
- [ ] Add a `'400'` response to PATCH in [openapi.yaml](internal/openapi/openapi.yaml).
- [ ] Reload `/docs/`, confirm the 400 now appears in the Responses
      list.

This is the headline cost of hand-curated specs. Codegen (swag) keeps
this aligned for free — provided the annotations are correct.

---

## Task 4 — Validate the spec

- [ ] Install one of:
  ```powershell
  # Spectral (most thorough, lots of warnings)
  npx --yes @stoplight/spectral-cli lint internal/openapi/openapi.yaml

  # OR redocly (faster, fewer findings, used in CI)
  npx --yes @redocly/cli@latest lint internal/openapi/openapi.yaml
  ```
- [ ] Fix every error and at least the load-bearing warnings. Common
      ones at this stage: missing operation `summary`, missing
      `description` on `Error` properties.
- [ ] Adding this as a CI step (Day 42) is a one-liner.

---

## Task 5 — Gate `/docs` to dev only

A public production API exposing its full spec is fine for SaaS but
not for internal tools. Add a feature flag.

- [ ] In `internal/config/config.go`, add `OpenAPIEnabled bool` with
      env var `OPENAPI_ENABLED` defaulting to `true`.
- [ ] In `main.go`, wrap the `r.Route("/docs", ...)` block in
      `if cfg.OpenAPIEnabled { ... }`.
- [ ] Set `OPENAPI_ENABLED=false` in `.env`, restart, hit `/docs/` →
      404. Confirm `/healthz` still works.

Alternative: keep it on but require auth on `/docs/openapi.yaml` only.
Spec stays hidden until you log in.

---

## Task 6 — Generate a client from the spec

This is the payoff. With a 3.0 spec, you can produce typed SDKs:

- [ ] TypeScript client:
  ```powershell
  npx --yes @openapitools/openapi-generator-cli generate `
    -i internal/openapi/openapi.yaml `
    -g typescript-fetch `
    -o /tmp/notes-sdk-ts
  ```
- [ ] Inspect `/tmp/notes-sdk-ts/apis/NotesApi.ts` — every operation
      has a typed function. Same spec → Python, Java, Rust SDKs.
- [ ] Bonus: try `swagger.json` from Task 1 with the same tool. Does
      the generated client look as good?

---

## Stretch — only if you're flying

- [ ] **ReDoc instead of Swagger UI.** Replace the HTML in
      `handler.go` with the ReDoc CDN snippet. ReDoc is read-only but
      much nicer for browsing large specs.
- [ ] **Vendor Swagger UI.** Pull the dist files from npm, embed
      with `//go:embed assets/*`, serve locally. No CDN.
- [ ] **Stoplight `swagger-cli bundle`**: if your spec grows, split it
      into multiple YAML files (per-tag) and bundle into one file on
      build.
- [ ] **`oapi-codegen` server stubs.** From the spec, generate a
      `ServerInterface` Go file. Implement it in a new package and
      compare to the hand-rolled handlers we have today.

---

## What I learned (Day 34)

> 3 bullets in your own words.

-
-
-
