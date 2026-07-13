package net

import (
	"errors"
	"net/url"
	"strings"
)

// CSP directive names recognized by the engine subset. Unrecognised directives
// are silently ignored per the CSP spec (unknown directives have no effect).
const (
	CSPDefaultSrc = "default-src"
	CSPScriptSrc  = "script-src"
	CSPStyleSrc   = "style-src"
	CSPConnectSrc = "connect-src"
	CSPBaseURI    = "base-uri"
)

// Source keyword constants.
const (
	CSPSourceNone  = "'none'"
	CSPSourceSelf  = "'self'"
	CSPSchemeHTTPS = "https:"
	CSPSchemeHTTP  = "http:"
	CSPSchemeData  = "data:"
	CSPSchemeBlob  = "blob:"
)

// ErrCSPViolation is returned when a URL is blocked by a Content Security
// Policy directive. Callers can check it with errors.Is. The error message
// identifies which directive was violated.
var ErrCSPViolation = errors.New("csp: policy violation")

// CSPPolicy represents a parsed Content-Security-Policy header. A nil or
// zero-value policy permits all requests (no restrictions).
type CSPPolicy struct {
	// directives maps directive name → source list (space-separated tokens
	// already split into individual strings).
	directives map[string][]string
}

// ParseCSPHeader parses one or more Content-Security-Policy header values.
// Multiple headers are merged in order (later values append sources). Returns
// nil for empty input.
//
// Example header:
//
//	default-src 'self'; script-src 'self' https://cdn.example.com; style-src 'self'
func ParseCSPHeader(headers ...string) *CSPPolicy {
	merged := make(map[string][]string)
	for _, h := range headers {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		// Split directives on ';'
		for _, part := range strings.Split(h, ";") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			// Split directive name from source list on first whitespace
			name, sourcesRaw, _ := strings.Cut(part, " ")
			name = strings.ToLower(strings.TrimSpace(name))
			if name == "" {
				continue
			}
			sourcesRaw = strings.TrimSpace(sourcesRaw)
			if sourcesRaw == "" {
				continue
			}
			merged[name] = append(merged[name], strings.Fields(sourcesRaw)...)
		}
	}
	if len(merged) == 0 {
		return nil
	}
	return &CSPPolicy{directives: merged}
}

// AllowsSource reports whether the given source list contains a source
// expression that matches the target URL. The baseURL is the document's
// own URL, used to resolve 'self' comparisons.
func AllowsSource(sources []string, target *url.URL, baseURL *url.URL) bool {
	for _, src := range sources {
		switch {
		case src == CSPSourceNone:
			// 'none' matches nothing; skip.
		case src == CSPSourceSelf:
			if baseURL != nil && target.Scheme == baseURL.Scheme &&
				target.Host == baseURL.Host {
				return true
			}
		case strings.Contains(src, "://"):
			// Full URL source: exact match against target.
			if target.String() == src {
				return true
			}
		case strings.HasSuffix(src, ":"):
			// Scheme source: matches any URL using that scheme.
			if target.Scheme == strings.TrimSuffix(src, ":") {
				return true
			}
		case strings.HasPrefix(src, "*."):
			// Wildcard host: *.example.com matches example.com and
			// any subdomain.
			wildcardHost := src[2:] // e.g. "example.com"
			if matchesHostWildcard(target.Host, wildcardHost) {
				return true
			}
		case strings.Contains(src, "/"):
			// Host source with optional path: "example.com/path"
			if matchesHostSource(src, target) {
				return true
			}
		default:
			// Bare host: exact host match (including port).
			if target.Host == src {
				return true
			}
		}
	}
	return false
}

// matchesHostWildcard reports whether host matches *.domain — i.e. host
// equals domain exactly or is a subdomain of domain.
func matchesHostWildcard(host, domain string) bool {
	if host == domain {
		return true
	}
	return strings.HasSuffix(host, "."+domain)
}

// matchesHostSource matches a source like "example.com" or "example.com/path"
// against a target URL. The source may include a port.
func matchesHostSource(source string, target *url.URL) bool {
	// Split source into host and optional path.
	hostPart, pathPart, _ := strings.Cut(source, "/")

	// Match host (exact or subdomain).
	if hostPart != target.Host {
		// Check if hostPart is a registrable suffix match.
		if !strings.HasSuffix(target.Host, "."+hostPart) {
			return false
		}
	}

	// If the source has no path component, any path matches.
	if pathPart == "" {
		return true
	}

	// Path prefix match: target path must start with the source path.
	return strings.HasPrefix(target.Path, "/"+pathPart)
}

