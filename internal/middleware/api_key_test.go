package middleware

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Carlos20052030/bookmarks-api/internal/domain"
	"github.com/Carlos20052030/bookmarks-api/internal/storage"
	"github.com/google/uuid"
)

// fakeAPIKeyStore is a test double for the API Key storage.
type fakeAPIKeyStore struct {
	getByHashErr   error
	getByHashKey   domain.APIKey
	lastHash       string
}

func (f *fakeAPIKeyStore) GetAPIKeyByHash(_ context.Context, hash string) (domain.APIKey, error) {
	f.lastHash = hash
	if f.getByHashErr != nil {
		return domain.APIKey{}, f.getByHashErr
	}
	return f.getByHashKey, nil
}

// Ensure fakeAPIKeyStore implements the required interface.
type apiKeyStoreInterface interface {
	GetAPIKeyByHash(ctx context.Context, hash string) (domain.APIKey, error)
}

var _ apiKeyStoreInterface = (*fakeAPIKeyStore)(nil)

func newAPIKeyMiddleware(store apiKeyStoreInterface) func(http.Handler) http.Handler {
	return APIKeyAuth(store, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
}

func generateTestAPIKey() (rawKey, prefix, hash string) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	rawKey = "nuntix_live_" + hex.EncodeToString(b)
	prefix = "nuntix_live_"
	h := sha256.Sum256([]byte(rawKey))
	hash = hex.EncodeToString(h[:])
	return
}

func TestAPIKeyAuth_ValidKey(t *testing.T) {
	userID := uuid.New()
	rawKey, _, hash := generateTestAPIKey()

	fakeStore := &fakeAPIKeyStore{
		getByHashKey: domain.APIKey{
			ID:     uuid.New(),
			UserID: userID,
			Name:   "Test Key",
		},
	}
	handler := newAPIKeyMiddleware(fakeStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen := UserIDFromContext(r.Context())
		if seen != userID {
			t.Fatalf("userID = %v, want %v", seen, userID)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", rawKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if fakeStore.lastHash != hash {
		t.Fatalf("store called with hash %q, want %q", fakeStore.lastHash, hash)
	}
}

func TestAPIKeyAuth_MissingHeader(t *testing.T) {
	fakeStore := &fakeAPIKeyStore{}
	handler := newAPIKeyMiddleware(fakeStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not be called")
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAPIKeyAuth_EmptyHeader(t *testing.T) {
	fakeStore := &fakeAPIKeyStore{}
	handler := newAPIKeyMiddleware(fakeStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAPIKeyAuth_InvalidKey(t *testing.T) {
	fakeStore := &fakeAPIKeyStore{
		getByHashErr: storage.ErrNotFound,
	}
	handler := newAPIKeyMiddleware(fakeStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "nuntix_live_invalidkey")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAPIKeyAuth_RevokedKey(t *testing.T) {
	now := time.Now()
	fakeStore := &fakeAPIKeyStore{
		getByHashKey: domain.APIKey{
			ID:        uuid.New(),
			UserID:    uuid.New(),
			Name:      "Revoked Key",
			RevokedAt: &now,
		},
	}
	handler := newAPIKeyMiddleware(fakeStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not be called")
	}))

	rawKey, _, _ := generateTestAPIKey()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", rawKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAPIKeyAuth_ErrorShapeIsStable(t *testing.T) {
	fakeStore := &fakeAPIKeyStore{}
	handler := newAPIKeyMiddleware(fakeStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

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