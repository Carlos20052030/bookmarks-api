package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLimiter_AllowsUpToCapacity(t *testing.T) {
	l := NewLimiter(3, 0.001) // 3 tokens, negligible refill

	for i := 0; i < 3; i++ {
		ok, _ := l.Allow("user")
		if !ok {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	ok, _ := l.Allow("user")
	if ok {
		t.Fatal("4th request should be blocked")
	}
}

func TestLimiter_RefillsOverTime(t *testing.T) {
	l := NewLimiter(1, 100) // 100 tokens/second → refill fast

	if ok, _ := l.Allow("user"); !ok {
		t.Fatal("first request should be allowed")
	}
	if ok, _ := l.Allow("user"); ok {
		t.Fatal("second request should be blocked")
	}

	time.Sleep(20 * time.Millisecond) // 100/s * 0.02s = 2 tokens

	if ok, _ := l.Allow("user"); !ok {
		t.Fatal("request after refill should be allowed")
	}
}

func TestLimiter_SeparateKeys(t *testing.T) {
	l := NewLimiter(1, 0.001)

	if ok, _ := l.Allow("alice"); !ok {
		t.Fatal("alice first should pass")
	}
	if ok, _ := l.Allow("bob"); !ok {
		t.Fatal("bob first should pass — different key")
	}
	if ok, _ := l.Allow("alice"); ok {
		t.Fatal("alice second should be blocked")
	}
}

func TestLimiter_RetryAfterPositive(t *testing.T) {
	l := NewLimiter(1, 0.1) // 1 token, 0.1/s refill → 10s to next token

	l.Allow("user")
	ok, wait := l.Allow("user")
	if ok {
		t.Fatal("expected block")
	}
	if wait <= 0 {
		t.Fatalf("retryAfter should be positive, got %v", wait)
	}
}

func TestRateLimitMiddleware_BlocksWith429(t *testing.T) {
	l := NewLimiter(2, 0.001)
	handler := RateLimit(l, func(_ *http.Request) string { return "fixed" })(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.RemoteAddr = "1.2.3.4:5678"
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200", i+1, rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("3rd request: status = %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("Retry-After header missing")
	}
}

func TestIPKey_ExtractsHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.1:54321"
	if got := IPKey(req); got != "192.0.2.1" {
		t.Fatalf("got %q, want 192.0.2.1", got)
	}
}

func TestIPKey_IgnoresXForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.1:54321"
	req.Header.Set("X-Forwarded-For", "10.0.0.1")
	if got := IPKey(req); got != "192.0.2.1" {
		t.Fatalf("IPKey trusted X-Forwarded-For: got %q", got)
	}
}

func TestLimiter_StartCleanerStops(t *testing.T) {
	l := NewLimiter(1, 1)
	ctx, cancel := context.WithCancel(context.Background())
	l.StartCleaner(ctx)
	cancel()
	// Give the goroutine time to observe the cancellation.
	time.Sleep(10 * time.Millisecond)
}