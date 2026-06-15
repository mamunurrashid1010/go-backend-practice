package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// CORSConfig — allowlist-based. AllowedOrigins MUST NOT contain "*"
// when AllowCredentials is true; the spec forbids that combination.
type CORSConfig struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	ExposedHeaders   []string
	MaxAge           time.Duration
	AllowCredentials bool
}

// CORS — returns a middleware that:
//   - on every request: sets Vary: Origin and, if Origin is allowed,
//     sets Access-Control-Allow-Origin (and ...Credentials when on).
//   - on a preflight (OPTIONS + Access-Control-Request-Method): writes
//     the allow-methods/-headers/-max-age and short-circuits with 204.
//     The next handler is never called for a preflight.
func CORS(cfg CORSConfig) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(cfg.AllowedOrigins))
	wildcard := false
	for _, o := range cfg.AllowedOrigins {
		if o == "*" {
			wildcard = true
			continue
		}
		allowed[o] = struct{}{}
	}
	methods := strings.Join(cfg.AllowedMethods, ", ")
	headers := strings.Join(cfg.AllowedHeaders, ", ")
	exposed := strings.Join(cfg.ExposedHeaders, ", ")
	maxAge := strconv.Itoa(int(cfg.MaxAge.Seconds()))

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Vary: Origin is always emitted on routes that may CORS-vary
			// so a cache doesn't reuse the response for a different origin.
			w.Header().Add("Vary", "Origin")

			if origin != "" && (wildcard || originAllowed(allowed, origin)) {
				// Wildcard + credentials is illegal per the CORS spec.
				// Echo the request origin instead so the combination works.
				if wildcard && !cfg.AllowCredentials {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else {
					w.Header().Set("Access-Control-Allow-Origin", origin)
				}
				if cfg.AllowCredentials {
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
				if exposed != "" {
					w.Header().Set("Access-Control-Expose-Headers", exposed)
				}
			}

			// Preflight short-circuit. A "real" OPTIONS request without
			// Access-Control-Request-Method falls through to the router.
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.Header().Add("Vary", "Access-Control-Request-Method")
				w.Header().Add("Vary", "Access-Control-Request-Headers")
				w.Header().Set("Access-Control-Allow-Methods", methods)
				w.Header().Set("Access-Control-Allow-Headers", headers)
				w.Header().Set("Access-Control-Max-Age", maxAge)
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func originAllowed(set map[string]struct{}, origin string) bool {
	_, ok := set[origin]
	return ok
}
