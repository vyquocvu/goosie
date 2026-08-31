package navigation_test

import (
	"context"
	"net/url"
	"testing"
	"github.com/vyquocvu/goosie/internal/engine/navigation"
)

func TestParseOriginHTTPS(t *testing.T) {
	o, err := navigation.ParseOrigin("https://example.com")
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
	o, err := navigation.ParseOrigin("http://example.com:80")
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
	o, err := navigation.ParseOrigin("https://example.com:443")
	if err != nil {
		t.Fatalf("ParseOrigin: %v", err)
	}
	if o.String() != "https://example.com" {
		t.Errorf("String() = %q, want %q", o.String(), "https://example.com")
	}
}

func TestParseOriginWithCustomPort(t *testing.T) {
	o, err := navigation.ParseOrigin("https://example.com:8443")
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
	o, err := navigation.ParseOrigin("https://example.com/path/to/page?q=hello#frag")
	if err != nil {
		t.Fatalf("ParseOrigin: %v", err)
	}
	if o.String() != "https://example.com" {
		t.Errorf("String() = %q, want %q", o.String(), "https://example.com")
	}
}

func TestParseOriginInvalidURL(t *testing.T) {
	_, err := navigation.ParseOrigin("://invalid")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestParseOriginEmptyString(t *testing.T) {
	o, err := navigation.ParseOrigin("")
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
	o, err := navigation.ParseOrigin("data:text/html,<h1>hello</h1>")
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
	o, err := navigation.ParseOrigin("javascript:void(0)")
	if err != nil {
		t.Fatalf("ParseOrigin: %v", err)
	}
	if !o.IsOpaque() {
		t.Fatal("javascript: URL should produce opaque origin")
	}
}

func TestOriginIsOpaqueAboutBlank(t *testing.T) {
	o, err := navigation.ParseOrigin("about:blank")
	if err != nil {
		t.Fatalf("ParseOrigin: %v", err)
	}
	if !o.IsOpaque() {
		t.Fatal("about:blank should produce opaque origin")
	}
}

func TestParseOriginFileURL(t *testing.T) {
	o, err := navigation.ParseOrigin("file:///path/to/file.html")
	if err != nil {
		t.Fatalf("ParseOrigin: %v", err)
	}
	if !o.IsOpaque() {
		t.Fatal("file: URL should produce opaque origin per current implementation")
	}
}

func TestOriginIsOpaqueConsistentWithInvalid(t *testing.T) {
	o, _ := navigation.ParseOrigin("data:text/html,hello")
	if o.IsValid() == o.IsOpaque() {
		t.Fatal("IsValid and IsOpaque should be opposites")
	}
}

func TestOriginZeroValue(t *testing.T) {
	var o navigation.Origin
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
	o := navigation.OriginFromURL(u)
	if o.String() != "https://example.com:8443" {
		t.Errorf("String() = %q, want %q", o.String(), "https://example.com:8443")
	}
}

func TestOriginFromURLNil(t *testing.T) {
	o := navigation.OriginFromURL(nil)
	if o.IsValid() {
		t.Fatal("nil URL should produce invalid origin")
	}
}

func TestOriginFromURLNoHost(t *testing.T) {
	u, _ := url.Parse("data:text/html,hello")
	o := navigation.OriginFromURL(u)
	if o.IsValid() {
		t.Fatal("URL with no host should produce invalid origin")
	}
}

func TestIsSameOriginIdentical(t *testing.T) {
	a, _ := navigation.ParseOrigin("https://example.com")
	b, _ := navigation.ParseOrigin("https://example.com")
	if !a.IsSameOrigin(b) {
		t.Fatal("identical origins should be same")
	}
}

func TestIsSameOriginDifferentScheme(t *testing.T) {
	a, _ := navigation.ParseOrigin("https://example.com")
	b, _ := navigation.ParseOrigin("http://example.com")
	if a.IsSameOrigin(b) {
		t.Fatal("different schemes should not be same origin")
	}
}

func TestIsSameOriginDifferentHost(t *testing.T) {
	a, _ := navigation.ParseOrigin("https://example.com")
	b, _ := navigation.ParseOrigin("https://other.com")
	if a.IsSameOrigin(b) {
		t.Fatal("different hosts should not be same origin")
	}
}

func TestIsSameOriginDefaultPortEquivalent(t *testing.T) {
	a, _ := navigation.ParseOrigin("https://example.com")
	b, _ := navigation.ParseOrigin("https://example.com:443")
	if !a.IsSameOrigin(b) {
		t.Fatal("https://example.com and https://example.com:443 should be same origin")
	}
}

func TestIsSameOriginDifferentPort(t *testing.T) {
	a, _ := navigation.ParseOrigin("https://example.com:8080")
	b, _ := navigation.ParseOrigin("https://example.com:9090")
	if a.IsSameOrigin(b) {
		t.Fatal("different ports should not be same origin")
	}
}

func TestIsSameOriginOpaqueNeverSame(t *testing.T) {
	a, _ := navigation.ParseOrigin("data:text/html,hello")
	b, _ := navigation.ParseOrigin("data:text/html,world")
	if a.IsSameOrigin(b) {
		t.Fatal("two opaque origins should not be same origin")
	}
}

func TestIsSameOriginOpaqueAndValid(t *testing.T) {
	a, _ := navigation.ParseOrigin("data:text/html,hello")
	b, _ := navigation.ParseOrigin("https://example.com")
	if a.IsSameOrigin(b) {
		t.Fatal("opaque and valid origin should not be same")
	}
}

func TestIsSameOriginCaseSensitiveHost(t *testing.T) {
	a, _ := navigation.ParseOrigin("https://Example.COM")
	b, _ := navigation.ParseOrigin("https://example.com")
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
		o, err := navigation.ParseOrigin(raw)
		if err != nil {
			t.Fatalf("ParseOrigin(%q): %v", raw, err)
		}
		got := o.String()
		if got == "" {
			t.Errorf("String() for %q returned empty", raw)
			continue
		}
		// Re-parse the serialized form
		o2, err := navigation.ParseOrigin(got)
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
	o, err := navigation.ParseOrigin("https://[::1]:8080/path")
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
	o, err := navigation.ParseOrigin("https://[::1]:443")
	if err != nil {
		t.Fatalf("ParseOrigin: %v", err)
	}
	if o.String() != "https://[::1]" {
		t.Errorf("String() = %q, want %q", o.String(), "https://[::1]")
	}
}

func TestOriginHostMethod(t *testing.T) {
	o, err := navigation.ParseOrigin("https://example.com:8080")
	if err != nil {
		t.Fatalf("ParseOrigin: %v", err)
	}
	if o.Host() != "example.com" {
		t.Errorf("Host() = %q, want %q", o.Host(), "example.com")
	}
}

func TestOriginSchemeMethod(t *testing.T) {
	o, err := navigation.ParseOrigin("http://example.com")
	if err != nil {
		t.Fatalf("ParseOrigin: %v", err)
	}
	if o.Scheme() != "http" {
		t.Errorf("Scheme() = %q, want %q", o.Scheme(), "http")
	}
}

func TestOriginPortMethod(t *testing.T) {
	o, err := navigation.ParseOrigin("https://example.com:8443")
	if err != nil {
		t.Fatalf("ParseOrigin: %v", err)
	}
	if o.Port() != "8443" {
		t.Errorf("Port() = %q, want %q", o.Port(), "8443")
	}
}

func TestOriginPortMethodEmpty(t *testing.T) {
	o, err := navigation.ParseOrigin("https://example.com")
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
		o, err := navigation.ParseOrigin(tt.rawURL)
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
			navigation.ParseOrigin(u)
		}
	}
}

