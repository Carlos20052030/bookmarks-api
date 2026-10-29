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

	"golang.org/x/crypto/bcrypt"

	"github.com/Carlos20052030/bookmarks-api/internal/domain"
	"github.com/Carlos20052030/bookmarks-api/internal/storage"
	"github.com/google/uuid"
)

// fakeAPIKeyStore is a test double for the API Key storage.
type fakeAPIKeyStore struct {
	getByPrefixErr  error
	getByPrefixKeys []domain.APIKey
	lastPrefix      string
}

func (f *fakeAPIKeyStore) GetAPIKeysByPrefix(_ context.Context, prefix string) ([]domain.APIKey, error) {
	f.lastPrefix = prefix
	if f.getByPrefixErr != nil {
		return nil, f.getByPrefixErr
	}
	// Filter out revoked keys to match the real storage behavior
	var activeKeys []domain.APIKey
	for _, key := range f.getByPrefixKeys {
		if key.RevokedAt == nil {
			activeKeys = append(activeKeys, key)
		}
	}
	return activeKeys, nil
}

// Ensure fakeAPIKeyStore implements the required interface.
type apiKeyStoreInterface interface {
	GetAPIKeysByPrefix(ctx context.Context, prefix string) ([]domain.APIKey, error)
}

var _ apiKeyStoreInterface = (*fakeAPIKeyStore)(nil)

func newAPIKeyMiddleware(store apiKeyStoreInterface) func(http.Handler) http.Handler {
	return APIKeyAuth(store, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
}

func generateTestAPIKey() (rawKey, prefix string) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	rawKey = "nuntix_live_" + hex.EncodeToString(b)
	prefix = "nuntix_live_" + hex.EncodeToString(b)[:8]
	return
}

func TestAPIKeyAuth_ValidKey(t *testing.T) {
	userID := uuid.New()
	rawKey, prefix := generateTestAPIKey()

	// Create a bcrypt hash using SHA-256 pre-hashing (same as implementation)
	shaHash := sha256.Sum256([]byte(rawKey))
	shaHex := hex.EncodeToString(shaHash[:])
	bcryptHash, err := bcrypt.GenerateFromPassword([]byte(shaHex), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("failed to generate bcrypt hash: %v", err)
	}

	fakeStore := &fakeAPIKeyStore{
		getByPrefixKeys: []domain.APIKey{
			{
				ID:        uuid.New(),
				UserID:    userID,
				Name:      "Test Key",
				KeyPrefix: prefix,
				KeyHash:   string(bcryptHash),
			},
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
	if fakeStore.lastPrefix != prefix {
		t.Fatalf("store called with prefix %q, want %q", fakeStore.lastPrefix, prefix)
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
		getByPrefixErr: storage.ErrNotFound,
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
	rawKey, prefix := generateTestAPIKey()

	// Create a bcrypt hash using SHA-256 pre-hashing (same as implementation)
	shaHash := sha256.Sum256([]byte(rawKey))
	shaHex := hex.EncodeToString(shaHash[:])
	bcryptHash, err := bcrypt.GenerateFromPassword([]byte(shaHex), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("failed to generate bcrypt hash: %v", err)
	}

	fakeStore := &fakeAPIKeyStore{
		getByPrefixKeys: []domain.APIKey{
			{
				ID:        uuid.New(),
				UserID:    uuid.New(),
				Name:      "Revoked Key",
				KeyPrefix: prefix,
				KeyHash:   string(bcryptHash),
				RevokedAt: &now,
			},
		},
	}
	handler := newAPIKeyMiddleware(fakeStore)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not be called")
	}))

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

// TestAPIKeyAuth_HashUsesBcrypt verifies that API key hashing uses bcrypt
// instead of fast hashes like SHA-256. This is a security requirement to
// prevent offline brute-force attacks if the database is compromised.
func TestAPIKeyAuth_HashUsesBcrypt(t *testing.T) {
	userID := uuid.New()

	// Generate a raw key using the same format as production
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	rawKey := "nuntix_live_" + hex.EncodeToString(b)

	// Create a bcrypt hash using SHA-256 pre-hashing (same as implementation)
	shaHash := sha256.Sum256([]byte(rawKey))
	shaHex := hex.EncodeToString(shaHash[:])
	bcryptHash, err := bcrypt.GenerateFromPassword([]byte(shaHex), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("failed to generate bcrypt hash: %v", err)
	}

	prefix := "nuntix_live_" + hex.EncodeToString(b)[:8]

	fakeStore := &fakeAPIKeyStore{
		getByPrefixKeys: []domain.APIKey{
			{
				ID:        uuid.New(),
				UserID:    userID,
				Name:      "Test Key",
				KeyPrefix: prefix,
				KeyHash:   string(bcryptHash),
			},
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

	// Verify the store was called with the correct prefix
	if fakeStore.lastPrefix != prefix {
		t.Fatalf("store called with prefix %q, want %q", fakeStore.lastPrefix, prefix)
	}

	// Verify the bcrypt hash is valid for the SHA-256 pre-hashed key
	if err := bcrypt.CompareHashAndPassword([]byte(bcryptHash), []byte(shaHex)); err != nil {
		t.Fatalf("bcrypt hash validation failed: %v", err)
	}
}