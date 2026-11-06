package email

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResendClient_SendPasswordResetEmail(t *testing.T) {
	ctx := context.Background()
	// Create a test server that mimics the Resend API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Recover from any panics in the handler
		defer func() {
			if rec := recover(); rec != nil {
				t.Logf("Handler panic: %v", rec)
			}
		}()

		// Verify the request
		require.Equal(t, "POST", r.Method)
		require.Equal(t, "/emails", r.URL.Path)
		require.Equal(t, "Bearer test-api-key", r.Header.Get("Authorization"))
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Read the body
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		// Verify the email content
		assert.Contains(t, string(body), "test@example.com")
		assert.Contains(t, string(body), "https://nuntix.com.br/reset-password?token=")

		// Return success
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id": "test-email-id"}`))
	}))
	defer server.Close()

	client := NewResendClient("test-api-key", server.URL)

	err := client.SendPasswordResetEmail(ctx, "test@example.com", "test-token-123")
	require.NoError(t, err)
}

func TestResendClient_SendPasswordResetEmail_APIError(t *testing.T) {
	ctx := context.Background()
	// Create a test server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"message": "Invalid email"}`))
	}))
	defer server.Close()

	client := NewResendClient("test-api-key", server.URL)

	err := client.SendPasswordResetEmail(ctx, "test@example.com", "test-token-123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid email")
}

func TestResendClient_SendPasswordResetEmail_NetworkError(t *testing.T) {
	ctx := context.Background()
	// Use an invalid URL to simulate network error
	client := NewResendClient("test-api-key", "http://invalid-url-that-does-not-exist.local")

	err := client.SendPasswordResetEmail(ctx, "test@example.com", "test-token-123")
	require.Error(t, err)
}