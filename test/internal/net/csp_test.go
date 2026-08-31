package net_test

import (
	"errors"
	"net/url"
	"testing"
	"github.com/vyquocvu/goosie/internal/net"
)

func mustParseURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return u
}

func TestParseCSPHeader(t *testing.T) {
	tests := []struct {
		name     string
		headers  []string
		wantNil  bool
		wantDirs map[string][]string
	}{
		{
			name:    "nil headers",
			headers: nil,
			wantNil: true,
		},
		{
			name:    "empty headers",
			headers: []string{"", "  "},
			wantNil: true,
		},
		{
			name:    "single directive",
			headers: []string{"default-src 'self'"},
			wantDirs: map[string][]string{
				"default-src": {"'self'"},
			},
		},
		{
			name:    "multiple directives",
			headers: []string{"default-src 'self'; script-src 'self' https://cdn.example.com; style-src 'self'"},
			wantDirs: map[string][]string{
				"default-src": {"'self'"},
				"script-src":  {"'self'", "https://cdn.example.com"},
				"style-src":   {"'self'"},
			},
		},
		{
			name:    "multiple CSP headers merged",
			headers: []string{"script-src 'self'", "script-src https://cdn.example.com"},
			wantDirs: map[string][]string{
				"script-src": {"'self'", "https://cdn.example.com"},
			},
		},
		{
			name:    "directive without sources",
			headers: []string{"default-src; script-src 'self'"},
			wantDirs: map[string][]string{
				"script-src": {"'self'"},
			},
		},
		{
			name:    "case insensitive directive name",
			headers: []string{"Script-Src 'self'"},
			wantDirs: map[string][]string{
				"script-src": {"'self'"},
			},
		},
		{
			name:    "extra whitespace",
			headers: []string{"  default-src   'self'  ;  script-src   'self'  "},
			wantDirs: map[string][]string{
				"default-src": {"'self'"},
				"script-src":  {"'self'"},
			},
		},
		{
			name:    "unknown directives preserved",
			headers: []string{"img-src 'self'; script-src 'self'"},
			wantDirs: map[string][]string{
				"img-src":    {"'self'"},
				"script-src": {"'self'"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := net.ParseCSPHeader(tt.headers...)
			if tt.wantNil {
				if p != nil {
					t.Fatalf("ParseCSPHeader returned non-nil, want nil")
				}
				return
			}
			if p == nil {
				t.Fatalf("ParseCSPHeader returned nil, want policy")
			}
			for name, wantSrcs := range tt.wantDirs {
				gotSrcs := p.Sources(name)
				if len(gotSrcs) != len(wantSrcs) {
					t.Errorf("directive %q: got %v, want %v", name, gotSrcs, wantSrcs)
					continue
				}
				for i, got := range gotSrcs {
					if got != wantSrcs[i] {
						t.Errorf("directive %q[%d]: got %q, want %q", name, i, got, wantSrcs[i])
					}
				}
			}
		})
	}
}

func TestCSPPolicy_HasDirective(t *testing.T) {
	p := net.ParseCSPHeader("script-src 'self'; style-src 'self'")
	if !p.HasDirective("script-src") {
		t.Error("HasDirective(script-src) = false, want true")
	}
	if !p.HasDirective("style-src") {
		t.Error("HasDirective(style-src) = false, want true")
	}
	if p.HasDirective("img-src") {
		t.Error("HasDirective(img-src) = true, want false")
	}
	var nilPolicy *net.CSPPolicy
	if nilPolicy.HasDirective("script-src") {
		t.Error("nil policy HasDirective should return false")
	}
}

func TestCSPPolicy_AllowsURL(t *testing.T) {
	base := mustParseURL("https://example.com/page")
	tests := []struct {
		name      string
		directive string
		sources   []string
		rawURL    string
		want      bool
	}{
		// 'self' matching
		{"self same origin", "script-src", []string{"'self'"}, "https://example.com/app.js", true},
		{"self different origin", "script-src", []string{"'self'"}, "https://other.com/app.js", false},

		// 'none' blocks everything
		{"none blocks all", "script-src", []string{"'none'"}, "https://example.com/app.js", false},

		// Scheme sources
		{"https scheme matches https url", "script-src", []string{"https:"}, "https://example.com/app.js", true},
		{"https scheme blocks http url", "script-src", []string{"https:"}, "http://example.com/app.js", false},
		{"http scheme matches http url", "connect-src", []string{"http:"}, "http://example.com/api", true},

		// Bare host sources
		{"exact host match", "connect-src", []string{"api.example.com"}, "https://api.example.com/data", true},
		{"exact host mismatch", "connect-src", []string{"api.example.com"}, "https://other.com/data", false},
		{"host with port", "connect-src", []string{"localhost:8080"}, "http://localhost:8080/api", true},

		// Wildcard host
		{"wildcard subdomain match", "script-src", []string{"*.example.com"}, "https://cdn.example.com/app.js", true},
		{"wildcard base domain match", "script-src", []string{"*.example.com"}, "https://example.com/app.js", true},
		{"wildcard different domain", "script-src", []string{"*.example.com"}, "https://cdn.other.com/app.js", false},

		// Full URL source
		{"full url exact match", "script-src", []string{"https://cdn.example.com/app.js"}, "https://cdn.example.com/app.js", true},
		{"full url mismatch", "script-src", []string{"https://cdn.example.com/app.js"}, "https://cdn.example.com/other.js", false},

		// Host with path
		{"host path prefix match", "script-src", []string{"example.com/assets"}, "https://example.com/assets/app.js", true},
		{"host path no match", "script-src", []string{"example.com/assets"}, "https://example.com/other/app.js", false},
		{"host empty path matches all", "script-src", []string{"example.com/"}, "https://example.com/anything", true},

		// Fallback chain
		{"falls back to default-src", "script-src", nil, "https://example.com/app.js", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &net.CSPPolicy{}
			if len(tt.sources) > 0 {
				p.SetDirectives(map[string][]string{tt.directive: tt.sources})
			}
			if tt.rawURL == "" {
				// Inline script: empty URL
			}
			got := p.AllowsURL(tt.directive, tt.rawURL, base)
			if got != tt.want {
				t.Errorf("AllowsURL(%q, %q) = %v, want %v", tt.directive, tt.rawURL, got, tt.want)
			}
		})
	}
}

