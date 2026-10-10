package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Carlos20052030/bookmarks-api/internal/domain"
	"github.com/Carlos20052030/bookmarks-api/internal/middleware"
	"github.com/Carlos20052030/bookmarks-api/internal/storage"
	"github.com/google/uuid"
)

const (
	defaultLimit = 20
	maxLimit     = 100
)

// BookmarkStore is the subset of storage operations the bookmark handler needs.
type BookmarkStore interface {
	CreateBookmark(ctx context.Context, userID uuid.UUID, url, title string) (domain.Bookmark, error)
	GetBookmarkByID(ctx context.Context, userID, id uuid.UUID) (domain.Bookmark, error)
	ListBookmarks(ctx context.Context, userID uuid.UUID, limit, offset int, tagID uuid.UUID) ([]domain.Bookmark, error)
	UpdateBookmark(ctx context.Context, userID, id uuid.UUID, url, title string) (domain.Bookmark, error)
	DeleteBookmark(ctx context.Context, userID, id uuid.UUID) error
	ListBookmarkTags(ctx context.Context, userID, bookmarkID uuid.UUID) ([]domain.Tag, error)
}

type BookmarkHandler struct {
	store  BookmarkStore
	logger *slog.Logger
}

func NewBookmarkHandler(store BookmarkStore, logger *slog.Logger) *BookmarkHandler {
	return &BookmarkHandler{store: store, logger: logger}
}

// --- request / response types ---

type bookmarkRequest struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

type tagResponse struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type bookmarkResponse struct {
	ID        uuid.UUID     `json:"id"`
	URL       string        `json:"url"`
	Title     string        `json:"title"`
	Tags      []tagResponse `json:"tags"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

type bookmarkListResponse struct {
	Bookmarks []bookmarkResponse `json:"bookmarks"`
	Limit     int                `json:"limit"`
	Offset    int                `json:"offset"`
}

// --- helpers ---

// toBookmarkResponse converts a domain.Bookmark into the API shape.
// Tags may be nil for list responses.
func toBookmarkResponse(b domain.Bookmark, tags []domain.Tag) bookmarkResponse {
	out := bookmarkResponse{
		ID:        b.ID,
		URL:       b.URL,
		Title:     b.Title,
		Tags:      make([]tagResponse, 0, len(tags)),
		CreatedAt: b.CreatedAt,
		UpdatedAt: b.UpdatedAt,
	}
	for _, t := range tags {
		out.Tags = append(out.Tags, tagResponse{ID: t.ID, Name: t.Name})
	}
	return out
}

// validURL accepts only absolute http/https URLs.
func validURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return u.Host != ""
}

// parseID extracts a UUID from a path parameter. Writes 404 and returns
// false when the path value is missing or malformed — a malformed ID
// cannot possibly identify a real resource.
func parseID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	raw := r.PathValue(name)
	id, err := uuid.Parse(raw)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "")
		return uuid.Nil, false
	}
	return id, true
}

// --- handlers ---

// Create handles POST /bookmarks.
func (h *BookmarkHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == uuid.Nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}

	var req bookmarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body is not valid json")
		return
	}

	req.URL = strings.TrimSpace(req.URL)
	req.Title = strings.TrimSpace(req.Title)
	if !validURL(req.URL) {
		writeError(w, http.StatusBadRequest, "invalid_url", "url must be an absolute http or https URL")
		return
	}

	bm, err := h.store.CreateBookmark(r.Context(), userID, req.URL, req.Title)
	if err != nil {
		h.logger.Error("create bookmark", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	writeJSON(w, http.StatusCreated, toBookmarkResponse(bm, nil))
}

// Get handles GET /bookmarks/{id}.
func (h *BookmarkHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == uuid.Nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}

	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}

	bm, err := h.store.GetBookmarkByID(r.Context(), userID, id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "")
			return
		}
		h.logger.Error("get bookmark", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	tags, err := h.store.ListBookmarkTags(r.Context(), userID, bm.ID)
	if err != nil {
		h.logger.Error("list bookmark tags", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	writeJSON(w, http.StatusOK, toBookmarkResponse(bm, tags))
}

// List handles GET /bookmarks with pagination and optional tag filter.
func (h *BookmarkHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == uuid.Nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}

	limit := defaultLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxLimit {
			writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be between 1 and 100")
			return
		}
		limit = n
	}

	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "invalid_offset", "offset must be >= 0")
			return
		}
		offset = n
	}

	var tagID uuid.UUID
	if v := r.URL.Query().Get("tag"); v != "" {
		parsed, err := uuid.Parse(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_tag", "tag must be a valid uuid")
			return
		}
		tagID = parsed
	}

	bookmarks, err := h.store.ListBookmarks(r.Context(), userID, limit, offset, tagID)
	if err != nil {
		h.logger.Error("list bookmarks", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	out := bookmarkListResponse{
		Bookmarks: make([]bookmarkResponse, 0, len(bookmarks)),
		Limit:     limit,
		Offset:    offset,
	}
	for _, b := range bookmarks {
		out.Bookmarks = append(out.Bookmarks, toBookmarkResponse(b, nil))
	}

	writeJSON(w, http.StatusOK, out)
}

// Update handles PUT /bookmarks/{id}.
func (h *BookmarkHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == uuid.Nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}

	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}

	var req bookmarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body is not valid json")
		return
	}

	req.URL = strings.TrimSpace(req.URL)
	req.Title = strings.TrimSpace(req.Title)
	if !validURL(req.URL) {
		writeError(w, http.StatusBadRequest, "invalid_url", "url must be an absolute http or https URL")
		return
	}

	bm, err := h.store.UpdateBookmark(r.Context(), userID, id, req.URL, req.Title)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "")
			return
		}
		h.logger.Error("update bookmark", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	writeJSON(w, http.StatusOK, toBookmarkResponse(bm, nil))
}

// Delete handles DELETE /bookmarks/{id}.
func (h *BookmarkHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == uuid.Nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}

	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}

	if err := h.store.DeleteBookmark(r.Context(), userID, id); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "")
			return
		}
		h.logger.Error("delete bookmark", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}