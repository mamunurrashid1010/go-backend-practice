# Day 34 — OpenAPI / Swagger Docs

> **Goal:** turn the API surface into a machine-readable spec — `docs/openapi.yaml` — and serve interactive Swagger UI at `/docs/`. Two routes get you there; we walk both, ship one.

By the end of today:

- A single YAML file describes every endpoint, every request/response shape, every error code, the auth scheme.
- `GET /docs/openapi.yaml` returns the spec.
- `GET /docs/` returns Swagger UI rendered against that spec — clickable, "try it" buttons that hit your local server.
- Handlers carry **swag-style annotations** as the source-of-truth pattern used in real Go codebases (even though we hand-curated the 3.0 spec for this day).

---

## 1. Two roads, pick one

### `swaggo/swag` — annotations-first

Annotate handlers with Godoc-style comments; `swag init` parses them, generates a Swagger **2.0** spec.

```go
// CreateNote creates a new note for the authenticated user.
//
// @Summary      Create a note
// @Tags         notes
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      notes.CreateRequest  true  "note to create"
// @Success      201   {object}  notes.Note
// @Failure      400   {object}  respond.Error
// @Router       /notes [post]
func (h *Handler) create(w http.ResponseWriter, r *http.Request) { ... }
```

**Pros:**
- Annotations live next to the handler — drift is rare because they're staring at you.
- Generated output is committed; no runtime spec building.
- Zero refactor; works on any existing handler.

**Cons:**
- Swagger 2.0 only (no `oneOf`, weaker enum, dated tooling).
- One more codegen step in the dev loop.
- Comments are stringly-typed; bugs in annotations only fail at `swag init` time.

### `oapi-codegen` — spec-first

Write `openapi.yaml` by hand; `oapi-codegen` generates Go server stubs (interfaces + types). You implement the interface.

**Pros:**
- OpenAPI 3.0+, the modern standard.
- Stubs guarantee request/response types match the spec.
- Same spec → SDK codegen for any client language.

**Cons:**
- Spec-first means refactoring existing hand-rolled handlers to match the generated interface — work for us.
- Spec drift is a real risk in a team that doesn't run the codegen step rigorously.

### What we ship

A **hand-curated `docs/openapi.yaml` (3.0)** + **swag-style annotations** on the handlers. The annotations are committed as documentation; the YAML is the spec served at runtime. TASKS Task 1 has you actually run `swag init`, compare its 2.0 output to the curated 3.0, and decide which you'd ship in production.

---

## 2. The OpenAPI 3.0 structure, very briefly

A spec is one big YAML file with three or four sections:

```yaml
openapi: 3.0.3                   # protocol version
info: { title, version, description }   # human metadata
servers:                          # base URLs (dev, staging, prod)
  - url: http://localhost:8080
components:                       # reusable definitions
  securitySchemes:
    BearerAuth: { type: http, scheme: bearer, bearerFormat: JWT }
  schemas:
    Note: { ... }
    Error: { ... }
paths:                            # the actual API surface
  /notes:
    get:
      summary: List notes
      parameters: [ ... ]
      responses:
        '200': { ... }
```

Two patterns are worth knowing:

- **`$ref`** — any object can be `{ $ref: '#/components/schemas/Note' }`. Define once, reference many places. Critical for keeping the spec DRY.
- **`oneOf` / `allOf` / `anyOf`** — composition. Our error envelope (`{error: {code, message, details?}}`) uses inheritance: a base `Error`, then `ValidationError` (which is `allOf [Error, {details: array}]`).

Read the file [internal/openapi/openapi.yaml](internal/openapi/openapi.yaml) top-to-bottom — it's not short, but it reads.

---

## 3. Serving the spec + Swagger UI

A spec sitting in version control is documentation; a spec *served* and *explorable* is a tool. Two routes:

```go
// internal/openapi/handler.go (sketch)
//go:embed openapi.yaml
var spec []byte

func Mount(r chi.Router) {
    r.Get("/openapi.yaml", func(w http.ResponseWriter, _ *http.Request) {
        w.Header().Set("Content-Type", "application/yaml")
        w.Write(spec)
    })
    r.Get("/", func(w http.ResponseWriter, _ *http.Request) {
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        w.Write([]byte(swaggerHTML))
    })
}
```

