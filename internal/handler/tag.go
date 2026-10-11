package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Carlos20052030/bookmarks-api/internal/domain"
	"github.com/Carlos20052030/bookmarks-api/internal/middleware"
	"github.com/Carlos20052030/bookmarks-api/internal/storage"
	"github.com/google/uuid"
)

const (
	minTagNameLen = 1
	maxTagNameLen = 50
)

// TagStore is the subset of storage operations the tag handler needs.
type TagStore interface {
	CreateTag(ctx context.Context, userID uuid.UUID, name string) (domain.Tag, error)
	ListTags(ctx context.Context, userID uuid.UUID) ([]domain.Tag, error)
	DeleteTag(ctx context.Context, userID, id uuid.UUID) error
}

type TagHandler struct {
	store  TagStore
	logger *slog.Logger
}

func NewTagHandler(store TagStore, logger *slog.Logger) *TagHandler {
	return &TagHandler{store: store, logger: logger}
}

// --- request / response types ---

type tagCreateRequest struct {
	Name string `json:"name"`
}

// --- handlers ---

// Create handles POST /tags.
func (h *TagHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == uuid.Nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}

	var req tagCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body is not valid json")
		return
	}

	name := strings.TrimSpace(req.Name)
	if len(name) < minTagNameLen {
		writeError(w, http.StatusBadRequest, "invalid_name", "name is required")
		return
	}
	if len(name) > maxTagNameLen {
		writeError(w, http.StatusBadRequest, "invalid_name", "name must be at most 50 characters")
		return
	}

	tag, err := h.store.CreateTag(r.Context(), userID, name)
	if err != nil {
		if errors.Is(err, storage.ErrTagNameTaken) {
			writeError(w, http.StatusConflict, "tag_name_taken", "a tag with this name already exists")
			return
		}
		h.logger.Error("create tag", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	writeJSON(w, http.StatusCreated, tagResponse{
		ID:   tag.ID,
		Name: tag.Name,
	})
}

// List handles GET /tags.
func (h *TagHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == uuid.Nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}

	tags, err := h.store.ListTags(r.Context(), userID)
	if err != nil {
		h.logger.Error("list tags", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	out := make([]tagResponse, 0, len(tags))
	for _, t := range tags {
		out = append(out, tagResponse{ID: t.ID, Name: t.Name})
	}

	writeJSON(w, http.StatusOK, map[string]any{"tags": out})
}

// Delete handles DELETE /tags/{id}.
func (h *TagHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == uuid.Nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}

	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}

	if err := h.store.DeleteTag(r.Context(), userID, id); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "")
			return
		}
		h.logger.Error("delete tag", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}