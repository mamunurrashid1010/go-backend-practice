// Package respond — consistent JSON envelope. Carried unchanged from Day 12.
package respond

import (
	"encoding/json"
	"log"
	"net/http"
)

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("respond.JSON encode error: %v", err)
	}
}

func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

func Error(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, errorEnvelope{Error: errorBody{Code: code, Message: message}})
}

func BadRequest(w http.ResponseWriter, message string) {
	Error(w, http.StatusBadRequest, "BAD_REQUEST", message)
}

func Unauthorized(w http.ResponseWriter, message string) {
	Error(w, http.StatusUnauthorized, "UNAUTHORIZED", message)
}

func NotFound(w http.ResponseWriter, message string) {
	Error(w, http.StatusNotFound, "NOT_FOUND", message)
}

func MethodNotAllowed(w http.ResponseWriter, allow string) {
	if allow != "" {
		w.Header().Set("Allow", allow)
	}
	Error(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
}

func Conflict(w http.ResponseWriter, message string) {
	Error(w, http.StatusConflict, "CONFLICT", message)
}

func UnsupportedMediaType(w http.ResponseWriter, message string) {
	Error(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", message)
}

func Internal(w http.ResponseWriter, err error) {
	log.Printf("internal error: %v", err)
	Error(w, http.StatusInternalServerError, "INTERNAL", "internal server error")
}
