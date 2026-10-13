package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// bucket is a token bucket for one key (IP, email, etc).
type bucket struct {
	tokens     float64
	lastRefill time.Time
}

// Limiter implements a token-bucket rate limiter keyed by an arbitrary
// string. Safe for concurrent use. Buckets are cleaned lazily by a
// background goroutine.
type Limiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	capacity float64
	refill   float64
	interval time.Duration
}

// NewLimiter creates a limiter with the given burst capacity and refill
// rate (tokens per second).
func NewLimiter(capacity int, tokensPerSecond float64) *Limiter {
	return &Limiter{
		buckets:  make(map[string]*bucket),
		capacity: float64(capacity),
		refill:   tokensPerSecond,
		interval: 5 * time.Minute,
	}
}

// Allow reports whether the request for key may proceed. When it returns
// false, retryAfter is the minimum time before the next token is available.
func (l *Limiter) Allow(key string) (allowed bool, retryAfter time.Duration) {
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.capacity, lastRefill: now}
		l.buckets[key] = b
	} else {
		elapsed := now.Sub(b.lastRefill).Seconds()
		b.tokens += elapsed * l.refill
		if b.tokens > l.capacity {
			b.tokens = l.capacity
		}
		b.lastRefill = now
	}

	if b.tokens < 1 {
		missing := 1 - b.tokens
		wait := time.Duration(missing/l.refill*1000) * time.Millisecond
		return false, wait
	}

	b.tokens--
	return true, 0
}

// StartCleaner runs a goroutine that removes idle buckets until ctx is
// cancelled. Buckets untouched for 2x interval are dropped.
func (l *Limiter) StartCleaner(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(l.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				l.cleanup()
			}
		}
	}()
}

func (l *Limiter) cleanup() {
	cutoff := time.Now().Add(-2 * l.interval)

	l.mu.Lock()
	defer l.mu.Unlock()
	for k, b := range l.buckets {
		if b.lastRefill.Before(cutoff) {
			delete(l.buckets, k)
		}
	}
}

// --- keys ---

// KeyFunc extracts the rate-limit key from a request. Return "" to skip
// limiting for that request.
type KeyFunc func(*http.Request) string

// IPKey returns the client's IP address as a rate-limit key. IPv4
// addresses are used as-is. IPv6 addresses are collapsed to their /64
// prefix so a single client cannot bypass limits by rotating through its
// own range (a /64 contains billions of addresses).
//
// X-Forwarded-For is deliberately ignored: without a trusted reverse proxy
// in front, the header is spoofable by the client.
func IPKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return host
	}
	return normalizeIP(ip)
}

// normalizeIP returns a string form of the IP suitable for use as a
// rate-limit key. IPv4 addresses are returned as-is; IPv6 addresses are
// masked to their /64 prefix. This follows the convention used by AWS and
// Cloudflare for per-client accounting.
func normalizeIP(ip net.IP) string {
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	v6 := ip.To16()
	if v6 == nil {
		return ip.String()
	}
	masked := make(net.IP, net.IPv6len)
	copy(masked, v6)
	for i := 8; i < 16; i++ {
		masked[i] = 0
	}
	return masked.String()
}

// EmailKeyFromBody extracts the "email" field from a JSON request body and
// returns it as a rate-limit key, prefixed with "email:" to avoid colliding
// with IP keys in the same limiter. The body is buffered and restored so
// the downstream handler can read it again.
//
// Returns "" when the body is missing, too large, or not JSON with an
// email field. In those cases the caller skips the email-based check and
// lets the handler produce the appropriate error.
func EmailKeyFromBody(r *http.Request) string {
	if r.Body == nil || r.ContentLength > maxBodyBytes {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		return ""
	}
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(body))

	var req struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return ""
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" {
		return ""
	}
	return "email:" + email
}

const maxBodyBytes = 4096

// --- middleware ---

// RateLimit returns a middleware that limits requests per key. When the
// limit is exceeded, it replies 429 with Retry-After and does not call
// the next handler. A key of "" skips limiting for that request.
func RateLimit(limiter *Limiter, keyFn KeyFunc) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := keyFn(r)
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			allowed, retryAfter := limiter.Allow(key)
			if !allowed {
				seconds := int(retryAfter.Seconds())
				if seconds < 1 {
					seconds = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(seconds))
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":"too_many_requests"}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}