package navigation

import (
	"context"
	"net/url"
	"testing"
)

func TestParseOriginHTTPS(t *testing.T) {
	o, err := ParseOrigin("https://example.com")
	if err != nil {
		t.Fatalf("ParseOrigin: %v", err)
	}
	if !o.IsValid() {
		t.Fatal("origin should be valid")
	}
	if o.IsOpaque() {
		t.Fatal("origin should not be opaque")
	}
	if o.Scheme() != "https" {
		t.Errorf("Scheme() = %q, want %q", o.Scheme(), "https")
	}
	if o.Host() != "example.com" {
		t.Errorf("Host() = %q, want %q", o.Host(), "example.com")
	}
	if o.Port() != "" {
		t.Errorf("Port() = %q, want %q", o.Port(), "")
	}
	if o.String() != "https://example.com" {
		t.Errorf("String() = %q, want %q", o.String(), "https://example.com")
	}
}

func TestParseOriginHTTPWithDefaultPort(t *testing.T) {
	o, err := ParseOrigin("http://example.com:80")
	if err != nil {
		t.Fatalf("ParseOrigin: %v", err)
	}
	// Default port should be omitted in String()
	if o.String() != "http://example.com" {
		t.Errorf("String() = %q, want %q", o.String(), "http://example.com")
	}
	if o.Port() != "80" {
		t.Errorf("Port() = %q, want %q", o.Port(), "80")
	}
}

func TestParseOriginHTTPSWithDefaultPort(t *testing.T) {
	o, err := ParseOrigin("https://example.com:443")
	if err != nil {
		t.Fatalf("ParseOrigin: %v", err)
	}
	if o.String() != "https://example.com" {
		t.Errorf("String() = %q, want %q", o.String(), "https://example.com")
	}
}

func TestParseOriginWithCustomPort(t *testing.T) {
	o, err := ParseOrigin("https://example.com:8443")
	if err != nil {
		t.Fatalf("ParseOrigin: %v", err)
	}
	if o.Port() != "8443" {
		t.Errorf("Port() = %q, want %q", o.Port(), "8443")
	}
	if o.String() != "https://example.com:8443" {
		t.Errorf("String() = %q, want %q", o.String(), "https://example.com:8443")
	}
}

func TestParseOriginWithPathAndQuery(t *testing.T) {
	o, err := ParseOrigin("https://example.com/path/to/page?q=hello#frag")
	if err != nil {
		t.Fatalf("ParseOrigin: %v", err)
	}
	if o.String() != "https://example.com" {
		t.Errorf("String() = %q, want %q", o.String(), "https://example.com")
	}
}

