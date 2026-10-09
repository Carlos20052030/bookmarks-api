package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Carlos20052030/bookmarks-api/internal/domain"
	"github.com/Carlos20052030/bookmarks-api/internal/storage"
	"github.com/google/uuid"
)

var testSecret = []byte("test-secret-at-least-32-bytes-long!!")

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeStore is a hand-written test double for Store. It records calls
// and can be configured to return specific errors per method.
type fakeStore struct {
	createUserErr          error
	createRefreshTokenErr  error
	lastCreatedEmail       string
	lastCreatedHash        string
	lastRefreshTokenUserID uuid.UUID
}

func (f *fakeStore) CreateUser(_ context.Context, email, hash string) (domain.User, error) {
	f.lastCreatedEmail = email
	f.lastCreatedHash = hash
	if f.createUserErr != nil {
		return domain.User{}, f.createUserErr
	}
	return domain.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: hash,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}, nil
}

func (f *fakeStore) GetUserByEmail(_ context.Context, _ string) (domain.User, error) {
	return domain.User{}, storage.ErrNotFound
}

func (f *fakeStore) CreateRefreshToken(_ context.Context, userID uuid.UUID, _ string, _ time.Time) (domain.RefreshToken, error) {
	f.lastRefreshTokenUserID = userID
	if f.createRefreshTokenErr != nil {
		return domain.RefreshToken{}, f.createRefreshTokenErr
	}
	return domain.RefreshToken{ID: uuid.New(), UserID: userID}, nil
}

func (f *fakeStore) GetRefreshTokenByHash(_ context.Context, _ string) (domain.RefreshToken, error) {
	return domain.RefreshToken{}, storage.ErrNotFound
}

func (f *fakeStore) RevokeRefreshToken(_ context.Context, _ uuid.UUID) error {
	return nil
}

func newTestAuthHandler(fs *fakeStore) *AuthHandler {
	return NewAuthHandler(fs, testSecret, discardLogger())
}

func postJSON(t *testing.T, h http.HandlerFunc, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// --- tests ---

func TestRegister_Success(t *testing.T) {
	fs := &fakeStore{}
	h := newTestAuthHandler(fs)

	rec := postJSON(t, h.Register, `{"email":"Alice@Example.com","password":"correct horse"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp authResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Fatal("tokens are empty")
	}
	if resp.TokenType != "Bearer" {
		t.Fatalf("token_type = %q, want Bearer", resp.TokenType)
	}
	if resp.ExpiresIn != 900 {
		t.Fatalf("expires_in = %d, want 900", resp.ExpiresIn)
	}

	// Email must have been normalized before being stored.
	if fs.lastCreatedEmail != "alice@example.com" {
		t.Fatalf("stored email = %q, want alice@example.com", fs.lastCreatedEmail)
	}
	// Password must be hashed, never stored in plaintext.
	if fs.lastCreatedHash == "correct horse" {
		t.Fatal("password was stored in plaintext")
	}
	if fs.lastRefreshTokenUserID != resp.User.ID {
		t.Fatal("refresh token was not bound to the created user")
	}
}

func TestRegister_InvalidJSON(t *testing.T) {
	h := newTestAuthHandler(&fakeStore{})
	rec := postJSON(t, h.Register, `{not json`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	assertErrorCode(t, rec, "invalid_json")
}

func TestRegister_InvalidEmail(t *testing.T) {
	h := newTestAuthHandler(&fakeStore{})
	rec := postJSON(t, h.Register, `{"email":"not-an-email","password":"longenough"}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	assertErrorCode(t, rec, "invalid_email")
}

func TestRegister_WeakPassword(t *testing.T) {
	h := newTestAuthHandler(&fakeStore{})
	rec := postJSON(t, h.Register, `{"email":"a@b.com","password":"short"}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	assertErrorCode(t, rec, "weak_password")
}

func TestRegister_EmailTaken(t *testing.T) {
	fs := &fakeStore{createUserErr: storage.ErrEmailTaken}
	h := newTestAuthHandler(fs)
	rec := postJSON(t, h.Register, `{"email":"a@b.com","password":"longenough"}`)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	assertErrorCode(t, rec, "email_taken")
}

func TestRegister_StoreFailure(t *testing.T) {
	fs := &fakeStore{createUserErr: errors.New("db down")}
	h := newTestAuthHandler(fs)
	rec := postJSON(t, h.Register, `{"email":"a@b.com","password":"longenough"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	assertErrorCode(t, rec, "internal_error")
}

func TestRegister_RefreshTokenFailure(t *testing.T) {
	fs := &fakeStore{createRefreshTokenErr: errors.New("db down")}
	h := newTestAuthHandler(fs)
	rec := postJSON(t, h.Register, `{"email":"a@b.com","password":"longenough"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	assertErrorCode(t, rec, "internal_error")
}

func assertErrorCode(t *testing.T, rec *httptest.ResponseRecorder, code string) {
	t.Helper()
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if body.Error != code {
		t.Fatalf("error = %q, want %q", body.Error, code)
	}
}
