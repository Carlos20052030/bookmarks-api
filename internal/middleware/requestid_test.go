package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestRequestID_GeneratesAndInjects(t *testing.T) {
	var seen string
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = RequestIDFromContext(r.Context())
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if seen == "" {
		t.Fatal("context has no request ID")
	}
	if _, err := uuid.Parse(seen); err != nil {
		t.Fatalf("request ID is not a UUID: %q", seen)
	}
	if got := rec.Header().Get(requestIDHeader); got != seen {
		t.Fatalf("header %q = %q, want %q", requestIDHeader, got, seen)
	}
}

func TestRequestID_UniquePerRequest(t *testing.T) {
	ids := make(map[string]bool)
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := RequestIDFromContext(r.Context())
		if ids[id] {
			t.Fatalf("duplicate request ID: %s", id)
		}
		ids[id] = true
	}))

	for i := 0; i < 10; i++ {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	}
}

func TestRequestID_IgnoresClientHeader(t *testing.T) {
	var seen string
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = RequestIDFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(requestIDHeader, "attacker-controlled")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if seen == "attacker-controlled" {
		t.Fatal("client header was trusted")
	}
}

func TestRequestIDFromContext_EmptyContext(t *testing.T) {
	if got := RequestIDFromContext(httptest.NewRequest(http.MethodGet, "/", nil).Context()); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

var _ = io.Discard