`swaggerHTML` is one HTML page that pulls Swagger UI from the unpkg CDN and points it at `/docs/openapi.yaml`. No vendoring, no node_modules, no build step. For air-gapped deployments, swap the CDN URLs for embedded assets later.

Wire-up in `main.go` is one line under the chi router:

```go
r.Route("/docs", openapi.Mount)
```

We do **not** require auth on `/docs/*` — the API surface is public knowledge once a developer can register. Some teams gate the docs behind staging-only env or basic auth. TASKS Task 5 covers it.

---

## 4. Documenting auth, query params, error responses well

Three places spec writers get sloppy:

**Auth.** Define the scheme once in `components/securitySchemes` and reference it at the operation level. Operations on `/notes` say `security: [{ BearerAuth: [] }]`; operations on `/auth/login` say `security: []` (no auth required).

**Query parameters.** Don't just list names — give each a `schema`, `required`, `description`, and `example`. Example from `/notes`:

```yaml
parameters:
  - name: limit
    in: query
    schema: { type: integer, minimum: 1, maximum: 100, default: 20 }
    description: Max notes to return.
  - name: after
    in: query
    schema: { type: string }
    description: Opaque cursor returned by a previous response.
  - name: sort
    in: query
    schema: { type: string, enum: [asc, desc], default: desc }
```

**Error responses.** *All* of them. A handler that can return 400/401/404/422/500 needs all five listed, each with the standard error envelope as the response body. Don't just document the happy path — anyone trying to build a client wants the error shape.

The spec we ship has a single reusable error envelope at `#/components/schemas/Error` and references it from every error response.

---

## 5. The annotation walkthrough

Even though our shipped spec is hand-curated, the handler comments use swag annotations because that's the production pattern. Quick reference:

| Annotation | Meaning |
| --- | --- |
| `@Summary` / `@Description` | human text |
| `@Tags` | groups operations under a section in Swagger UI |
| `@Accept` / `@Produce` | content-type negotiation |
| `@Security <scheme>` | requires this scheme; default is none |
| `@Param <name> <in> <type> <required> <desc>` | query/path/body/header param |
| `@Success <code> {object} <type>` | response shape on success |
| `@Failure <code> {object} <type>` | response shape on failure |
| `@Router <path> [<method>]` | the route this handler serves |

The `<type>` in `@Success` and `@Failure` is a Go fully-qualified type — `notes.Note`, `respond.errorEnvelope` etc. swag turns those into `components/schemas/...` references at codegen time.

---

## 6. What changed from Day 33

| File | Change |
| --- | --- |
| `internal/openapi/openapi.yaml` | **NEW** — full OpenAPI 3.0 spec |
| `internal/openapi/handler.go` | **NEW** — embeds the spec, serves Swagger UI |
| `internal/auth/handler.go` | + swag-style annotations on every handler |
| `internal/notes/handler.go` | + annotations |
| `internal/audit/handler.go` | + annotations |
| `main.go` | mounts `/docs` |

All Day 33 wiring (rate-limit, idempotency, cache, sqlc, transactions, audit, JWT) is unchanged.

---

## 7. Run it

```powershell
cd Day_34_openapi_swagger
docker compose up -d
go mod init day34
go mod tidy
go run .
```

Open in a browser:

- <http://localhost:8080/docs/> — Swagger UI rendered against the spec
- <http://localhost:8080/docs/openapi.yaml> — the raw spec

In Swagger UI, click any endpoint, expand it, hit **Try it out**, fill in fields, click **Execute** — it actually hits your server. For authed endpoints, click the **Authorize** button at the top and paste a `Bearer <access_token>` from `/auth/login`.

---

## 8. What's next

**Day 35 — Hardened Notes API mini-project.** Week 5 closer. The cumulative result of Days 29–34 is a serious-looking API: transactions, smart indexes, sqlc, Redis cache + distributed rate limit + idempotency, OpenAPI docs. Day 35 polishes the whole bundle into a clonable Week 5 repo (the equivalent of Day 28 for Phase 3).
