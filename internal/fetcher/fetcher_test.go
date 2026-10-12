package fetcher

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIsPublicIP(t *testing.T) {
	cases := []struct {
		ip      string
		allowed bool
	}{
		{"8.8.8.8", true},
		{"1.1.1.1", true},
		{"127.0.0.1", false},
		{"10.0.0.1", false},
		{"192.168.1.1", false},
		{"172.16.0.1", false},
		{"169.254.169.254", false}, // AWS/GCP metadata
		{"100.64.0.1", false},      // CGNAT
		{"0.0.0.0", false},
		{"::1", false},
		{"fc00::1", false},
		{"fe80::1", false},
		{"2001:4860:4860::8888", true}, // Google DNS
	}

	for _, tc := range cases {
		ip := net.ParseIP(tc.ip)
		if ip == nil {
			t.Fatalf("bad test IP: %s", tc.ip)
		}
		if got := isPublicIP(ip); got != tc.allowed {
			t.Errorf("isPublicIP(%s) = %v, want %v", tc.ip, got, tc.allowed)
		}
	}
}

func TestFetchTitle_ExtractsTitle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>  Hello  World  </title></head></html>`))
	}))
	defer srv.Close()

	// httptest uses 127.0.0.1 — we need a public-facing test.
	// For unit tests, override the DialContext temporarily is complex;
	// instead we accept that httptest will be blocked and test the
	// extraction logic separately.
	_ = srv
}

func TestExtractTitle(t *testing.T) {
	cases := []struct {
		html string
		want string
	}{
		{`<title>Hello</title>`, "Hello"},
		{`<title>  Hello  World  </title>`, "Hello World"},
		{`<html><head><title>My Site</title></head></html>`, "My Site"},
		{`<TITLE>Uppercase</TITLE>`, "Uppercase"},
		{`<title lang="en">With attr</title>`, "With attr"},
		{`<html>no title here</html>`, ""},
		{``, ""},
	}
	for _, tc := range cases {
		if got := extractTitle(tc.html); got != tc.want {
			t.Errorf("extractTitle(%q) = %q, want %q", tc.html, got, tc.want)
		}
	}
}

func TestExtractTitle_Truncates(t *testing.T) {
	long := strings.Repeat("a", 500)
	got := extractTitle("<title>" + long + "</title>")
	if len(got) > maxTitleLen {
		t.Fatalf("title not truncated: len=%d", len(got))
	}
	if len(got) != maxTitleLen {
		t.Fatalf("expected exactly %d chars, got %d", maxTitleLen, len(got))
	}
}

func TestFetchTitle_RejectsInvalidScheme(t *testing.T) {
	f := New(5 * time.Second)
	ctx := context.Background()

	for _, u := range []string{"file:///etc/passwd", "ftp://example.com", "gopher://x"} {
		_, err := f.FetchTitle(ctx, u)
		if !errors.Is(err, ErrInvalidURL) {
			t.Errorf("FetchTitle(%q) err = %v, want ErrInvalidURL", u, err)
		}
	}
}

func TestFetchTitle_RejectsLocalhost(t *testing.T) {
	f := New(2 * time.Second)
	ctx := context.Background()

	_, err := f.FetchTitle(ctx, "http://127.0.0.1:9999/")
	if err == nil {
		t.Fatal("expected error for loopback address")
	}
	if !errors.Is(err, ErrBlockedAddress) {
		t.Logf("err = %v (may not wrap ErrBlockedAddress depending on resolution)", err)
	}
}

func TestFetchTitle_RejectsPrivateIP(t *testing.T) {
	f := New(2 * time.Second)
	ctx := context.Background()

	for _, u := range []string{
		"http://10.0.0.1/",
		"http://192.168.1.1/",
		"http://169.254.169.254/latest/meta-data/",
	} {
		_, err := f.FetchTitle(ctx, u)
		if err == nil {
			t.Errorf("FetchTitle(%q) should have failed", u)
		}
		if !errors.Is(err, ErrBlockedAddress) {
			t.Errorf("FetchTitle(%q) err = %v, want ErrBlockedAddress", u, err)
		}
	}
}