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

// GetHeader searches a string-to-string map case-insensitively.
func GetHeader(headers map[string]string, key string) string {
	for k, v := range headers {
		if strings.EqualFold(k, key) {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// checkCORSResponse validates the Access-Control-Allow-Origin header (without credentials).
func checkCORSResponse(origin string, headers map[string]string) bool {
	return CheckCORSResponse(origin, headers, false)
}

// CheckCORSResponse validates the CORS headers for a response.
// If withCredentials is true, the wildcard '*' is forbidden and
// Access-Control-Allow-Credentials must equal "true".
func CheckCORSResponse(origin string, headers map[string]string, withCredentials bool) bool {
	allowOrigin := GetHeader(headers, "access-control-allow-origin")
	if allowOrigin == "" {
		return false
	}
	if withCredentials {
		// Wildcard '*' is strictly forbidden with credentials per the CORS specification
		if allowOrigin == "*" {
			return false
		}
		if allowOrigin != origin {
			return false
		}
		allowCreds := GetHeader(headers, "access-control-allow-credentials")
		return strings.EqualFold(allowCreds, "true")
	}

	if allowOrigin == "*" {
		return true
	}
	return allowOrigin == origin
}

// CheckPreflightResponse validates preflight OPTIONS response headers against the requested method and headers.
func CheckPreflightResponse(origin, method string, reqHeaders map[string]string, respHeaders map[string]string, withCredentials bool) bool {
	if !CheckCORSResponse(origin, respHeaders, withCredentials) {
		return false
	}

	// Validate method
	if !IsSimpleMethod(method) {
		allowMethods := GetHeader(respHeaders, "access-control-allow-methods")
		if allowMethods == "" {
			return false
		}
		if allowMethods != "*" {
			allowed := false
			for _, m := range strings.Split(allowMethods, ",") {
				if strings.EqualFold(strings.TrimSpace(m), method) {
					allowed = true
					break
				}
			}
			if !allowed {
				return false
			}
		}
	}

	// Validate headers
	allowHeaders := GetHeader(respHeaders, "access-control-allow-headers")
	for name := range reqHeaders {
		if isSimpleHeader(name) {
			continue
		}
		if allowHeaders == "" {
			return false
		}
		if allowHeaders == "*" {
			continue
		}
		allowed := false
		for _, h := range strings.Split(allowHeaders, ",") {
			if strings.EqualFold(strings.TrimSpace(h), name) {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}

	return true
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
