package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// errorBody is the stable JSON shape returned for every error.
// Keeping this shape consistent lets clients parse errors uniformly.
type errorBody struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// writeJSON serializes payload as JSON and writes it with the given status.
// It sets Content-Type before WriteHeader so headers are not flushed early.
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Error("write json response", "err", err)
	}
}

// writeError writes an error response with a stable JSON shape.
// code is a short machine-readable identifier (snake_case).
// message is optional human-readable detail.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Error: code, Message: message})
}
