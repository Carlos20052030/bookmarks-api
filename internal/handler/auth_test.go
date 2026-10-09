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

	"github.com/Carlos20052030/bookmarks-api/internal/auth" // IMPORTANTE: Adicionado
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
	getUserByEmailErr      error
	getUserByEmailUser     domain.User
	lastCreatedEmail       string
	lastCreatedHash        string
	lastRefreshTokenUserID uuid.UUID
}

func (f *fakeStore) GetUserByEmail(_ context.Context, _ string) (domain.User, error) {
	if f.getUserByEmailErr != nil {
		return domain.User{}, f.getUserByEmailErr
	}
	return f.getUserByEmailUser, nil
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

// --- Testes de Registro ---

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

	if fs.lastCreatedEmail != "alice@example.com" {
		t.Fatalf("stored email = %q, want alice@example.com", fs.lastCreatedEmail)
	}
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

// --- Testes de Login ---

func TestLogin_Success(t *testing.T) {
	hash, err := auth.HashPassword("correct horse")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	user := domain.User{
		ID:           uuid.New(),
		Email:        "alice@example.com",
		PasswordHash: hash,
		CreatedAt:    time.Now(),
	}
	fs := &fakeStore{getUserByEmailUser: user}
	h := newTestAuthHandler(fs)

	rec := postJSON(t, h.Login, `{"email":"ALICE@example.com","password":"correct horse"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp authResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Fatal("tokens are empty")
	}
	if resp.User.Email != "alice@example.com" {
		t.Fatalf("email = %q", resp.User.Email)
	}
}

func TestLogin_UnknownEmail(t *testing.T) {
	fs := &fakeStore{getUserByEmailErr: storage.ErrNotFound}
	h := newTestAuthHandler(fs)

	rec := postJSON(t, h.Login, `{"email":"nobody@example.com","password":"whatever"}`)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	assertErrorCode(t, rec, "invalid_credentials")
}

func TestLogin_WrongPassword(t *testing.T) {
	hash, err := auth.HashPassword("correct horse")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	user := domain.User{ID: uuid.New(), Email: "a@b.com", PasswordHash: hash}
	fs := &fakeStore{getUserByEmailUser: user}
	h := newTestAuthHandler(fs)

	rec := postJSON(t, h.Login, `{"email":"a@b.com","password":"wrong password"}`)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	assertErrorCode(t, rec, "invalid_credentials")
}

func TestLogin_SameErrorForUnknownEmailAndWrongPassword(t *testing.T) {
	// The two failure modes must be indistinguishable to the client.
	hash, _ := auth.HashPassword("correct horse")
	existing := domain.User{ID: uuid.New(), Email: "a@b.com", PasswordHash: hash}

	h1 := newTestAuthHandler(&fakeStore{getUserByEmailErr: storage.ErrNotFound})
	h2 := newTestAuthHandler(&fakeStore{getUserByEmailUser: existing})

	rec1 := postJSON(t, h1.Login, `{"email":"x@y.com","password":"any"}`)
	rec2 := postJSON(t, h2.Login, `{"email":"a@b.com","password":"wrong"}`)

	if rec1.Code != rec2.Code {
		t.Fatalf("status differs: %d vs %d", rec1.Code, rec2.Code)
	}
	if rec1.Body.String() != rec2.Body.String() {
		t.Fatalf("body differs:\n%s\n%s", rec1.Body.String(), rec2.Body.String())
	}
}

func TestLogin_MissingFields(t *testing.T) {
	h := newTestAuthHandler(&fakeStore{})

	for _, body := range []string{
		`{"email":"","password":"x"}`,
		`{"email":"a@b.com","password":""}`,
		`{}`,
	} {
		rec := postJSON(t, h.Login, body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body=%s: status = %d, want 400", body, rec.Code)
		}
		assertErrorCode(t, rec, "missing_fields")
	}
}