func TestCSPPolicy_AllowsURL_NilPolicy(t *testing.T) {
	var p *net.CSPPolicy
	base := mustParseURL("https://example.com")
	if !p.AllowsURL("script-src", "https://example.com/app.js", base) {
		t.Error("nil policy should allow all URLs")
	}
}

func TestCSPPolicy_AllowsURL_NoEffectiveSources(t *testing.T) {
	// Policy with only img-src: script-src falls back to default-src which
	// is absent → no restriction.
	p := net.ParseCSPHeader("img-src 'self'")
	base := mustParseURL("https://example.com")
	if !p.AllowsURL("script-src", "https://anything.com/app.js", base) {
		t.Error("should allow when no effective sources (no default-src)")
	}
}

func TestCSPPolicy_AllowsURL_InvalidURL(t *testing.T) {
	p := net.ParseCSPHeader("script-src 'self'")
	base := mustParseURL("https://example.com")
	if p.AllowsURL("script-src", "://invalid", base) {
		t.Error("invalid URL should not be allowed")
	}
}

func TestAllowScript(t *testing.T) {
	base := mustParseURL("https://example.com/page")

	tests := []struct {
		name    string
		policy  *net.CSPPolicy
		rawURL  string
		wantErr error
	}{
		{
			name:    "nil policy allows all",
			policy:  nil,
			rawURL:  "https://example.com/app.js",
			wantErr: nil,
		},
		{
			name:    "self allows same origin",
			policy:  net.ParseCSPHeader("script-src 'self'"),
			rawURL:  "https://example.com/app.js",
			wantErr: nil,
		},
		{
			name:    "self blocks cross origin",
			policy:  net.ParseCSPHeader("script-src 'self'"),
			rawURL:  "https://other.com/app.js",
			wantErr: net.ErrCSPViolation,
		},
		{
			name:    "explicit host allowed",
			policy:  net.ParseCSPHeader("script-src *.example.com"),
			rawURL:  "https://cdn.example.com/lib.js",
			wantErr: nil,
		},
		{
			name:    "explicit host blocked",
			policy:  net.ParseCSPHeader("script-src *.example.com"),
			rawURL:  "https://evil.com/lib.js",
			wantErr: net.ErrCSPViolation,
		},
		{
			name:    "none blocks all",
			policy:  net.ParseCSPHeader("script-src 'none'"),
			rawURL:  "https://example.com/app.js",
			wantErr: net.ErrCSPViolation,
		},
		{
			name:    "wildcard allows subdomain",
			policy:  net.ParseCSPHeader("script-src *.example.com"),
			rawURL:  "https://cdn.example.com/lib.js",
			wantErr: nil,
		},
		{
			name:    "https: allows any https",
			policy:  net.ParseCSPHeader("script-src https:"),
			rawURL:  "https://anywhere.com/app.js",
			wantErr: nil,
		},
		{
			name:    "https: blocks http",
			policy:  net.ParseCSPHeader("script-src https:"),
			rawURL:  "http://example.com/app.js",
			wantErr: net.ErrCSPViolation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.policy.AllowScript(tt.rawURL, base)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("AllowScript(%q) = %v, want %v", tt.rawURL, err, tt.wantErr)
			}
		})
	}
}

