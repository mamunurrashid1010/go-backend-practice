package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"day19/internal/respond"
	"day19/internal/validate"
)

type Handler struct {
	Svc *Service
}

type LoginResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// Router mounts public + protected routes. The verifier is passed in so
// Router itself isn't responsible for building it.
func (h *Handler) Router(verifier *TokenVerifier) chi.Router {
	r := chi.NewRouter()

	// Public.
	r.Post("/register", h.register)
	r.Post("/login", h.login)

	// Protected — anything inside this group requires a valid Bearer token.
	r.Group(func(r chi.Router) {
		r.Use(RequireAuth(verifier))
		r.Get("/me", h.me)
	})

	return r
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var in RegisterRequest
	if !decodeJSON(w, r, &in) {
		return
	}
	if fields := validate.Struct(in); fields != nil {
		respond.ValidationFailed(w, fields)
		return
	}
	u, err := h.Svc.Register(r.Context(), in)
	if err != nil {
		writeAuthErr(w, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/users/%d", u.ID))
	respond.JSON(w, http.StatusCreated, u)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var in LoginRequest
	if !decodeJSON(w, r, &in) {
		return
	}
	if fields := validate.Struct(in); fields != nil {
		respond.ValidationFailed(w, fields)
		return
	}
	res, err := h.Svc.Login(r.Context(), in)
	if err != nil {
		writeAuthErr(w, err)
		return
	}
	respond.JSON(w, http.StatusOK, LoginResponse{
		AccessToken: res.AccessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int(res.ExpiresIn.Seconds()),
	})
}

// me is the canonical protected route — "who am I, according to the token
// you sent?"
func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r.Context())
	if !ok {
		// This branch should be unreachable if RequireAuth ran. If you see
		// it in logs, you've forgotten to wrap the route in the middleware.
		respond.Unauthorized(w, "not authenticated")
		return
	}
	u, err := h.Svc.GetByID(r.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			// Token was valid but the user was deleted — treat as 401.
			respond.Unauthorized(w, "user no longer exists")
			return
		}
		respond.Internal(w, err)
		return
	}
	respond.JSON(w, http.StatusOK, u)
}

func writeAuthErr(w http.ResponseWriter, err error) {
	var conflict *ConflictError
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		respond.Unauthorized(w, err.Error())
	case errors.As(err, &conflict):
		respond.Conflict(w, conflict.Error())
	default:
		respond.Internal(w, err)
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if ct := r.Header.Get("Content-Type"); ct != "" && ct != "application/json" {
		respond.UnsupportedMediaType(w, "Content-Type must be application/json")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(v); err != nil {
		respond.BadRequest(w, decodeErrorMessage(err))
		return false
	}
	if dec.More() {
		respond.BadRequest(w, "body must contain a single JSON object")
		return false
	}
	return true
}

func decodeErrorMessage(err error) string {
	var (
		syntaxErr    *json.SyntaxError
		unmarshalErr *json.UnmarshalTypeError
		maxBytesErr  *http.MaxBytesError
	)
	switch {
	case errors.As(err, &syntaxErr):
		return fmt.Sprintf("malformed JSON at byte %d", syntaxErr.Offset)
	case errors.As(err, &unmarshalErr):
		return fmt.Sprintf("field %q has wrong type", unmarshalErr.Field)
	case errors.As(err, &maxBytesErr):
		return "request body too large"
	default:
		return "invalid JSON request body"
	}
}
