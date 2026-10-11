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

// fakeTagStore is a hand-written test double for TagStore.
type fakeTagStore struct {
	createErr     error
	listErr       error
	deleteErr     error
	createdName   string
	createdUserID uuid.UUID
	deletedID     uuid.UUID
	list          []domain.Tag
}

func (f *fakeTagStore) CreateTag(_ context.Context, userID uuid.UUID, name string) (domain.Tag, error) {
	f.createdUserID = userID
	f.createdName = name
	if f.createErr != nil {
		return domain.Tag{}, f.createErr
	}
	return domain.Tag{ID: uuid.New(), UserID: userID, Name: name}, nil
}

func (f *fakeTagStore) ListTags(_ context.Context, _ uuid.UUID) ([]domain.Tag, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.list, nil
}

func (f *fakeTagStore) DeleteTag(_ context.Context, _, id uuid.UUID) error {
	f.deletedID = id
	return f.deleteErr
}

func newTestTagHandler(fs *fakeTagStore) *TagHandler {
	return NewTagHandler(fs, discardLogger())
}

// postAsUser issues a POST with the given user ID injected into context,
// simulating what the Auth middleware does in production.
func postAsUser(t *testing.T, h http.HandlerFunc, body string, userID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/tags", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(middleware.WithUserID(req.Context(), userID))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// --- tests ---

func TestTagCreate_Success(t *testing.T) {
	fs := &fakeTagStore{}
	h := newTestTagHandler(fs)
	userID := uuid.New()

	rec := postAsUser(t, h.Create, `{"name":"go"}`, userID)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp tagResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Name != "go" {
		t.Fatalf("name = %q, want go", resp.Name)
	}
	if fs.createdUserID != userID {
		t.Fatalf("userID not propagated: got %v, want %v", fs.createdUserID, userID)
	}
	if fs.createdName != "go" {
		t.Fatalf("name not normalized: %q", fs.createdName)
	}
}

func TestTagCreate_TrimsWhitespace(t *testing.T) {
	fs := &fakeTagStore{}
	h := newTestTagHandler(fs)

	rec := postAsUser(t, h.Create, `{"name":"  go  "}`, uuid.New())
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d", rec.Code)
	}
	if fs.createdName != "go" {
		t.Fatalf("name = %q, want trimmed 'go'", fs.createdName)
	}
}

func TestTagCreate_EmptyName(t *testing.T) {
	h := newTestTagHandler(&fakeTagStore{})
	rec := postAsUser(t, h.Create, `{"name":"   "}`, uuid.New())

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	assertErrorCode(t, rec, "invalid_name")
}

func TestTagCreate_NameTooLong(t *testing.T) {
	h := newTestTagHandler(&fakeTagStore{})
	long := make([]byte, maxTagNameLen+1)
	for i := range long {
		long[i] = 'a'
	}
	rec := postAsUser(t, h.Create, `{"name":"`+string(long)+`"}`, uuid.New())

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	assertErrorCode(t, rec, "invalid_name")
}

func TestTagCreate_InvalidJSON(t *testing.T) {
	h := newTestTagHandler(&fakeTagStore{})
	rec := postAsUser(t, h.Create, `{not json`, uuid.New())

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	assertErrorCode(t, rec, "invalid_json")
}

func TestTagCreate_NameTaken(t *testing.T) {
	fs := &fakeTagStore{createErr: storage.ErrTagNameTaken}
	h := newTestTagHandler(fs)

	rec := postAsUser(t, h.Create, `{"name":"go"}`, uuid.New())
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	assertErrorCode(t, rec, "tag_name_taken")
}

func TestTagCreate_StoreFailure(t *testing.T) {
	fs := &fakeTagStore{createErr: errors.New("db down")}
	h := newTestTagHandler(fs)

	rec := postAsUser(t, h.Create, `{"name":"go"}`, uuid.New())
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	assertErrorCode(t, rec, "internal_error")
}

func TestTagCreate_Unauthorized(t *testing.T) {
	h := newTestTagHandler(&fakeTagStore{})
	// no user in context (uuid.Nil)
	rec := postAsUser(t, h.Create, `{"name":"go"}`, uuid.Nil)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	assertErrorCode(t, rec, "unauthorized")
}

func TestTagList_Empty(t *testing.T) {
	fs := &fakeTagStore{list: nil}
	h := newTestTagHandler(fs)

	req := httptest.NewRequest(http.MethodGet, "/tags", nil)
	req = req.WithContext(middleware.WithUserID(req.Context(), uuid.New()))
	rec := httptest.NewRecorder()
	h.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Tags []tagResponse `json:"tags"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Tags == nil {
		t.Fatal("tags should be an empty array, not null")
	}
	if len(body.Tags) != 0 {
		t.Fatalf("len = %d, want 0", len(body.Tags))
	}
}

func TestTagList_WithItems(t *testing.T) {
	fs := &fakeTagStore{list: []domain.Tag{
		{ID: uuid.New(), Name: "go"},
		{ID: uuid.New(), Name: "rust"},
	}}
	h := newTestTagHandler(fs)

	req := httptest.NewRequest(http.MethodGet, "/tags", nil)
	req = req.WithContext(middleware.WithUserID(req.Context(), uuid.New()))
	rec := httptest.NewRecorder()
	h.List(rec, req)

	var body struct {
		Tags []tagResponse `json:"tags"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Tags) != 2 {
		t.Fatalf("len = %d, want 2", len(body.Tags))
	}
}

func TestTagDelete_Success(t *testing.T) {
	fs := &fakeTagStore{}
	h := newTestTagHandler(fs)
	id := uuid.New()

	req := httptest.NewRequest(http.MethodDelete, "/tags/"+id.String(), nil)
	req.SetPathValue("id", id.String())
	req = req.WithContext(middleware.WithUserID(req.Context(), uuid.New()))
	rec := httptest.NewRecorder()
	h.Delete(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if fs.deletedID != id {
		t.Fatalf("deletedID = %v, want %v", fs.deletedID, id)
	}
}

func TestTagDelete_NotFound(t *testing.T) {
	fs := &fakeTagStore{deleteErr: storage.ErrNotFound}
	h := newTestTagHandler(fs)
	id := uuid.New()

	req := httptest.NewRequest(http.MethodDelete, "/tags/"+id.String(), nil)
	req.SetPathValue("id", id.String())
	req = req.WithContext(middleware.WithUserID(req.Context(), uuid.New()))
	rec := httptest.NewRecorder()
	h.Delete(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	assertErrorCode(t, rec, "not_found")
}

func TestTagDelete_MalformedID(t *testing.T) {
	h := newTestTagHandler(&fakeTagStore{})

	req := httptest.NewRequest(http.MethodDelete, "/tags/not-a-uuid", nil)
	req.SetPathValue("id", "not-a-uuid")
	req = req.WithContext(middleware.WithUserID(req.Context(), uuid.New()))
	rec := httptest.NewRecorder()
	h.Delete(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestTagDelete_Unauthorized(t *testing.T) {
	h := newTestTagHandler(&fakeTagStore{})
	id := uuid.New()

	req := httptest.NewRequest(http.MethodDelete, "/tags/"+id.String(), nil)
	req.SetPathValue("id", id.String())
	rec := httptest.NewRecorder()
	h.Delete(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}