func TestAllowStyle(t *testing.T) {
	base := mustParseURL("https://example.com/page")

	p := net.ParseCSPHeader("style-src 'self' *.fonts.example.com")
	if err := p.AllowStyle("https://example.com/style.css", base); err != nil {
		t.Errorf("self style should be allowed: %v", err)
	}
	if err := p.AllowStyle("https://cdn.fonts.example.com/fonts.css", base); err != nil {
		t.Errorf("explicit host style should be allowed: %v", err)
	}
	if err := p.AllowStyle("https://evil.com/style.css", base); err == nil {
		t.Error("cross-origin style should be blocked")
	}
}

func TestAllowConnect(t *testing.T) {
	base := mustParseURL("https://example.com/page")

	p := net.ParseCSPHeader("connect-src api.example.com")
	if err := p.AllowConnect("https://api.example.com/data", base); err != nil {
		t.Errorf("allowed connect should succeed: %v", err)
	}
	if err := p.AllowConnect("https://other.com/data", base); err == nil {
		t.Error("blocked connect should fail")
	}
}

func TestAllowBaseURI(t *testing.T) {
	base := mustParseURL("https://example.com/page")

	p := net.ParseCSPHeader("base-uri 'self'")
	if err := p.AllowBaseURI("https://example.com/", base); err != nil {
		t.Errorf("self base-uri should be allowed: %v", err)
	}
	if err := p.AllowBaseURI("https://evil.com/", base); err == nil {
		t.Error("cross-origin base-uri should be blocked")
	}
}

func TestAllowScript_InlineScript(t *testing.T) {
	base := mustParseURL("https://example.com/page")

	// Inline script (empty URL) with script-src 'self' should be blocked
	// because 'self' never matches inline scripts (CSP spec).
	p := net.ParseCSPHeader("script-src 'self'")
	if err := p.AllowScript("", base); err == nil {
		t.Error("inline script should be blocked with script-src 'self'")
	}

	// Inline script with default-src 'self' should also be blocked.
	p2 := net.ParseCSPHeader("default-src 'self'")
	if err := p2.AllowScript("", base); err == nil {
		t.Error("inline script should be blocked with default-src 'self'")
	}

	// No CSP at all should allow inline scripts.
	p3 := net.ParseCSPHeader("img-src 'self'")
	if err := p3.AllowScript("", base); err != nil {
		t.Errorf("inline script with no script/default-src should be allowed: %v", err)
	}

	// No CSP at all (nil policy) should allow inline scripts.
	var nilPolicy *net.CSPPolicy
	if err := nilPolicy.AllowScript("", base); err != nil {
		t.Errorf("nil policy should allow inline scripts: %v", err)
	}
}

