package js

import (
	"net/url"
	"strings"
)

// CORS checking for fetch requests.

// IsCORSRequest returns true if the request URL is cross-origin relative to
// the document URL.
func IsCORSRequest(documentURL, requestURL string) bool {
	doc, err := url.Parse(documentURL)
	if err != nil {
		return false
	}
	req, err := url.Parse(requestURL)
	if err != nil {
		return false
	}
	return doc.Scheme != req.Scheme || doc.Host != req.Host
}

// IsSimpleMethod returns true for CORS-safelisted methods (GET, HEAD, POST).
func IsSimpleMethod(method string) bool {
	switch strings.ToUpper(method) {
	case "GET", "HEAD", "POST":
		return true
	}
	return false
}

// isSimpleHeader returns true for CORS-safelisted request headers.
func isSimpleHeader(name string) bool {
	lower := strings.ToLower(name)
	switch lower {
	case "accept", "accept-language", "content-language", "content-type":
		return true
	}
	return false
}

// isSimpleContentType returns true for CORS-safelisted content types.
func isSimpleContentType(ct string) bool {
	lower := strings.ToLower(ct)
	return strings.HasPrefix(lower, "application/x-www-form-urlencoded") ||
		strings.HasPrefix(lower, "multipart/form-data") ||
		strings.HasPrefix(lower, "text/plain")
}

// NeedsPreflight returns true if the request requires a CORS preflight.
func NeedsPreflight(method string, headers map[string]string) bool {
	if !IsSimpleMethod(method) {
		return true
	}
	for name, value := range headers {
		if !isSimpleHeader(name) {
			return true
		}
		if strings.ToLower(name) == "content-type" && !isSimpleContentType(value) {
			return true
		}
	}
	return false
}

// checkCORSResponse validates the Access-Control-Allow-Origin header.
func checkCORSResponse(origin string, headers map[string]string) bool {
	allowOrigin := headers["access-control-allow-origin"]
	if allowOrigin == "" {
		allowOrigin = headers["Access-Control-Allow-Origin"]
	}
	if allowOrigin == "" {
		return false
	}
	if allowOrigin == "*" {
		return true
	}
	return allowOrigin == origin
}

// buildPreflightHeaders returns headers for a CORS preflight request.
func buildPreflightHeaders(origin, method string, headers map[string]string) map[string]string {
	pfHeaders := map[string]string{
		"Origin":                        origin,
		"Access-Control-Request-Method": method,
	}

	var nonSimple []string
	for name := range headers {
		if !isSimpleHeader(name) {
			nonSimple = append(nonSimple, name)
		}
	}
	if len(nonSimple) > 0 {
		pfHeaders["Access-Control-Request-Headers"] = strings.Join(nonSimple, ", ")
	}

	return pfHeaders
}

// getOrigin returns the origin of a URL (scheme + host).
func getOrigin(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}
