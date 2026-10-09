package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

func TestLogging_CapturesStatusAndPath(t *testing.T) {
	var buf bytes.Buffer
	logger := testLogger(&buf)

	handler := Logging(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("hello"))
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/brew", nil))

	out := buf.String()
	for _, want := range []string{"method=GET", "path=/brew", "status=418", "bytes=5", "duration_ms="} {
		if !strings.Contains(out, want) {
			t.Fatalf("log missing %q\ngot: %s", want, out)
		}
	}
}

func TestLogging_DefaultsTo200(t *testing.T) {
	var buf bytes.Buffer
	handler := Logging(testLogger(&buf))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(buf.String(), "status=200") {
		t.Fatalf("expected status=200, got: %s", buf.String())
	}
}

func TestLogging_IncludesRequestID(t *testing.T) {
	var buf bytes.Buffer
	inner := Logging(testLogger(&buf))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	handler := RequestID(inner)

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(buf.String(), "request_id=") {
		t.Fatalf("log missing request_id: %s", buf.String())
	}
	if strings.Contains(buf.String(), "request_id= ") {
		t.Fatalf("request_id is empty: %s", buf.String())
	}
}
