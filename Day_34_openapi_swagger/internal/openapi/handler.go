// Package openapi serves the spec + Swagger UI.
//
// The spec is embedded at compile time so the binary is self-contained.
// Swagger UI is loaded from unpkg.com — no node_modules, no vendored
// assets. For air-gapped deployments, swap the script/stylesheet URLs
// for embedded files later.
package openapi

import (
	_ "embed"
	"net/http"

	"github.com/go-chi/chi/v5"
)

//go:embed openapi.yaml
var spec []byte

// Mount wires the docs routes:
//
//	GET /docs/                — HTML page with Swagger UI loading from CDN
//	GET /docs/openapi.yaml    — the raw spec
//
// Designed to be passed to chi.Router.Route("/docs", openapi.Mount).
func Mount(r chi.Router) {
	r.Get("/openapi.yaml", serveSpec)
	r.Get("/", serveUI)
}

func serveSpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(spec)
}

func serveUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(swaggerHTML))
}

const swaggerHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Notes API — Swagger UI</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
  <style> body { margin: 0; } </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js" crossorigin></script>
  <script>
    window.addEventListener('load', () => {
      window.ui = SwaggerUIBundle({
        url: 'openapi.yaml',
        dom_id: '#swagger-ui',
        deepLinking: true,
        persistAuthorization: true,
      });
    });
  </script>
</body>
</html>
`
