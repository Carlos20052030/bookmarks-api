package middleware

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestNormalizeIP_IPv4(t *testing.T) {
	for _, tc := range []struct {
		in, want string
	}{
		{"192.0.2.1", "192.0.2.1"},
		{"8.8.8.8", "8.8.8.8"},
	} {
		got := normalizeIP(net.ParseIP(tc.in))
		if got != tc.want {
			t.Errorf("normalizeIP(%s) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeIP_IPv6CollapsesToSlash64(t *testing.T) {
	// Two addresses in the same /64 must collapse to the same key.
	a := normalizeIP(net.ParseIP("2001:db8:abcd:1234::1"))
	b := normalizeIP(net.ParseIP("2001:db8:abcd:1234::ffff:ffff"))
	if a != b {
		t.Fatalf("same /64 produced different keys: %s vs %s", a, b)
	}

	// A different /64 must produce a different key.
	c := normalizeIP(net.ParseIP("2001:db8:abcd:5678::1"))
	if a == c {
		t.Fatalf("different /64 collapsed: %s", a)
	}
}

func TestEmailKeyFromBody_ExtractsAndNormalizes(t *testing.T) {
	body := `{"email":"  Alice@Example.COM  ","password":"x"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	got := EmailKeyFromBody(req)
	if got != "email:alice@example.com" {
		t.Fatalf("got %q", got)
	}

	// Body must be readable again by the handler.
	restored, _ := io.ReadAll(req.Body)
	if string(restored) != body {
		t.Fatalf("body not restored: %q", string(restored))
	}
}

func TestEmailKeyFromBody_MalformedJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{not json`))
	if got := EmailKeyFromBody(req); got != "" {
		t.Fatalf("malformed JSON should return empty key, got %q", got)
	}
}

func TestEmailKeyFromBody_MissingEmail(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"password":"x"}`))
	if got := EmailKeyFromBody(req); got != "" {
		t.Fatalf("missing email should return empty key, got %q", got)
	}
}