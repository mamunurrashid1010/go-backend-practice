package httpjson

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type sample struct {
	Name string `json:"name"`
}

func newJSONReq(body string) *http.Request {
	r := httptest.NewRequest("POST", "/", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	return r
}

func TestDecodeJSON_OK(t *testing.T) {
	rec := httptest.NewRecorder()
	var v sample
	if !DecodeJSON(rec, newJSONReq(`{"name":"x"}`), &v) {
		t.Fatalf("decode should succeed, body=%s", rec.Body.String())
	}
	if v.Name != "x" {
		t.Errorf("name: want x, got %q", v.Name)
	}
}

func TestDecodeJSON_RejectsUnknownField(t *testing.T) {
	rec := httptest.NewRecorder()
	var v sample
	if DecodeJSON(rec, newJSONReq(`{"name":"x","oops":1}`), &v) {
		t.Fatalf("decode should fail on unknown field")
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: want 400, got %d", rec.Code)
	}
}

func TestDecodeJSON_RejectsMalformed(t *testing.T) {
	rec := httptest.NewRecorder()
	var v sample
	if DecodeJSON(rec, newJSONReq(`{"name":`), &v) {
		t.Fatalf("decode should fail on malformed JSON")
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status: want 400, got %d", rec.Code)
	}
}

func TestDecodeJSON_RejectsTrailingObject(t *testing.T) {
	rec := httptest.NewRecorder()
	var v sample
	if DecodeJSON(rec, newJSONReq(`{"name":"a"}{"name":"b"}`), &v) {
		t.Fatalf("decode should fail when body has more than one object")
	}
}

func TestDecodeJSON_RejectsNonJSONContentType(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"x"}`))
	r.Header.Set("Content-Type", "text/plain")
	var v sample
	if DecodeJSON(rec, r, &v) {
		t.Fatalf("decode should reject non-JSON Content-Type")
	}
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("status: want 415, got %d", rec.Code)
	}
}
