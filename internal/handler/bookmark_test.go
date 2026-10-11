package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Carlos20052030/bookmarks-api/internal/domain"
	"github.com/Carlos20052030/bookmarks-api/internal/middleware"
	"github.com/Carlos20052030/bookmarks-api/internal/storage"
	"github.com/google/uuid"
)

// fakeBookmarkStore is a hand-written test double for BookmarkStore.
type fakeBookmarkStore struct {
	createErr   error
	getErr      error
	listErr     error
	updateErr   error
	deleteErr   error
	listTagsErr error
	attachErr   error
	detachErr   error

	// captured inputs
	createdUserID      uuid.UUID
	createdURL         string
	createdTitle       string
	lastAttachBookmark uuid.UUID
	lastAttachTag      uuid.UUID
	lastDetachBookmark uuid.UUID
	lastDetachTag      uuid.UUID

	// canned returns
	bookmark     domain.Bookmark
	bookmarkList []domain.Bookmark
	listTags     []domain.Tag
}

func (f *fakeBookmarkStore) CreateBookmark(_ context.Context, userID uuid.UUID, url, title string) (domain.Bookmark, error) {
	f.createdUserID = userID
	f.createdURL = url
	f.createdTitle = title
	if f.createErr != nil {
		return domain.Bookmark{}, f.createErr
	}
	return f.bookmark, nil
}

func (f *fakeBookmarkStore) GetBookmarkByID(_ context.Context, _, _ uuid.UUID) (domain.Bookmark, error) {
	if f.getErr != nil {
		return domain.Bookmark{}, f.getErr
	}
	return f.bookmark, nil
}

func (f *fakeBookmarkStore) ListBookmarks(_ context.Context, _ uuid.UUID, _, _ int, _ uuid.UUID) ([]domain.Bookmark, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.bookmarkList, nil
}

func (f *fakeBookmarkStore) UpdateBookmark(_ context.Context, _, _ uuid.UUID, _, _ string) (domain.Bookmark, error) {
	if f.updateErr != nil {
		return domain.Bookmark{}, f.updateErr
	}
	return f.bookmark, nil
}

func (f *fakeBookmarkStore) DeleteBookmark(_ context.Context, _, _ uuid.UUID) error {
	return f.deleteErr
}

func (f *fakeBookmarkStore) ListBookmarkTags(_ context.Context, _, _ uuid.UUID) ([]domain.Tag, error) {
	if f.listTagsErr != nil {
		return nil, f.listTagsErr
	}
	return f.listTags, nil
}

func (f *fakeBookmarkStore) AttachTag(_ context.Context, _, bookmarkID, tagID uuid.UUID) error {
	f.lastAttachBookmark = bookmarkID
	f.lastAttachTag = tagID
	return f.attachErr
}

func (f *fakeBookmarkStore) DetachTag(_ context.Context, _, bookmarkID, tagID uuid.UUID) error {
	f.lastDetachBookmark = bookmarkID
	f.lastDetachTag = tagID
	return f.detachErr
}

func newTestBookmarkHandler(fs *fakeBookmarkStore) *BookmarkHandler {
	return NewBookmarkHandler(fs, discardLogger())
}

// --- CRUD tests ---