func TestParseOriginInvalidURL(t *testing.T) {
	_, err := ParseOrigin("://invalid")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestParseOriginEmptyString(t *testing.T) {
	o, err := ParseOrigin("")
	if err != nil {
		t.Fatalf("ParseOrigin: %v", err)
	}
	if o.IsValid() {
		t.Fatal("empty string should produce opaque origin")
	}
	if !o.IsOpaque() {
		t.Fatal("empty string should be opaque")
	}
}

func TestOriginIsOpaqueDataURL(t *testing.T) {
	o, err := ParseOrigin("data:text/html,<h1>hello</h1>")
	if err != nil {
		t.Fatalf("ParseOrigin: %v", err)
	}
	if !o.IsOpaque() {
		t.Fatal("data: URL should produce opaque origin")
	}
	if o.IsValid() {
		t.Fatal("opaque origin should not be valid")
	}
}

func TestOriginIsOpaqueJavascriptURL(t *testing.T) {
	o, err := ParseOrigin("javascript:void(0)")
	if err != nil {
		t.Fatalf("ParseOrigin: %v", err)
	}
	if !o.IsOpaque() {
		t.Fatal("javascript: URL should produce opaque origin")
	}
}

func TestOriginIsOpaqueAboutBlank(t *testing.T) {
	o, err := ParseOrigin("about:blank")
	if err != nil {
		t.Fatalf("ParseOrigin: %v", err)
	}
	if !o.IsOpaque() {
		t.Fatal("about:blank should produce opaque origin")
	}
}

func TestParseOriginFileURL(t *testing.T) {
	o, err := ParseOrigin("file:///path/to/file.html")
	if err != nil {
		t.Fatalf("ParseOrigin: %v", err)
	}
	if !o.IsOpaque() {
		t.Fatal("file: URL should produce opaque origin per current implementation")
	}
}

func TestOriginIsOpaqueConsistentWithInvalid(t *testing.T) {
	o, _ := ParseOrigin("data:text/html,hello")
	if o.IsValid() == o.IsOpaque() {
		t.Fatal("IsValid and IsOpaque should be opposites")
	}
}

func TestOriginZeroValue(t *testing.T) {
	var o Origin
	if o.IsValid() {
		t.Fatal("zero value origin should not be valid")
	}
	if !o.IsOpaque() {
		t.Fatal("zero value origin should be opaque")
	}
	if o.String() != "" {
		t.Errorf("String() = %q, want empty", o.String())
	}
}

func TestOriginFromURL(t *testing.T) {
	u, _ := url.Parse("https://example.com:8443/path")
	o := OriginFromURL(u)
	if o.String() != "https://example.com:8443" {
		t.Errorf("String() = %q, want %q", o.String(), "https://example.com:8443")
	}
}

func TestOriginFromURLNil(t *testing.T) {
	o := OriginFromURL(nil)
	if o.IsValid() {
		t.Fatal("nil URL should produce invalid origin")
	}
}

func TestOriginFromURLNoHost(t *testing.T) {
	u, _ := url.Parse("data:text/html,hello")
	o := OriginFromURL(u)
	if o.IsValid() {
		t.Fatal("URL with no host should produce invalid origin")
	}
}

func TestIsSameOriginIdentical(t *testing.T) {
	a, _ := ParseOrigin("https://example.com")
	b, _ := ParseOrigin("https://example.com")
	if !a.IsSameOrigin(b) {
		t.Fatal("identical origins should be same")
	}
}

func TestIsSameOriginDifferentScheme(t *testing.T) {
	a, _ := ParseOrigin("https://example.com")
	b, _ := ParseOrigin("http://example.com")
	if a.IsSameOrigin(b) {
		t.Fatal("different schemes should not be same origin")
	}
}

func TestIsSameOriginDifferentHost(t *testing.T) {
	a, _ := ParseOrigin("https://example.com")
	b, _ := ParseOrigin("https://other.com")
	if a.IsSameOrigin(b) {
		t.Fatal("different hosts should not be same origin")
	}
}

func TestIsSameOriginDefaultPortEquivalent(t *testing.T) {
	a, _ := ParseOrigin("https://example.com")
	b, _ := ParseOrigin("https://example.com:443")
	if !a.IsSameOrigin(b) {
		t.Fatal("https://example.com and https://example.com:443 should be same origin")
	}
}

func TestIsSameOriginDifferentPort(t *testing.T) {
	a, _ := ParseOrigin("https://example.com:8080")
	b, _ := ParseOrigin("https://example.com:9090")
	if a.IsSameOrigin(b) {
		t.Fatal("different ports should not be same origin")
	}
}

func TestIsSameOriginOpaqueNeverSame(t *testing.T) {
	a, _ := ParseOrigin("data:text/html,hello")
	b, _ := ParseOrigin("data:text/html,world")
	if a.IsSameOrigin(b) {
		t.Fatal("two opaque origins should not be same origin")
	}
}

func TestIsSameOriginOpaqueAndValid(t *testing.T) {
	a, _ := ParseOrigin("data:text/html,hello")
	b, _ := ParseOrigin("https://example.com")
	if a.IsSameOrigin(b) {
		t.Fatal("opaque and valid origin should not be same")
	}
}

func TestIsSameOriginCaseSensitiveHost(t *testing.T) {
	a, _ := ParseOrigin("https://Example.COM")
	b, _ := ParseOrigin("https://example.com")
	if a.Host() != b.Host() {
		t.Fatal("host comparison is case-insensitive per URL spec; url.Parse lowercases host")
	}
	if !a.IsSameOrigin(b) {
		t.Fatal("same origin after url.Parse normalization")
	}
}

func TestOriginStringRoundTrip(t *testing.T) {
	urls := []string{
		"https://example.com",
		"http://example.com:8080",
		"https://example.com:8443",
		"http://localhost:3000",
		"https://127.0.0.1",
		"https://[::1]:8080",
	}
	for _, raw := range urls {
		o, err := ParseOrigin(raw)
		if err != nil {
			t.Fatalf("ParseOrigin(%q): %v", raw, err)
		}
		got := o.String()
		if got == "" {
			t.Errorf("String() for %q returned empty", raw)
			continue
		}
		// Re-parse the serialized form
		o2, err := ParseOrigin(got)
		if err != nil {
			t.Errorf("re-parse of %q failed: %v", got, err)
			continue
		}
		if !o.IsSameOrigin(o2) {
			t.Errorf("round-trip: %q -> %q -> not same origin", raw, got)
		}
	}
}

func TestOriginIPv6(t *testing.T) {
	o, err := ParseOrigin("https://[::1]:8080/path")
	if err != nil {
		t.Fatalf("ParseOrigin: %v", err)
	}
	if o.Host() != "::1" {
		t.Errorf("Host() = %q, want %q", o.Host(), "::1")
	}
	if o.Port() != "8080" {
		t.Errorf("Port() = %q, want %q", o.Port(), "8080")
	}
	if o.String() != "https://[::1]:8080" {
		t.Errorf("String() = %q, want %q", o.String(), "https://[::1]:8080")
	}
}

func TestOriginIPv6DefaultPort(t *testing.T) {
	o, err := ParseOrigin("https://[::1]:443")
	if err != nil {
		t.Fatalf("ParseOrigin: %v", err)
	}
	if o.String() != "https://[::1]" {
		t.Errorf("String() = %q, want %q", o.String(), "https://[::1]")
	}
}

func TestOriginHostMethod(t *testing.T) {
	o, err := ParseOrigin("https://example.com:8080")
	if err != nil {
		t.Fatalf("ParseOrigin: %v", err)
	}
	if o.Host() != "example.com" {
		t.Errorf("Host() = %q, want %q", o.Host(), "example.com")
	}
}

func TestOriginSchemeMethod(t *testing.T) {
	o, err := ParseOrigin("http://example.com")
	if err != nil {
		t.Fatalf("ParseOrigin: %v", err)
	}
	if o.Scheme() != "http" {
		t.Errorf("Scheme() = %q, want %q", o.Scheme(), "http")
	}
}

func TestOriginPortMethod(t *testing.T) {
	o, err := ParseOrigin("https://example.com:8443")
	if err != nil {
		t.Fatalf("ParseOrigin: %v", err)
	}
	if o.Port() != "8443" {
		t.Errorf("Port() = %q, want %q", o.Port(), "8443")
	}
}

func TestOriginPortMethodEmpty(t *testing.T) {
	o, err := ParseOrigin("https://example.com")
	if err != nil {
		t.Fatalf("ParseOrigin: %v", err)
	}
	if o.Port() != "" {
		t.Errorf("Port() = %q, want empty", o.Port())
	}
}

// TestExtractOriginConsistency verifies that ParseOrigin().Host() returns
// the same value the old extractOrigin returned for known inputs.
func TestExtractOriginConsistency(t *testing.T) {
	tests := []struct {
		rawURL string
		want   string
	}{
		{"https://example.com", "example.com"},
		{"https://example.com:8443", "example.com"},
		{"http://example.com:80/path?q=1", "example.com"},
		{"https://[::1]:8080", "::1"},
		{"http://localhost:3000", "localhost"},
	}
	for _, tt := range tests {
		o, err := ParseOrigin(tt.rawURL)
		if err != nil {
			t.Errorf("ParseOrigin(%q): %v", tt.rawURL, err)
			continue
		}
		if o.Host() != tt.want {
			t.Errorf("ParseOrigin(%q).Host() = %q, want %q", tt.rawURL, o.Host(), tt.want)
		}
	}
}

func BenchmarkParseOrigin(b *testing.B) {
	urls := []string{
		"https://example.com",
		"https://example.com:8443/path?q=1",
		"http://localhost:3000",
		"https://[::1]:8080",
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		for _, u := range urls {
			ParseOrigin(u)
		}
	}
}

func BenchmarkOriginString(b *testing.B) {
	o, _ := ParseOrigin("https://example.com:8443")
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = o.String()
	}
}

func BenchmarkIsSameOrigin(b *testing.B) {
	a, _ := ParseOrigin("https://example.com:8443")
	c, _ := ParseOrigin("https://example.com:8443")
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = a.IsSameOrigin(c)
	}
}

func BenchmarkOriginFromURL(b *testing.B) {
	u, _ := url.Parse("https://example.com:8443/path")
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = OriginFromURL(u)
	}
}

func TestRateLimiterAcceptsOriginHost(t *testing.T) {
	rl := NewRateLimiter(6, 24)
	ctx := context.Background()

	o, _ := ParseOrigin("https://example.com:8443")
	if err := rl.Acquire(ctx, o.Host(), PriorityDocument); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	rl.Release(o.Host())

	rl.mu.Lock()
	if rl.globalActive != 0 {
		t.Errorf("globalActive = %d, want 0", rl.globalActive)
	}
	rl.mu.Unlock()
}
