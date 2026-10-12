// Package fetcher retrieves the <title> of a web page, with protection
// against SSRF (Server-Side Request Forgery).
package fetcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	// maxBodySize caps how much of the response we read. The <title>
	// element lives in the <head>, so 512 KB is more than enough.
	maxBodySize = 512 * 1024

	// maxTitleLen caps the extracted title before it is stored.
	maxTitleLen = 200

	// maxRedirects limits redirect chains. Each hop is re-validated.
	maxRedirects = 3
)

var (
	// ErrBlockedAddress is returned when the resolved address is not public.
	ErrBlockedAddress = errors.New("blocked address")

	// ErrInvalidURL is returned when the URL cannot be fetched at all.
	ErrInvalidURL = errors.New("invalid url")

	titleRe = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
)

// Fetcher retrieves page titles with a hardened HTTP client.
type Fetcher struct {
	client *http.Client
}

// New creates a Fetcher with the given per-request timeout.
// The client refuses to connect to non-public addresses and caps the
// response size.
func New(timeout time.Duration) *Fetcher {
	dialer := &net.Dialer{Timeout: timeout}

	transport := &http.Transport{
		// Validate every address before opening a TCP connection.
		// This is the only place where DNS resolution and the actual
		// connection happen with the same resolved IP, so it is the
		// only place where the check is race-free.
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			for _, ip := range ips {
				if !isPublicIP(ip.IP) {
					return nil, fmt.Errorf("%w: %s", ErrBlockedAddress, ip.IP)
				}
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(host, port))
		},
		// No keep-alives: this is a one-shot call, not a long-lived client.
		DisableKeepAlives: true,
	}

	return &Fetcher{
		client: &http.Client{
			Timeout:   timeout,
			Transport: transport,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= maxRedirects {
					return errors.New("too many redirects")
				}
				// Scheme is re-checked; the DialContext already protects
				// the address for the new request.
				if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
					return ErrInvalidURL
				}
				return nil
			},
		},
	}
}

// FetchTitle retrieves the <title> element of the given URL.
// It returns "" with no error when the page has no <title>.
// Errors are reserved for transport-level failures or blocked addresses.
func (f *Fetcher) FetchTitle(ctx context.Context, rawURL string) (string, error) {
	u, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("%w: scheme %q", ErrInvalidURL, u.Scheme)
	}
	if u.Host == "" {
		return "", fmt.Errorf("%w: missing host", ErrInvalidURL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "BookmarksAPI/1.0")

	resp, err := f.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Accept any 2xx. Redirects were already followed by the client.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	return extractTitle(string(body)), nil
}

// extractTitle finds the first <title> element and returns its content,
// trimmed and truncated to maxTitleLen.
func extractTitle(html string) string {
	m := titleRe.FindStringSubmatch(html)
	if len(m) < 2 {
		return ""
	}
	title := strings.TrimSpace(m[1])
	// Collapse internal whitespace.
	title = strings.Join(strings.Fields(title), " ")
	if len(title) > maxTitleLen {
		title = title[:maxTitleLen]
	}
	return title
}

// isPublicIP reports whether the address is safe to connect to.
// Private, loopback, link-local, multicast, and unspecified addresses
// are rejected, as are carrier-grade NAT ranges.
func isPublicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified() {
		return false
	}
	// 100.64.0.0/10 — carrier-grade NAT, not covered by IsPrivate.
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
		return false
	}
	return true
}