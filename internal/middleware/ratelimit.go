package middleware

import (
	"context"
	"net"
	"net/http"
	"strconv"
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

// --- middleware ---

// KeyFunc extracts the rate-limit key from a request. Return "" to skip
// limiting for that request.
type KeyFunc func(*http.Request) string

// IPKey returns the client's IP address. It trusts RemoteAddr only and
// ignores X-Forwarded-For, which can be spoofed without a trusted proxy.
func IPKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// RateLimit returns a middleware that limits requests per key. When the
// limit is exceeded, it replies 429 with Retry-After and does not call
// the next handler.
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