package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/Carlos20052030/bookmarks-api/internal/middleware"
	"github.com/Carlos20052030/bookmarks-api/internal/storage"
	"github.com/google/uuid"
)

type APIKeyHandler struct {
	Store *storage.Store
}

// NewAPIKeyHandler creates a new APIKeyHandler with the given store.
func NewAPIKeyHandler(store *storage.Store) *APIKeyHandler {
	return &APIKeyHandler{
		Store: store,
	}
}

// GenerateRandomKey creates a raw key and its SHA-256 hash.
func GenerateRandomKey() (string, string, string) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	rawKey := "nuntix_live_" + hex.EncodeToString(b)
	prefix := "nuntix_live_"

	hash := sha256.Sum256([]byte(rawKey))
	hashString := hex.EncodeToString(hash[:])

	return rawKey, prefix, hashString
}

// Create handles the POST /api-keys request.
func (h *APIKeyHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == uuid.Nil {
		http.Error(w, "Invalid user ID", http.StatusUnauthorized)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		req.Name = "Default Key"
	}

	rawKey, prefix, hash := GenerateRandomKey()

	key, err := h.Store.CreateAPIKey(r.Context(), userID, req.Name, prefix, hash)
	if err != nil {
		http.Error(w, "Failed to create key", http.StatusInternalServerError)
		return
	}

	// Return the raw key ONLY ONCE during creation
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"key":        key,
		"raw_key":    rawKey, // Client must save this
		"created_at": time.Now(),
	})
}

// List handles the GET /api-keys request.
func (h *APIKeyHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == uuid.Nil {
		http.Error(w, "Invalid user ID", http.StatusUnauthorized)
		return
	}

	keys, err := h.Store.ListAPIKeys(r.Context(), userID)
	if err != nil {
		http.Error(w, "Failed to list keys", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(keys)
}

// Revoke handles the DELETE /api-keys/{id} request.
func (h *APIKeyHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == uuid.Nil {
		http.Error(w, "Invalid user ID", http.StatusUnauthorized)
		return
	}

	keyIDStr := r.PathValue("id")
	keyID, err := uuid.Parse(keyIDStr)
	if err != nil {
		http.Error(w, "Invalid key ID", http.StatusBadRequest)
		return
	}

	err = h.Store.RevokeAPIKey(r.Context(), userID, keyID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
