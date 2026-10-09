package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/Carlos20052030/bookmarks-api/internal/auth"
	"github.com/Carlos20052030/bookmarks-api/internal/domain"
	"github.com/Carlos20052030/bookmarks-api/internal/storage"
	"github.com/google/uuid"
)

const minPasswordLen = 8

// Store is the subset of storage operations the auth handler needs.
// Defined here so tests can provide a hand-written fake.
type Store interface {
	CreateUser(ctx context.Context, email, passwordHash string) (domain.User, error)
	GetUserByEmail(ctx context.Context, email string) (domain.User, error)
	CreateRefreshToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) (domain.RefreshToken, error)
	GetRefreshTokenByHash(ctx context.Context, tokenHash string) (domain.RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, id uuid.UUID) error
	RevokeAllUserTokens(ctx context.Context, userID uuid.UUID) error
}

// AuthHandler holds the dependencies for auth endpoints.
type AuthHandler struct {
	store     Store
	jwtSecret []byte
	logger    *slog.Logger
}

func NewAuthHandler(store Store, jwtSecret []byte, logger *slog.Logger) *AuthHandler {
	return &AuthHandler{store: store, jwtSecret: jwtSecret, logger: logger}
}

// --- request / response types ---

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userResponse struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

type authResponse struct {
	User         userResponse `json:"user"`
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	TokenType    string       `json:"token_type"`
	ExpiresIn    int          `json:"expires_in"`
}

// --- helpers ---

// normalizeEmail lowercases and trims whitespace so that "Alice@X.com "
// and "alice@x.com" resolve to the same account.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// validEmail is a deliberately permissive check. The authoritative
// validation happens when we later send a verification email; here we
// only keep obvious garbage out of the database.
func validEmail(email string) bool {
	if len(email) > 254 {
		return false
	}
	_, err := mail.ParseAddress(email)
	return err == nil
}

// --- handlers ---

// Register creates a user and immediately issues tokens so the client
// does not need a second round-trip to /auth/login.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body is not valid json")
		return
	}

	email := normalizeEmail(req.Email)
	if !validEmail(email) {
		writeError(w, http.StatusBadRequest, "invalid_email", "email is malformed")
		return
	}
	if len(req.Password) < minPasswordLen {
		writeError(w, http.StatusBadRequest, "weak_password", "password must be at least 8 characters")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		h.logger.Error("hash password", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	user, err := h.store.CreateUser(r.Context(), email, hash)
	if err != nil {
		if errors.Is(err, storage.ErrEmailTaken) {
			writeError(w, http.StatusConflict, "email_taken", "email already registered")
			return
		}
		h.logger.Error("create user", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	accessToken, refreshToken, err := h.issueTokens(r.Context(), user.ID)
	if err != nil {
		h.logger.Error("issue tokens", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	writeJSON(w, http.StatusCreated, authResponse{
		User: userResponse{
			ID:        user.ID,
			Email:     user.Email,
			CreatedAt: user.CreatedAt,
		},
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(auth.AccessTokenTTL.Seconds()),
	})
}

// issueTokens generates a signed access JWT and an opaque refresh token,
// persisting only the hash of the latter.
func (h *AuthHandler) issueTokens(ctx context.Context, userID uuid.UUID) (access, refresh string, err error) {
	access, err = auth.IssueAccessToken(userID, h.jwtSecret)
	if err != nil {
		return "", "", err
	}

	plain, hash, err := auth.GenerateRefreshToken()
	if err != nil {
		return "", "", err
	}

	expiresAt := time.Now().Add(auth.RefreshTokenTTL)
	if _, err := h.store.CreateRefreshToken(ctx, userID, hash, expiresAt); err != nil {
		return "", "", err
	}

	return access, plain, nil
}

// dummyHash is a precomputed bcrypt hash of a random string, used to
// keep login timing constant whether or not the email exists.
// Never matches any real password.
var dummyHash = []byte("$2a$12$0000000000000000000000000000000000000000000000000000")


type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Login verifies credentials and issues a fresh token pair.
// It returns the same error code for unknown email and wrong password,
// and takes the same amount of time in both cases, to prevent user
// enumeration through responses or timing.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body is not valid json")
		return
	}

	email := normalizeEmail(req.Email)
	if email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "email and password are required")
		return
	}

	user, err := h.store.GetUserByEmail(r.Context(), email)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			// Spend the same time as a real bcrypt check would, then
			// fail with the same response. Do not leak that the user
			// is unknown.
			_ = auth.CheckPassword(string(dummyHash), req.Password)
			writeError(w, http.StatusUnauthorized, "invalid_credentials", "email or password is incorrect")
			return
		}
		h.logger.Error("get user by email", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	if err := auth.CheckPassword(user.PasswordHash, req.Password); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "email or password is incorrect")
		return
	}

	accessToken, refreshToken, err := h.issueTokens(r.Context(), user.ID)
	if err != nil {
		h.logger.Error("issue tokens", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	writeJSON(w, http.StatusOK, authResponse{
		User: userResponse{
			ID:        user.ID,
			Email:     user.Email,
			CreatedAt: user.CreatedAt,
		},
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(auth.AccessTokenTTL.Seconds()),
	})
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type refreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// Refresh rotates a refresh token. If the incoming token is valid, it is
// revoked and a new access+refresh pair is issued. If a revoked token is
// presented (reuse), all of the user's tokens are revoked as a safety
// measure, forcing re-login on every device.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body is not valid json")
		return
	}
	if req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "refresh_token is required")
		return
	}

	hash := auth.HashRefreshToken(req.RefreshToken)
	token, err := h.store.GetRefreshTokenByHash(r.Context(), hash)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "invalid_refresh_token", "")
			return
		}
		h.logger.Error("get refresh token", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	// Reuse detection: a revoked token is being presented. Either the
	// client raced itself, or the token was stolen. Treat as compromise
	// and revoke everything for this user.
	if token.RevokedAt != nil {
		h.logger.Warn("refresh token reuse detected", "user_id", token.UserID)
		if err := h.store.RevokeAllUserTokens(r.Context(), token.UserID); err != nil {
			h.logger.Error("revoke all user tokens", "err", err)
		}
		writeError(w, http.StatusUnauthorized, "invalid_refresh_token", "")
		return
	}

	if time.Now().After(token.ExpiresAt) {
		writeError(w, http.StatusUnauthorized, "invalid_refresh_token", "")
		return
	}

	if err := h.store.RevokeRefreshToken(r.Context(), token.ID); err != nil {
		h.logger.Error("revoke refresh token", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	accessToken, newRefresh, err := h.issueTokens(r.Context(), token.UserID)
	if err != nil {
		h.logger.Error("issue tokens", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	writeJSON(w, http.StatusOK, refreshResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefresh,
		TokenType:    "Bearer",
		ExpiresIn:    int(auth.AccessTokenTTL.Seconds()),
	})
}

// Logout revokes the presented refresh token. It is idempotent: a missing
// or already-revoked token returns 204 to avoid leaking which tokens exist.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body is not valid json")
		return
	}
	if req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "refresh_token is required")
		return
	}

	hash := auth.HashRefreshToken(req.RefreshToken)
	token, err := h.store.GetRefreshTokenByHash(r.Context(), hash)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.logger.Error("get refresh token", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	if err := h.store.RevokeRefreshToken(r.Context(), token.ID); err != nil {
		h.logger.Error("revoke refresh token", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