func TestBookmarkCreate_Success(t *testing.T) {
	userID := uuid.New()
	fs := &fakeBookmarkStore{bookmark: domain.Bookmark{ID: uuid.New(), URL: "https://x.com", Title: "X"}}
	h := newTestBookmarkHandler(fs)

	req := httptest.NewRequest(http.MethodPost, "/bookmarks", bytes.NewBufferString(`{"url":"https://x.com","title":"X"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(middleware.WithUserID(req.Context(), userID))
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if fs.createdUserID != userID {
		t.Fatalf("userID not propagated")
	}
	if fs.createdURL != "https://x.com" {
		t.Fatalf("url = %q", fs.createdURL)
	}
}

func TestBookmarkCreate_InvalidURL(t *testing.T) {
	h := newTestBookmarkHandler(&fakeBookmarkStore{})
	req := httptest.NewRequest(http.MethodPost, "/bookmarks", bytes.NewBufferString(`{"url":"ftp://x"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(middleware.WithUserID(req.Context(), uuid.New()))
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	assertErrorCode(t, rec, "invalid_url")
}

func TestBookmarkGet_Success(t *testing.T) {
	bmID := uuid.New()
	fs := &fakeBookmarkStore{bookmark: domain.Bookmark{ID: bmID, URL: "https://x.com"}}
	h := newTestBookmarkHandler(fs)

	req := httptest.NewRequest(http.MethodGet, "/bookmarks/"+bmID.String(), nil)
	req.SetPathValue("id", bmID.String())
	req = req.WithContext(middleware.WithUserID(req.Context(), uuid.New()))
	rec := httptest.NewRecorder()
	h.Get(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestBookmarkGet_NotFound(t *testing.T) {
	fs := &fakeBookmarkStore{getErr: storage.ErrNotFound}
	h := newTestBookmarkHandler(fs)
	id := uuid.New()

	req := httptest.NewRequest(http.MethodGet, "/bookmarks/"+id.String(), nil)
	req.SetPathValue("id", id.String())
	req = req.WithContext(middleware.WithUserID(req.Context(), uuid.New()))
	rec := httptest.NewRecorder()
	h.Get(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestBookmarkList_InvalidLimit(t *testing.T) {
	h := newTestBookmarkHandler(&fakeBookmarkStore{})

	req := httptest.NewRequest(http.MethodGet, "/bookmarks?limit=999", nil)
	req = req.WithContext(middleware.WithUserID(req.Context(), uuid.New()))
	rec := httptest.NewRecorder()
	h.List(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	assertErrorCode(t, rec, "invalid_limit")
}

func TestBookmarkDelete_Success(t *testing.T) {
	fs := &fakeBookmarkStore{}
	h := newTestBookmarkHandler(fs)
	id := uuid.New()

	req := httptest.NewRequest(http.MethodDelete, "/bookmarks/"+id.String(), nil)
	req.SetPathValue("id", id.String())
	req = req.WithContext(middleware.WithUserID(req.Context(), uuid.New()))
	rec := httptest.NewRecorder()
	h.Delete(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

// --- AttachTag / DetachTag tests ---

func TestAttachTag_Success(t *testing.T) {
	fs := &fakeBookmarkStore{}
	h := newTestBookmarkHandler(fs)
	bmID, tagID := uuid.New(), uuid.New()

	req := httptest.NewRequest(http.MethodPost, "/bookmarks/"+bmID.String()+"/tags/"+tagID.String(), nil)
	req.SetPathValue("id", bmID.String())
	req.SetPathValue("tagID", tagID.String())
	req = req.WithContext(middleware.WithUserID(req.Context(), uuid.New()))
	rec := httptest.NewRecorder()
	h.AttachTag(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if fs.lastAttachBookmark != bmID {
		t.Fatalf("bookmarkID not propagated")
	}
	if fs.lastAttachTag != tagID {
		t.Fatalf("tagID not propagated")
	}
}

func TestAttachTag_NotFound(t *testing.T) {
	fs := &fakeBookmarkStore{attachErr: storage.ErrNotFound}
	h := newTestBookmarkHandler(fs)
	bmID, tagID := uuid.New(), uuid.New()

	req := httptest.NewRequest(http.MethodPost, "/bookmarks/"+bmID.String()+"/tags/"+tagID.String(), nil)
	req.SetPathValue("id", bmID.String())
	req.SetPathValue("tagID", tagID.String())
	req = req.WithContext(middleware.WithUserID(req.Context(), uuid.New()))
	rec := httptest.NewRecorder()
	h.AttachTag(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	assertErrorCode(t, rec, "not_found")
}

func TestAttachTag_MalformedTagID(t *testing.T) {
	h := newTestBookmarkHandler(&fakeBookmarkStore{})
	bmID := uuid.New()

	req := httptest.NewRequest(http.MethodPost, "/bookmarks/"+bmID.String()+"/tags/not-a-uuid", nil)
	req.SetPathValue("id", bmID.String())
	req.SetPathValue("tagID", "not-a-uuid")
	req = req.WithContext(middleware.WithUserID(req.Context(), uuid.New()))
	rec := httptest.NewRecorder()
	h.AttachTag(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestAttachTag_Unauthorized(t *testing.T) {
	h := newTestBookmarkHandler(&fakeBookmarkStore{})
	bmID, tagID := uuid.New(), uuid.New()

	req := httptest.NewRequest(http.MethodPost, "/bookmarks/"+bmID.String()+"/tags/"+tagID.String(), nil)
	req.SetPathValue("id", bmID.String())
	req.SetPathValue("tagID", tagID.String())
	rec := httptest.NewRecorder()
	h.AttachTag(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestDetachTag_Success(t *testing.T) {
	fs := &fakeBookmarkStore{}
	h := newTestBookmarkHandler(fs)
	bmID, tagID := uuid.New(), uuid.New()

	req := httptest.NewRequest(http.MethodDelete, "/bookmarks/"+bmID.String()+"/tags/"+tagID.String(), nil)
	req.SetPathValue("id", bmID.String())
	req.SetPathValue("tagID", tagID.String())
	req = req.WithContext(middleware.WithUserID(req.Context(), uuid.New()))
	rec := httptest.NewRecorder()
	h.DetachTag(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
	if fs.lastDetachBookmark != bmID || fs.lastDetachTag != tagID {
		t.Fatal("IDs not propagated")
	}
}

func TestDetachTag_NotFound(t *testing.T) {
	fs := &fakeBookmarkStore{detachErr: storage.ErrNotFound}
	h := newTestBookmarkHandler(fs)
	bmID, tagID := uuid.New(), uuid.New()

	req := httptest.NewRequest(http.MethodDelete, "/bookmarks/"+bmID.String()+"/tags/"+tagID.String(), nil)
	req.SetPathValue("id", bmID.String())
	req.SetPathValue("tagID", tagID.String())
	req = req.WithContext(middleware.WithUserID(req.Context(), uuid.New()))
	rec := httptest.NewRecorder()
	h.DetachTag(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// Sanity: unused import guard (encoding/json used in some tests above).
var _ = json.Marshal
var _ = errors.New