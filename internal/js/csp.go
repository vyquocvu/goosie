package js

import (
	"net/url"
	"strings"
)

// Content Security Policy parsing and enforcement.

// CSPPolicy holds a parsed Content-Security-Policy header.
type CSPPolicy struct {
	Directives map[string][]string
	ReportOnly bool
}

// ParseCSP parses a Content-Security-Policy header value.
func ParseCSP(header string) *CSPPolicy {
	policy := &CSPPolicy{
		Directives: make(map[string][]string),
	}

	for _, directive := range strings.Split(header, ";") {
		directive = strings.TrimSpace(directive)
		if directive == "" {
			continue
		}

		parts := strings.Fields(directive)
		if len(parts) == 0 {
			continue
		}

		name := strings.ToLower(parts[0])
		values := parts[1:]
		policy.Directives[name] = values
	}

	return policy
}

// AllowsScript returns true if the script source is allowed by the policy.
func (p *CSPPolicy) AllowsScript(src string, documentURL string) bool {
	if p == nil {
		return true
	}
	return p.allows("script-src", src, documentURL)
}

// AllowsStyle returns true if the style source is allowed by the policy.
func (p *CSPPolicy) AllowsStyle(src string, documentURL string) bool {
	if p == nil {
		return true
	}
	return p.allows("style-src", src, documentURL)
}

// AllowsImage returns true if the image source is allowed by the policy.
func (p *CSPPolicy) AllowsImage(src string, documentURL string) bool {
	if p == nil {
		return true
	}
	return p.allows("img-src", src, documentURL)
}

// AllowsConnect returns true if the connect source is allowed by the policy.
func (p *CSPPolicy) AllowsConnect(src string, documentURL string) bool {
	if p == nil {
		return true
	}
	return p.allows("connect-src", src, documentURL)
}

// AllowsFont returns true if the font source is allowed by the policy.
func (p *CSPPolicy) AllowsFont(src string, documentURL string) bool {
	if p == nil {
		return true
	}
	return p.allows("font-src", src, documentURL)
}

// AllowsMedia returns true if the media source is allowed by the policy.
func (p *CSPPolicy) AllowsMedia(src string, documentURL string) bool {
	if p == nil {
		return true
	}
	return p.allows("media-src", src, documentURL)
}

// AllowsInlineScript returns true if inline scripts are allowed.
func (p *CSPPolicy) AllowsInlineScript() bool {
	if p == nil {
		return true
	}
	return p.allowsInline("script-src")
}

// AllowsInlineStyle returns true if inline styles are allowed.
func (p *CSPPolicy) AllowsInlineStyle() bool {
	if p == nil {
		return true
	}
	return p.allowsInline("style-src")
}

// AllowsEval returns true if eval() is allowed.
func (p *CSPPolicy) AllowsEval() bool {
	if p == nil {
		return true
	}
	return p.hasSource("script-src", "'unsafe-eval'")
}

func (p *CSPPolicy) allows(directive, src, documentURL string) bool {
	sources := p.getSources(directive)
	if len(sources) == 0 {
		return true
	}

	for _, source := range sources {
		if source == "'none'" {
			return false
		}
		if source == "'self'" {
			if isSameOrigin(src, documentURL) {
				return true
			}
			continue
		}
		if source == "*" {
			return true
		}
		if strings.HasPrefix(source, "'") {
			continue
		}
		if matchesSource(src, source) {
			return true
		}
	}

	return false
}

func (p *CSPPolicy) allowsInline(directive string) bool {
	sources := p.getSources(directive)
	if len(sources) == 0 {
		return true
	}
	for _, source := range sources {
		if source == "'unsafe-inline'" {
			return true
		}
	}
	return false
}

func (p *CSPPolicy) hasSource(directive, source string) bool {
	sources := p.getSources(directive)
	for _, s := range sources {
		if s == source {
			return true
		}
	}
	return false
}

func (p *CSPPolicy) getSources(directive string) []string {
	if sources, ok := p.Directives[directive]; ok {
		return sources
	}
	if directive != "default-src" {
		if sources, ok := p.Directives["default-src"]; ok {
			return sources
		}
	}
	return nil
}

func isSameOrigin(url1, url2 string) bool {
	u1, err := url.Parse(url1)
	if err != nil {
		return false
	}
	u2, err := url.Parse(url2)
	if err != nil {
		return false
	}
	return u1.Scheme == u2.Scheme && u1.Host == u2.Host
}

func matchesSource(src, pattern string) bool {
	if pattern == "*" {
		return true
	}

	if strings.HasPrefix(pattern, "*.") {
		domain := pattern[2:]
		u, err := url.Parse(src)
		if err != nil {
			return false
		}
		host := u.Host
		if strings.Contains(host, ":") {
			host = host[:strings.Index(host, ":")]
		}
		return host == domain || strings.HasSuffix(host, "."+domain)
	}

	u, err := url.Parse(src)
	if err != nil {
		return false
	}
	srcOrigin := u.Scheme + "://" + u.Host

	patternU, err := url.Parse(pattern)
	if err != nil {
		return false
	}
	patternOrigin := patternU.Scheme + "://" + patternU.Host

	return srcOrigin == patternOrigin
}
