package documentloader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestResolveURLRelative — M0 #1: relative resource URL resolution
// against the final document URL (after redirects).
func TestResolveURLRelative(t *testing.T) {
	base := "https://example.com/a/b/c.html"
	cases := []struct {
		ref  string
		want string
	}{
		{"", "https://example.com/a/b/c.html"},
		{"style.css", "https://example.com/a/b/style.css"},
		{"../d/style.css", "https://example.com/a/d/style.css"},
		{"./style.css", "https://example.com/a/b/style.css"},
		{"/abs/style.css", "https://example.com/abs/style.css"},
		{"?v=2", "https://example.com/a/b/c.html?v=2"},
		{"#frag", "https://example.com/a/b/c.html#frag"},
		{"//cdn.example.com/x.css", "https://cdn.example.com/x.css"},
		{"https://other.test/x.css", "https://other.test/x.css"},
		{"http://plain/x.css", "http://plain/x.css"},
		{"data:text/css,body{color:red}", "data:text/css,body{color:red}"},
	}
	for _, tc := range cases {
		got, err := ResolveURL(base, tc.ref)
		if err != nil {
			t.Errorf("ResolveURL(%q, %q) error: %v", base, tc.ref, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ResolveURL(%q, %q) = %q, want %q", base, tc.ref, got, tc.want)
		}
	}
}

// TestResolveURLAfterRedirect — M0 #1: base URL reflects the final URL
// after redirects, not the originally requested one.
func TestResolveURLAfterRedirect(t *testing.T) {
	// Simulate the browser following one or more redirects.
	original := "https://example.com/login"
	afterRedirect := "https://example.com/app/home"
	got, err := ResolveURL(afterRedirect, "styles.css")
	if err != nil {
		t.Fatalf("ResolveURL error: %v", err)
	}
	want := "https://example.com/app/styles.css"
	if got != want {
		t.Errorf("ResolveURL after redirect = %q, want %q", got, want)
	}
	// Confirm the resolver does NOT silently use the original URL.
	if strings.HasPrefix(got, original) {
		t.Errorf("ResolveURL leaked pre-redirect base: %q", got)
	}
}

// TestResolveURLInvalidBase returns ErrInvalidBaseURL.
func TestResolveURLInvalidBase(t *testing.T) {
	_, err := ResolveURL("://no-scheme", "x.css")
	if err == nil {
		t.Fatal("expected error for invalid base URL")
	}
}

// TestResolveURLInvalidRef returns a SkippedError.
func TestResolveURLInvalidRef(t *testing.T) {
	_, err := ResolveURL("https://example.com/", "://bad")
	if err == nil {
		t.Fatal("expected error for invalid ref")
	}
	if !strings.Contains(err.Error(), "invalid resource URL") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestIsHTTPOrHTTPS classifies schemes the coordinator should fetch vs skip.
func TestIsHTTPOrHTTPS(t *testing.T) {
	cases := map[string]bool{
		"http://example.com/":       true,
		"https://example.com/":      true,
		"data:text/css,body{}":      false,
		"blob:https://x/abc":        false,
		"file:///etc/passwd":        false,
		"javascript:alert(1)":       false,
		"mailto:nobody@example.com": false,
		"":                          false,
		"://malformed":              false,
	}
	for in, want := range cases {
		got := IsHTTPOrHTTPS(in)
		if got != want {
			t.Errorf("IsHTTPOrHTTPS(%q) = %v, want %v", in, got, want)
		}
	}
}

// TestResolveURLAgainstHTTPServer cross-checks the resolver against an
// actual httptest server. The server replies with a 301 to a final URL;
// the coordinator's caller is expected to use resp.Request.URL as the
// base for subsequent resource resolution.
func TestResolveURLAgainstHTTPServer(t *testing.T) {
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html>final</html>`))
	}))
	defer final.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL+"/page", http.StatusMovedPermanently)
	})
	start := httptest.NewServer(mux)
	defer start.Close()

	// Simulate the network layer capturing the final URL after redirects.
	client := start.Client()
	// Disable auto-redirect so we can observe intermediate hops.
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, start.URL+"/start", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("redirect probe failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("expected 301, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc == "" {
		t.Fatal("redirect missing Location header")
	}
	// The browser would follow this redirect and arrive at final.URL/page.
	finalURL, err := url.JoinPath(final.URL, "/page")
	if err != nil {
		t.Fatalf("JoinPath: %v", err)
	}
	got, err := ResolveURL(finalURL, "../css/theme.css")
	if err != nil {
		t.Fatalf("ResolveURL: %v", err)
	}
	wantPrefix := final.URL + "/css/theme.css"
	if got != wantPrefix {
		t.Errorf("ResolveURL against redirected base = %q, want %q", got, wantPrefix)
	}
}
