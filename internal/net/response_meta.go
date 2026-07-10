// Package net provides HTTP fetching, caching, and response metadata for the browser engine.
package net

import (
	"net/http"
	"strings"
)

// ResponseMeta preserves immutable metadata from an HTTP response for security
// inspection, developer tools, and caching decisions. All fields are populated
// at capture time and remain valid after the response body is closed.
type ResponseMeta struct {
	// Status is the HTTP status code (e.g., 200, 404).
	Status int

	// ContentType is the full Content-Type header value.
	ContentType string

	// ContentLength is the declared body length in bytes, or -1 if unknown.
	ContentLength int64

	// ContentEncoding is the Content-Encoding header (e.g., "gzip", "br").
	ContentEncoding string

	// Protocol is the HTTP protocol version (e.g., "HTTP/1.1", "HTTP/2.0").
	Protocol string

	// Charset is the charset parameter extracted from Content-Type, if present.
	Charset string

	// Header contains a copy of all response headers.
	Header http.Header

	// Cached indicates whether this metadata came from a cache hit rather
	// than a live network response.
	Cached bool
}

// ResponseMetaFromResponse captures response metadata into an immutable struct.
// Returns a zero-value meta with an empty Header if resp is nil.
func ResponseMetaFromResponse(resp *http.Response) ResponseMeta {
	meta := ResponseMeta{
		Header: make(http.Header),
	}
	if resp == nil {
		return meta
	}
	meta.Status = resp.StatusCode
	meta.ContentType = resp.Header.Get("Content-Type")
	meta.ContentLength = resp.ContentLength
	meta.ContentEncoding = resp.Header.Get("Content-Encoding")
	meta.Protocol = resp.Proto
	meta.Charset = extractCharset(meta.ContentType)
	// Copy headers to prevent mutation after body close.
	for k, v := range resp.Header {
		meta.Header[k] = append([]string(nil), v...)
	}
	return meta
}

// ResponseMetaFromCacheEntry synthesizes metadata from a cache entry when the
// original response is no longer available.
func ResponseMetaFromCacheEntry(entry CacheEntry) ResponseMeta {
	return ResponseMeta{
		Status:      entry.Status,
		ContentType: entry.ContentType,
		Header:      make(http.Header),
		Cached:      true,
	}
}

// extractCharset parses the charset parameter from a Content-Type value.
// Returns empty string if no charset is present.
func extractCharset(contentType string) string {
	// Look for "charset=" in the content type (case-insensitive)
	lower := strings.ToLower(contentType)
	idx := strings.Index(lower, "charset=")
	if idx < 0 {
		return ""
	}
	value := contentType[idx+len("charset="):]
	// Handle quoted values
	if len(value) > 0 && value[0] == '"' {
		value = value[1:]
		if end := strings.IndexByte(value, '"'); end >= 0 {
			value = value[:end]
		}
		return value
	}
	// Unquoted: take until semicolon, comma, or whitespace
	for i, c := range value {
		if c == ';' || c == ',' || c == ' ' || c == '\t' {
			return value[:i]
		}
	}
	return value
}