func TestMatchesHostWildcard(t *testing.T) {
	tests := []struct {
		host   string
		domain string
		want   bool
	}{
		{"example.com", "example.com", true},
		{"cdn.example.com", "example.com", true},
		{"a.b.c.example.com", "example.com", true},
		{"notexample.com", "example.com", false},
		{"evil.com", "example.com", false},
		{"example.com.evil.com", "example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.host+"/"+tt.domain, func(t *testing.T) {
			got := net.MatchesHostWildcard(tt.host, tt.domain)
			if got != tt.want {
				t.Errorf("net.MatchesHostWildcard(%q, %q) = %v, want %v", tt.host, tt.domain, got, tt.want)
			}
		})
	}
}

func TestAllowsSource(t *testing.T) {
	tests := []struct {
		name    string
		sources []string
		target  string
		base    string
		want    bool
	}{
		{"none blocks", []string{"'none'"}, "https://example.com", "https://example.com", false},
		{"self match", []string{"'self'"}, "https://example.com/path", "https://example.com", true},
		{"self no match", []string{"'self'"}, "https://other.com/path", "https://example.com", false},
		{"scheme match", []string{"https:"}, "https://example.com", "https://example.com", true},
		{"scheme no match", []string{"http:"}, "https://example.com", "https://example.com", false},
		{"wildcard match", []string{"*.example.com"}, "https://cdn.example.com", "https://example.com", true},
		{"wildcard no match", []string{"*.example.com"}, "https://evil.com", "https://example.com", false},
		{"host match", []string{"example.com"}, "https://example.com/path", "https://example.com", true},
		{"host no match", []string{"example.com"}, "https://other.com/path", "https://example.com", false},
		{"full url match", []string{"https://example.com/app.js"}, "https://example.com/app.js", "https://example.com", true},
		{"full url mismatch", []string{"https://example.com/app.js"}, "https://example.com/other.js", "https://example.com", false},
		{"multiple sources first matches", []string{"'none'", "https:"}, "https://example.com", "https://example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := mustParseURL(tt.target)
			base := mustParseURL(tt.base)
			got := net.AllowsSource(tt.sources, target, base)
			if got != tt.want {
				t.Errorf("AllowsSource(%v, %q, %q) = %v, want %v", tt.sources, tt.target, tt.base, got, tt.want)
			}
		})
	}
}

func TestCSPPolicy_DirectiveFallback(t *testing.T) {
	// script-src absent, default-src present → fallback
	p := net.ParseCSPHeader("default-src 'self'")
	base := mustParseURL("https://example.com")
	if err := p.AllowScript("https://example.com/app.js", base); err != nil {
		t.Errorf("should fall back to default-src: %v", err)
	}
	if err := p.AllowScript("https://evil.com/app.js", base); err == nil {
		t.Error("should block cross-origin via default-src fallback")
	}

	// script-src present, default-src different → script-src wins
	p2 := net.ParseCSPHeader("default-src 'none'; script-src *.example.com")
	if err := p2.AllowScript("https://cdn.example.com/app.js", base); err != nil {
		t.Errorf("explicit script-src should override default-src: %v", err)
	}
}

func BenchmarkParseCSPHeader(b *testing.B) {
	header := "default-src 'self'; script-src 'self' https://cdn.example.com https://fonts.example.com; style-src 'self' https://fonts.example.com; connect-src https://api.example.com; img-src 'self' https:"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = net.ParseCSPHeader(header)
	}
}

func BenchmarkCSPPolicy_AllowsURL(b *testing.B) {
	p := net.ParseCSPHeader("default-src 'self'; script-src 'self' https://cdn.example.com; connect-src https://api.example.com")
	base := mustParseURL("https://example.com/page")
	rawURL := "https://cdn.example.com/app.js"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = p.AllowsURL("script-src", rawURL, base)
	}
}

func BenchmarkAllowScript(b *testing.B) {
	p := net.ParseCSPHeader("script-src 'self' https://cdn.example.com")
	base := mustParseURL("https://example.com/page")
	rawURL := "https://cdn.example.com/app.js"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = p.AllowScript(rawURL, base)
	}
}

func BenchmarkAllowScriptURL(b *testing.B) {
	p := net.ParseCSPHeader("script-src 'self' https://cdn.example.com")
	base := mustParseURL("https://example.com/page")
	rawURL := mustParseURL("https://cdn.example.com/app.js")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = p.AllowScriptURL(rawURL, base)
	}
}