// HasDirective reports whether the policy contains the given directive name.
func (p *CSPPolicy) HasDirective(name string) bool {
	if p == nil {
		return false
	}
	_, ok := p.directives[name]
	return ok
}

// Sources returns the source list for the given directive. Returns nil if the
// directive is not present or the policy is nil.
func (p *CSPPolicy) Sources(name string) []string {
	if p == nil {
		return nil
	}
	return p.directives[name]
}

// AllowsURL checks whether a URL is permitted by the given directive. The
// directive is resolved with the standard CSP fallback chain: if the
// directive is absent, default-src is consulted; if default-src is also
// absent, the URL is allowed (no policy restriction).
func (p *CSPPolicy) AllowsURL(directive string, rawURL string, baseURL *url.URL) bool {
	if p == nil {
		return true
	}
	sources := p.effectiveSources(directive)
	if len(sources) == 0 {
		// No effective sources = no restriction for this directive.
		return true
	}
	target, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return AllowsSource(sources, target, baseURL)
}

// effectiveSources implements the CSP fallback chain: directive → default-src.
func (p *CSPPolicy) effectiveSources(directive string) []string {
	if p == nil {
		return nil
	}
	if srcs, ok := p.directives[directive]; ok {
		return srcs
	}
	return p.directives[CSPDefaultSrc]
}

// AllowScript reports whether a script at rawURL is permitted by this policy.
// An empty rawURL represents an inline script.
func (p *CSPPolicy) AllowScript(rawURL string, baseURL *url.URL) error {
	if p.AllowsURL(CSPScriptSrc, rawURL, baseURL) {
		return nil
	}
	return ErrCSPViolation
}

// AllowScriptURL reports whether a script at the given pre-parsed URL is
// permitted. Use this instead of AllowScript when you already have a parsed
// URL to avoid an extra allocation.
func (p *CSPPolicy) AllowScriptURL(rawURL *url.URL, baseURL *url.URL) error {
	if p.AllowsURLParsed(CSPScriptSrc, rawURL, baseURL) {
		return nil
	}
	return ErrCSPViolation
}

// AllowStyle reports whether a stylesheet at rawURL is permitted by this policy.
func (p *CSPPolicy) AllowStyle(rawURL string, baseURL *url.URL) error {
	if p.AllowsURL(CSPStyleSrc, rawURL, baseURL) {
		return nil
	}
	return ErrCSPViolation
}

// AllowStyleURL reports whether a stylesheet at the given pre-parsed URL is
// permitted.
func (p *CSPPolicy) AllowStyleURL(rawURL *url.URL, baseURL *url.URL) error {
	if p.AllowsURLParsed(CSPStyleSrc, rawURL, baseURL) {
		return nil
	}
	return ErrCSPViolation
}

// AllowConnect reports whether a fetch/XHR to rawURL is permitted by this policy.
func (p *CSPPolicy) AllowConnect(rawURL string, baseURL *url.URL) error {
	if p.AllowsURL(CSPConnectSrc, rawURL, baseURL) {
		return nil
	}
	return ErrCSPViolation
}

// AllowConnectURL reports whether a fetch/XHR to the given pre-parsed URL is
// permitted.
func (p *CSPPolicy) AllowConnectURL(rawURL *url.URL, baseURL *url.URL) error {
	if p.AllowsURLParsed(CSPConnectSrc, rawURL, baseURL) {
		return nil
	}
	return ErrCSPViolation
}

// AllowBaseURI reports whether setting the document base to rawURL is permitted.
func (p *CSPPolicy) AllowBaseURI(rawURL string, baseURL *url.URL) error {
	if p.AllowsURL(CSPBaseURI, rawURL, baseURL) {
		return nil
	}
	return ErrCSPViolation
}

// AllowsURLParsed is like AllowsURL but accepts a pre-parsed target URL,
// avoiding an allocation from url.Parse.
func (p *CSPPolicy) AllowsURLParsed(directive string, target *url.URL, baseURL *url.URL) bool {
	if p == nil {
		return true
	}
	sources := p.effectiveSources(directive)
	if len(sources) == 0 {
		return true
	}
	if target == nil {
		return false
	}
	return AllowsSource(sources, target, baseURL)
}