func BenchmarkOriginString(b *testing.B) {
	o, _ := navigation.ParseOrigin("https://example.com:8443")
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = o.String()
	}
}

func BenchmarkIsSameOrigin(b *testing.B) {
	a, _ := navigation.ParseOrigin("https://example.com:8443")
	c, _ := navigation.ParseOrigin("https://example.com:8443")
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
		_ = navigation.OriginFromURL(u)
	}
}

func TestRegistrableDomainStandard(t *testing.T) {
	o, _ := navigation.ParseOrigin("https://example.com")
	dom, ok := o.RegistrableDomain()
	if !ok {
		t.Fatal("RegistrableDomain() returned false for example.com")
	}
	if dom != "example.com" {
		t.Errorf("RegistrableDomain() = %q, want %q", dom, "example.com")
	}
}

func TestRegistrableDomainSubdomain(t *testing.T) {
	o, _ := navigation.ParseOrigin("https://sub.example.com")
	dom, ok := o.RegistrableDomain()
	if !ok {
		t.Fatal("RegistrableDomain() returned false for sub.example.com")
	}
	if dom != "example.com" {
		t.Errorf("RegistrableDomain() = %q, want %q", dom, "example.com")
	}
}

func TestRegistrableDomainDeepSubdomain(t *testing.T) {
	o, _ := navigation.ParseOrigin("https://a.b.c.example.com")
	dom, ok := o.RegistrableDomain()
	if !ok {
		t.Fatal("RegistrableDomain() returned false for a.b.c.example.com")
	}
	if dom != "example.com" {
		t.Errorf("RegistrableDomain() = %q, want %q", dom, "example.com")
	}
}

func TestRegistrableDomainUK(t *testing.T) {
	o, _ := navigation.ParseOrigin("https://sub.example.co.uk")
	dom, ok := o.RegistrableDomain()
	if !ok {
		t.Fatal("RegistrableDomain() returned false for sub.example.co.uk")
	}
	if dom != "example.co.uk" {
		t.Errorf("RegistrableDomain() = %q, want %q", dom, "example.co.uk")
	}
}

func TestRegistrableDomainIP(t *testing.T) {
	o, _ := navigation.ParseOrigin("https://127.0.0.1")
	_, ok := o.RegistrableDomain()
	if ok {
		t.Fatal("RegistrableDomain() should return false for IP address")
	}
}

func TestRegistrableDomainIPv6(t *testing.T) {
	o, _ := navigation.ParseOrigin("https://[::1]:8080")
	_, ok := o.RegistrableDomain()
	if ok {
		t.Fatal("RegistrableDomain() should return false for IPv6 address")
	}
}

