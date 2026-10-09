package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteJSON_SetsContentTypeAndStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusCreated, map[string]string{"hello": "world"})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("content-type = %q", got)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}
	if body["hello"] != "world" {
		t.Fatalf("body = %v, want hello=world", body)
	}
}

func TestWriteJSON_NilPayload(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusNoContent, nil)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("body should be empty for nil payload, got %q", rec.Body.String())
	}
}

func TestWriteError_StableShape(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusBadRequest, "invalid_email", "email is malformed")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}

	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}
	if body.Error != "invalid_email" {
		t.Fatalf("error = %q, want invalid_email", body.Error)
	}
	if body.Message != "email is malformed" {
		t.Fatalf("message = %q", body.Message)
	}
}

func TestWriteError_OmitsEmptyMessage(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusUnauthorized, "unauthorized", "")

	// The "message" key should be absent (omitempty), not an empty string.
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, exists := raw["message"]; exists {
		t.Fatalf("message should be omitted when empty, got: %s", rec.Body.String())
	}
}
