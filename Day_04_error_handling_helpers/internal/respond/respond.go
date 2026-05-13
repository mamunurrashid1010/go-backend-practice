// Package respond writes consistent JSON responses for an HTTP API.
//
// Every error response shares the same envelope:
//
//	{ "error": { "code": "USER_NOT_FOUND", "message": "user not found" } }
//
// A client can parse one type and handle every error the server returns.
package respond

import (
	"encoding/json"
	"log"
	"net/http"
)

// JSON writes v as the response body with the given status.
// Use it for SUCCESS responses (2xx). For errors, use the helpers below.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Headers are already sent — nothing useful to send to the client.
		log.Printf("respond.JSON encode error: %v", err)
	}
}

// NoContent writes a 204 with no body. Use for successful DELETE etc.
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// ---- Error envelope ---------------------------------------------------

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

// Error writes a generic error envelope at the given status.
// Prefer the named helpers below for the common cases.
func Error(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, errorEnvelope{Error: errorBody{Code: code, Message: message}})
}

// ---- Named convenience helpers ----------------------------------------

func BadRequest(w http.ResponseWriter, message string) {
	Error(w, http.StatusBadRequest, "BAD_REQUEST", message)
}

func Unauthorized(w http.ResponseWriter, message string) {
	Error(w, http.StatusUnauthorized, "UNAUTHORIZED", message)
}

func Forbidden(w http.ResponseWriter, message string) {
	Error(w, http.StatusForbidden, "FORBIDDEN", message)
}

func NotFound(w http.ResponseWriter, message string) {
	Error(w, http.StatusNotFound, "NOT_FOUND", message)
}

func MethodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	Error(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
}

func Conflict(w http.ResponseWriter, message string) {
	Error(w, http.StatusConflict, "CONFLICT", message)
}

func UnsupportedMediaType(w http.ResponseWriter, message string) {
	Error(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", message)
}

func TooLarge(w http.ResponseWriter) {
	Error(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "request body too large")
}

// Internal logs the real error server-side and returns an opaque body.
// NEVER include err.Error() in the response — it can leak DB hosts,
// stack traces, file paths, etc.
func Internal(w http.ResponseWriter, err error) {
	log.Printf("internal error: %v", err)
	Error(w, http.StatusInternalServerError, "INTERNAL", "internal server error")
}