func TestRegistrableDomainOpaque(t *testing.T) {
	o, _ := navigation.ParseOrigin("data:text/html,hello")
	_, ok := o.RegistrableDomain()
	if ok {
		t.Fatal("RegistrableDomain() should return false for opaque origin")
	}
}

func TestRegistrableDomainZeroValue(t *testing.T) {
	var o navigation.Origin
	_, ok := o.RegistrableDomain()
	if ok {
		t.Fatal("RegistrableDomain() should return false for zero value")
	}
}

func TestIsSameSiteIdentical(t *testing.T) {
	a, _ := navigation.ParseOrigin("https://example.com")
	b, _ := navigation.ParseOrigin("https://example.com")
	if !a.IsSameSite(b) {
		t.Fatal("identical origins should be same-site")
	}
}

func TestIsSameSiteDifferentSchemes(t *testing.T) {
	a, _ := navigation.ParseOrigin("https://example.com")
	b, _ := navigation.ParseOrigin("http://example.com")
	if !a.IsSameSite(b) {
		t.Fatal("different schemes but same registrable domain = same-site")
	}
}

func TestIsSameSiteDifferentPorts(t *testing.T) {
	a, _ := navigation.ParseOrigin("https://example.com:8080")
	b, _ := navigation.ParseOrigin("https://example.com:9090")
	if !a.IsSameSite(b) {
		t.Fatal("different ports but same registrable domain = same-site")
	}
}

func TestIsSameSiteSubdomain(t *testing.T) {
	a, _ := navigation.ParseOrigin("https://example.com")
	b, _ := navigation.ParseOrigin("https://sub.example.com")
	if !a.IsSameSite(b) {
		t.Fatal("subdomain should be same-site with parent")
	}
}

func TestIsSameSiteDeepSubdomain(t *testing.T) {
	a, _ := navigation.ParseOrigin("https://a.example.com")
	b, _ := navigation.ParseOrigin("https://deep.b.example.com")
	if !a.IsSameSite(b) {
		t.Fatal("two subdomains of same site should be same-site")
	}
}

func TestIsSameSiteDifferentTLD(t *testing.T) {
	a, _ := navigation.ParseOrigin("https://example.com")
	b, _ := navigation.ParseOrigin("https://example.org")
	if a.IsSameSite(b) {
		t.Fatal("different TLDs should not be same-site")
	}
}

func TestIsSameSiteOpaque(t *testing.T) {
	a, _ := navigation.ParseOrigin("data:text/html,hello")
	b, _ := navigation.ParseOrigin("https://example.com")
	if a.IsSameSite(b) {
		t.Fatal("opaque and valid origin should not be same-site")
	}
}

func TestIsSameSiteOpaqueBoth(t *testing.T) {
	a, _ := navigation.ParseOrigin("data:text/html,hello")
	b, _ := navigation.ParseOrigin("javascript:void(0)")
	if a.IsSameSite(b) {
		t.Fatal("two opaque origins should not be same-site")
	}
}

func TestIsSameSiteIP(t *testing.T) {
	a, _ := navigation.ParseOrigin("https://127.0.0.1")
	b, _ := navigation.ParseOrigin("https://127.0.0.1")
	if a.IsSameSite(b) {
		t.Fatal("IP addresses should not be same-site (no registrable domain)")
	}
}

func TestIsSameSiteDifferentSubdomainsUK(t *testing.T) {
	a, _ := navigation.ParseOrigin("https://sub1.example.co.uk")
	b, _ := navigation.ParseOrigin("https://sub2.example.co.uk")
	if !a.IsSameSite(b) {
		t.Fatal("two subdomains under same .co.uk should be same-site")
	}
}

func BenchmarkRegistrableDomain(b *testing.B) {
	o, _ := navigation.ParseOrigin("https://sub.example.com")
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		o.RegistrableDomain()
	}
}

func BenchmarkIsSameSite(b *testing.B) {
	a, _ := navigation.ParseOrigin("https://example.com")
	c, _ := navigation.ParseOrigin("https://sub.example.com")
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		a.IsSameSite(c)
	}
}

func BenchmarkRegistrableDomainIP(b *testing.B) {
	o, _ := navigation.ParseOrigin("https://127.0.0.1")
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		o.RegistrableDomain()
	}
}

func BenchmarkRegistrableDomainOpaque(b *testing.B) {
	o, _ := navigation.ParseOrigin("data:text/html,hello")
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		o.RegistrableDomain()
	}
}

func TestRateLimiterAcceptsOriginHost(t *testing.T) {
	rl := navigation.NewRateLimiter(6, 24)
	ctx := context.Background()

	o, _ := navigation.ParseOrigin("https://example.com:8443")
	if err := rl.Acquire(ctx, o.Host(), navigation.PriorityDocument); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	rl.Release(o.Host())

	rl.Mu.Lock()
	if rl.GlobalActive != 0 {
		t.Errorf("GlobalActive = %d, want 0", rl.GlobalActive)
	}
	rl.Mu.Unlock()
}
