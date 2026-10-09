package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecovery_CatchesPanic(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	handler := Recovery(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"internal_server_error"`) {
		t.Fatalf("body = %q, want JSON error", rec.Body.String())
	}
	if !strings.Contains(buf.String(), "panic recovered") {
		t.Fatalf("panic not logged: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "boom") {
		t.Fatalf("panic value not logged: %s", buf.String())
	}
}

func TestRecovery_PassesThroughWhenNoPanic(t *testing.T) {
	handler := Recovery(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte("fine"))
		}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
	if rec.Body.String() != "fine" {
		t.Fatalf("body = %q, want fine", rec.Body.String())
	}
}
