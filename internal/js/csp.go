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

// AllowsScriptURL reports whether the runtime's document policy permits an
// external script URL. Hosts use it when admitting dynamically inserted tags.
func (r *Runtime) AllowsScriptURL(src, documentURL string) bool {
	return r != nil && r.opts.CSP.AllowsScript(src, documentURL)
}

// AllowsScriptWithNonce returns true if the script is allowed by source or by nonce.
func (p *CSPPolicy) AllowsScriptWithNonce(src, nonce, documentURL string) bool {
	if p == nil {
		return true
	}
	if nonce != "" && p.hasNonce("script-src", nonce) {
		return true
	}
	return p.AllowsScript(src, documentURL)
}

// AllowsStyle returns true if the style source is allowed by the policy.
func (p *CSPPolicy) AllowsStyle(src string, documentURL string) bool {
	if p == nil {
		return true
	}
	return p.allows("style-src", src, documentURL)
}

// AllowsStyleWithNonce returns true if the style is allowed by source or by nonce.
func (p *CSPPolicy) AllowsStyleWithNonce(src, nonce, documentURL string) bool {
	if p == nil {
		return true
	}
	if nonce != "" && p.hasNonce("style-src", nonce) {
		return true
	}
	return p.AllowsStyle(src, documentURL)
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

// AllowsFrame returns true if frame/iframe source is allowed by the policy.
func (p *CSPPolicy) AllowsFrame(src string, documentURL string) bool {
	if p == nil {
		return true
	}
	return p.allows("frame-src", src, documentURL)
}

// AllowsWorker returns true if dedicated or shared worker source is allowed by the policy.
func (p *CSPPolicy) AllowsWorker(src string, documentURL string) bool {
	if p == nil {
		return true
	}
	return p.allows("worker-src", src, documentURL)
}

// AllowsInlineScript returns true if inline scripts are allowed.
func (p *CSPPolicy) AllowsInlineScript() bool {
	if p == nil {
		return true
	}
	return p.allowsInline("script-src")
}

// AllowsInlineScriptWithNonce returns true if inline script is allowed either by nonce or 'unsafe-inline'.
func (p *CSPPolicy) AllowsInlineScriptWithNonce(nonce string) bool {
	if p == nil {
		return true
	}
	if nonce != "" && p.hasNonce("script-src", nonce) {
		return true
	}
	return p.AllowsInlineScript()
}

// AllowsInlineStyle returns true if inline styles are allowed.
func (p *CSPPolicy) AllowsInlineStyle() bool {
	if p == nil {
		return true
	}
	return p.allowsInline("style-src")
}

// AllowsInlineStyleWithNonce returns true if inline style is allowed either by nonce or 'unsafe-inline'.
func (p *CSPPolicy) AllowsInlineStyleWithNonce(nonce string) bool {
	if p == nil {
		return true
	}
	if nonce != "" && p.hasNonce("style-src", nonce) {
		return true
	}
	return p.AllowsInlineStyle()
}

// AllowsEval returns true if eval() is allowed.
func (p *CSPPolicy) AllowsEval() bool {
	if p == nil {
		return true
	}
	return p.hasSource("script-src", "'unsafe-eval'")
}

func (p *CSPPolicy) hasNonce(directive, nonce string) bool {
	sources := p.getSources(directive)
	wanted := "'nonce-" + nonce + "'"
	for _, s := range sources {
		if s == wanted {
			return true
		}
	}
	return false
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
	// Fallback hierarchy per CSP Level 3:
	switch directive {
	case "frame-src":
		if sources, ok := p.Directives["child-src"]; ok {
			return sources
		}
	case "worker-src":
		if sources, ok := p.Directives["child-src"]; ok {
			return sources
		}
		if sources, ok := p.Directives["script-src"]; ok {
			return sources
		}
	case "child-src":
		// child-src falls back directly to default-src
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

	u, err := url.Parse(src)
	if err != nil {
		return false
	}

	// Scheme source matching (e.g. "https:", "http:", "data:", "blob:")
	if strings.HasSuffix(pattern, ":") && !strings.Contains(pattern, "/") {
		return strings.EqualFold(u.Scheme+":", pattern)
	}

	// Host / URL pattern matching
	targetScheme := ""
	targetHost := pattern
	targetPath := ""

	if strings.Contains(pattern, "://") {
		parts := strings.SplitN(pattern, "://", 2)
		targetScheme = strings.ToLower(parts[0])
		targetHost = parts[1]
	}

	if slash := strings.IndexByte(targetHost, '/'); slash >= 0 {
		targetPath = targetHost[slash:]
		targetHost = targetHost[:slash]
	}

	// If scheme is specified, verify scheme matches
	if targetScheme != "" && !strings.EqualFold(u.Scheme, targetScheme) {
		return false
	}

	// Port matching
	srcHost := u.Hostname()
	srcPort := u.Port()

	targetPort := ""
	if colon := strings.LastIndexByte(targetHost, ':'); colon >= 0 {
		targetPort = targetHost[colon+1:]
		targetHost = targetHost[:colon]
	}

	if targetPort != "" && targetPort != "*" && targetPort != srcPort {
		return false
	}

	// Domain / Subdomain matching
	if targetHost != "" && targetHost != "*" {
		if strings.HasPrefix(targetHost, "*.") {
			baseDomain := strings.ToLower(targetHost[2:])
			srcLower := strings.ToLower(srcHost)
			if srcLower != baseDomain && !strings.HasSuffix(srcLower, "."+baseDomain) {
				return false
			}
		} else {
			if !strings.EqualFold(srcHost, targetHost) {
				return false
			}
		}
	}

	// Path prefix matching
	if targetPath != "" {
		if !strings.HasPrefix(u.Path, targetPath) {
			return false
		}
	}

	return true
}
