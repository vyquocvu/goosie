package engine

import (
	"net/url"
	"strings"
)

// Cross-origin policy for @font-face subresources. Browsers gate web fonts
// on CORS: a same-origin face loads freely, a cross-origin one needs an
// Access-Control-Allow-Origin grant. (The Fetch API's fuller CORS machinery
// lives in internal/js, which the engine cannot import; this is the small
// subresource half, kept beside resolveSubresourceURL's scheme and
// private-network checks.)
//
// Only http(s)-against-http(s) pairs are judged: other schemes (a file://
// document loading any face) are allowed, the same leniency the
// private-network rule extends to local initiators. Anything unparseable
// fails closed.

// fontCORSAllowed reports whether a fetched face may register, given the
// document base it resolved against and the response headers.
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
		// Deterministic fallback for non-canonical maps: exact key first,
		// then any casing (a map holding two casings with different values
		// must not resolve by iteration order).
		for k, v := range headers {
			if strings.EqualFold(k, "Access-Control-Allow-Origin") {
				allow = strings.TrimSpace(v)
				break
			}
		}
	}
	if allow == "*" {
		// Treated as anonymous: our font fetcher sends no credentials, so
		// the wildcard grant applies. (With credentials it must not.)
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

// sameOrigin compares scheme and host (including any port), the two parts
// that define an origin for CORS purposes.
func sameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}
