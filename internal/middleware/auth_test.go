package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Carlos20052030/bookmarks-api/internal/auth"
	"github.com/google/uuid"
)

var authTestSecret = []byte("test-secret-at-least-32-bytes-long!!")

func newAuthMiddleware() func(http.Handler) http.Handler {
	return Auth(authTestSecret, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
}

func TestAuth_ValidToken(t *testing.T) {
	userID := uuid.New()
	token, err := auth.IssueAccessToken(userID, authTestSecret)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	var seen uuid.UUID
	handler := newAuthMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = UserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if seen != userID {
		t.Fatalf("userID = %v, want %v", seen, userID)
	}
}

func TestAuth_MissingHeader(t *testing.T) {
	handler := newAuthMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not be called")
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAuth_MalformedHeader(t *testing.T) {
	handler := newAuthMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not be called")
	}))

	for _, h := range []string{"token-without-bearer", "Basic abc", "Bearer"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", h)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("header %q: status = %d, want 401", h, rec.Code)
		}
	}
}

func TestAuth_InvalidToken(t *testing.T) {
	handler := newAuthMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer not-a-jwt")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAuth_WrongSecret(t *testing.T) {
	userID := uuid.New()
	token, err := auth.IssueAccessToken(userID, []byte("another-secret-at-least-32-bytes!"))
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	handler := newAuthMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAuth_ErrorShapeIsStable(t *testing.T) {
	handler := newAuthMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if body["error"] != "unauthorized" {
		t.Fatalf("error = %q, want unauthorized", body["error"])
	}
}

func TestUserIDFromContext_EmptyContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := UserIDFromContext(req.Context()); got != uuid.Nil {
		t.Fatalf("expected uuid.Nil, got %v", got)
	}
}
