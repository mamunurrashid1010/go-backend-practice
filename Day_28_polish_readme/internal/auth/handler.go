package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"day28/internal/httpjson"
	"day28/internal/respond"
	"day28/internal/validate"
)

type Handler struct {
	Svc *Service
}

type LoginResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// Router — auth routes. The tightAuth middleware is applied ONLY to
// /register and /login (the brute-force-prone routes). /refresh, /logout
// don't need the tight limit; the global limiter covers them.
func (h *Handler) Router(verifier *TokenVerifier, tightAuth func(http.Handler) http.Handler) chi.Router {
	r := chi.NewRouter()

	r.Group(func(r chi.Router) {
		r.Use(tightAuth)
		r.Post("/register", h.register)
		r.Post("/login", h.login)
	})
	r.Post("/refresh", h.refresh)
	r.Post("/logout", h.logout)

	r.Group(func(r chi.Router) {
		r.Use(RequireAuth(verifier))
		r.Get("/me", h.me)
	})
	return r
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var in RegisterRequest
	if !httpjson.DecodeJSON(w, r, &in) {
		return
	}
	if fields := validate.Struct(in); fields != nil {
		respond.ValidationFailed(w, fields)
		return
	}
	u, err := h.Svc.Register(r.Context(), in)
	if err != nil {
		writeAuthErr(r.Context(), w, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/users/%d", u.ID))
	respond.JSON(w, http.StatusCreated, u)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var in LoginRequest
	if !httpjson.DecodeJSON(w, r, &in) {
		return
	}
	if fields := validate.Struct(in); fields != nil {
		respond.ValidationFailed(w, fields)
		return
	}
	res, err := h.Svc.Login(r.Context(), in)
	if err != nil {
		writeAuthErr(r.Context(), w, err)
		return
	}
	respond.JSON(w, http.StatusOK, LoginResponse{
		AccessToken: res.AccessToken, RefreshToken: res.RefreshToken,
		TokenType: "Bearer", ExpiresIn: int(res.ExpiresIn.Seconds()),
	})
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	var in RefreshRequest
	if !httpjson.DecodeJSON(w, r, &in) {
		return
	}
	if fields := validate.Struct(in); fields != nil {
		respond.ValidationFailed(w, fields)
		return
	}
	res, err := h.Svc.Refresh(r.Context(), in.RefreshToken)
	if err != nil {
		writeAuthErr(r.Context(), w, err)
		return
	}
	respond.JSON(w, http.StatusOK, LoginResponse{
		AccessToken: res.AccessToken, RefreshToken: res.RefreshToken,
		TokenType: "Bearer", ExpiresIn: int(res.ExpiresIn.Seconds()),
	})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	var in LogoutRequest
	if !httpjson.DecodeJSON(w, r, &in) {
		return
	}
	if fields := validate.Struct(in); fields != nil {
		respond.ValidationFailed(w, fields)
		return
	}
	if err := h.Svc.Logout(r.Context(), in.RefreshToken); err != nil {
		writeAuthErr(r.Context(), w, err)
		return
	}
	respond.NoContent(w)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r.Context())
	if !ok {
		respond.Unauthorized(w, "not authenticated")
		return
	}
	u, err := h.Svc.GetByID(r.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			respond.Unauthorized(w, "user no longer exists")
			return
		}
		respond.Internal(r.Context(), w, err)
		return
	}
	respond.JSON(w, http.StatusOK, u)
}

func writeAuthErr(ctx context.Context, w http.ResponseWriter, err error) {
	var conflict *ConflictError
	switch {
	case errors.Is(err, ErrInvalidCredentials), errors.Is(err, ErrInvalidRefreshToken):
		respond.Unauthorized(w, err.Error())
	case errors.As(err, &conflict):
		respond.Conflict(w, conflict.Error())
	default:
		respond.Internal(ctx, w, err)
	}
}
