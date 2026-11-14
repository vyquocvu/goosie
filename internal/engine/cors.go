package engine

import (
	"net/url"
	"strings"
)

func fontCORSAllowed(fontBase, srcURL string, headers map[string]string) bool {
	base, err := url.Parse(fontBase)
	if err != nil {
		return false
	}
	target, err := url.Parse(srcURL)
	if err != nil {
		return false
	}
	if !isWebScheme(base.Scheme) || !isWebScheme(target.Scheme) {
		return true
	}
	if sameOrigin(base, target) {
		return true
	}
	allow := strings.TrimSpace(headers["Access-Control-Allow-Origin"])
	if allow == "" {
		for k, v := range headers {
			if strings.EqualFold(k, "Access-Control-Allow-Origin") {
				allow = strings.TrimSpace(v)
				break
			}
		}
	}
	if allow == "*" {
		return true
	}
	if allow == "" {
		return false
	}
	u, err := url.Parse(allow)
	if err != nil {
		return false
	}
	return sameOrigin(base, u)
}

func isWebScheme(scheme string) bool {
	return strings.EqualFold(scheme, "http") || strings.EqualFold(scheme, "https")
}

func sameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}
