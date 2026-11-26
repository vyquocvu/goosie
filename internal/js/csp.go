package js

import (
	"net/url"
	"strings"
)

type CSPPolicy struct {
	Directives map[string][]string
	ReportOnly bool
	Policies   []*CSPPolicy
}

func ComposeCSP(policies ...*CSPPolicy) *CSPPolicy {
	enforcing := make([]*CSPPolicy, 0, len(policies))
	for _, policy := range policies {
		if policy != nil && !policy.ReportOnly {
			enforcing = append(enforcing, policy)
		}
	}
	switch len(enforcing) {
	case 0:
		return nil
	case 1:
		return enforcing[0]
	default:
		return &CSPPolicy{Policies: enforcing}
	}
}

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

func (p *CSPPolicy) AllowsScript(src string, documentURL string) bool {
	return p.all(func(policy *CSPPolicy) bool {
		return policy.allows("script-src", src, documentURL)
	})
}

func (r *Runtime) AllowsScriptURL(src, documentURL string) bool {
	return r != nil && r.opts.CSP.AllowsScript(src, documentURL)
}

func (r *Runtime) AllowsInlineScriptWithNonce(nonce string) bool {
	return r != nil && r.opts.CSP.AllowsInlineScriptWithNonce(nonce)
}

func (p *CSPPolicy) AllowsScriptWithNonce(src, nonce, documentURL string) bool {
	return p.all(func(policy *CSPPolicy) bool {
		return nonce != "" && policy.hasNonce("script-src", nonce) || policy.allows("script-src", src, documentURL)
	})
}

func (p *CSPPolicy) AllowsStyle(src string, documentURL string) bool {
	return p.all(func(policy *CSPPolicy) bool {
		return policy.allows("style-src", src, documentURL)
	})
}

func (p *CSPPolicy) AllowsStyleWithNonce(src, nonce, documentURL string) bool {
	return p.all(func(policy *CSPPolicy) bool {
		return nonce != "" && policy.hasNonce("style-src", nonce) || policy.allows("style-src", src, documentURL)
	})
}

func (p *CSPPolicy) AllowsImage(src string, documentURL string) bool {
	return p.all(func(policy *CSPPolicy) bool {
		return policy.allows("img-src", src, documentURL)
	})
}

func (p *CSPPolicy) AllowsConnect(src string, documentURL string) bool {
	return p.all(func(policy *CSPPolicy) bool {
		return policy.allows("connect-src", src, documentURL)
	})
}

func (p *CSPPolicy) AllowsFont(src string, documentURL string) bool {
	return p.all(func(policy *CSPPolicy) bool {
		return policy.allows("font-src", src, documentURL)
	})
}

func (p *CSPPolicy) AllowsMedia(src string, documentURL string) bool {
	return p.all(func(policy *CSPPolicy) bool {
		return policy.allows("media-src", src, documentURL)
	})
}

func (p *CSPPolicy) AllowsFrame(src string, documentURL string) bool {
	return p.all(func(policy *CSPPolicy) bool {
		return policy.allows("frame-src", src, documentURL)
	})
}

func (p *CSPPolicy) AllowsWorker(src string, documentURL string) bool {
	return p.all(func(policy *CSPPolicy) bool {
		return policy.allows("worker-src", src, documentURL)
	})
}

func (p *CSPPolicy) AllowsInlineScript() bool {
	return p.all(func(policy *CSPPolicy) bool {
		return policy.allowsInline("script-src")
	})
}

func (p *CSPPolicy) AllowsInlineScriptWithNonce(nonce string) bool {
	return p.all(func(policy *CSPPolicy) bool {
		return nonce != "" && policy.hasNonce("script-src", nonce) || policy.allowsInline("script-src")
	})
}

func (p *CSPPolicy) AllowsInlineStyle() bool {
	return p.all(func(policy *CSPPolicy) bool {
		return policy.allowsInline("style-src")
	})
}

func (p *CSPPolicy) AllowsInlineStyleWithNonce(nonce string) bool {
	return p.all(func(policy *CSPPolicy) bool {
		return nonce != "" && policy.hasNonce("style-src", nonce) || policy.allowsInline("style-src")
	})
}

func (p *CSPPolicy) AllowsEval() bool {
	return p.all(func(policy *CSPPolicy) bool {
		return policy.hasSource("script-src", "'unsafe-eval'")
	})
}

func (p *CSPPolicy) all(allowed func(*CSPPolicy) bool) bool {
	if p == nil || p.ReportOnly {
		return true
	}
	if len(p.Policies) == 0 {
		return allowed(p)
	}
	for _, policy := range p.Policies {
		if !policy.all(allowed) {
			return false
		}
	}
	return true
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
	}
	if directive != "default-src" {
		if sources, ok := p.Directives["default-src"]; ok {
			return sources
		}
	}
	return nil
}

func isSameOrigin(url1, url2 string) bool {
	u2, err := url.Parse(url2)
	if err != nil {
		return false
	}
	u1, err := url.Parse(url1)
	if err != nil {
		return false
	}
	if !u1.IsAbs() {
		u1 = u2.ResolveReference(u1)
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

	if strings.HasSuffix(pattern, ":") && !strings.Contains(pattern, "/") {
		return strings.EqualFold(u.Scheme+":", pattern)
	}

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

	if targetScheme != "" && !strings.EqualFold(u.Scheme, targetScheme) {
		return false
	}

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

	if targetPath != "" {
		if !strings.HasPrefix(u.Path, targetPath) {
			return false
		}
	}

	return true
}
