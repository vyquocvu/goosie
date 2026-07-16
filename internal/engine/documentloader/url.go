package documentloader

import (
	"errors"
	"net/url"
	"strings"
)

// ErrInvalidBaseURL is returned when ResolveURL is given a base URL that
// cannot be parsed. The caller should treat this as a fatal error for
// the navigation.
var ErrInvalidBaseURL = errors.New("documentloader: invalid base URL")

// ResolveURL resolves ref against base per RFC 3986 section 5. The base
// MUST be the final document URL after redirects, not the original
// request URL. Resolution rules:
//
//   - ref is empty → return base unchanged.
//   - ref has a scheme → return ref unchanged (absolute URL).
//   - ref starts with "//" → reuse base scheme.
//   - ref starts with "/" → resolve against base origin.
//   - ref starts with "?" → reuse base path, replace query.
//   - ref starts with "#" → reuse base path+query, replace fragment.
//   - otherwise → resolve as a relative path against base path.
//
// The returned string is the fully qualified URL with no trailing
// whitespace or fragment-prefixed content that the caller did not ask
// for. The function never silently drops a fragment; pass "#frag" if
// the caller wants the document URL with the fragment stripped.
func ResolveURL(base, ref string) (string, error) {
	if strings.TrimSpace(ref) == "" {
		return base, nil
	}
	baseURL, err := url.Parse(strings.TrimSpace(base))
	if err != nil || baseURL == nil {
		return "", ErrInvalidBaseURL
	}
	refURL, err := url.Parse(ref)
	if err != nil || refURL == nil {
		return "", &SkippedError{Reason: "invalid resource URL: " + ref}
	}
	// url.Parse treats "#frag" as relative to base; ResolveReference
	// handles every case per RFC 3986.
	resolved := baseURL.ResolveReference(refURL)
	return resolved.String(), nil
}

// IsHTTPOrHTTPS reports whether rawURL has an http(s) scheme. Used by
// the coordinator to short-circuit non-network resources (data:, blob:,
// about:, file:, mailto:, javascript:, etc.) before calling CSP, since
// those schemes cannot meaningfully reach CSP source expressions.
func IsHTTPOrHTTPS(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u == nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}