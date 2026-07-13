package net

import (
	"errors"
	"strings"
)

// MIME type constants for resource classification.
const (
	MIMETextHTML       = "text/html"
	MIMETextCSS        = "text/css"
	MIMETextJavaScript = "text/javascript"
	MIMEApplicationJS  = "application/javascript"
	MIMEImageSVG       = "image/svg+xml"
	MIMEImagePNG       = "image/png"
	MIMEImageJPEG      = "image/jpeg"
	MIMEImageGIF       = "image/gif"
	MIMEImageWebP      = "image/webp"
	MIMEImageAVIF      = "image/avif"
	MIMEImageBMP       = "image/bmp"
	MIMEImageICO       = "image/x-icon"
)

// ErrUnsupportedMediaType is returned when a response's Content-Type does not
// match any of the allowed media types for the requested resource. Callers can
// check it with errors.Is.
var ErrUnsupportedMediaType = errors.New("unsupported media type")

// MediaTypeClass classifies a response into a broad resource category.
type MediaTypeClass int

const (
	MediaTypeUnknown MediaTypeClass = iota
	MediaTypeHTML
	MediaTypeCSS
	MediaTypeJavaScript
	MediaTypeImage
)

// String returns a human-readable label for the media type class.
func (c MediaTypeClass) String() string {
	switch c {
	case MediaTypeHTML:
		return "html"
	case MediaTypeCSS:
		return "css"
	case MediaTypeJavaScript:
		return "javascript"
	case MediaTypeImage:
		return "image"
	default:
		return "unknown"
	}
}

// SupportedMediaTypes lists the Content-Type prefixes accepted for each
// resource class. Entries are matched case-insensitively against the
// media-type/subtype portion of Content-Type (after stripping parameters
// and whitespace). The list is ordered so that the first match wins.
var SupportedMediaTypes = map[MediaTypeClass][]string{
	MediaTypeHTML: {
		"text/html",
	},
	MediaTypeCSS: {
		"text/css",
	},
	MediaTypeJavaScript: {
		"text/javascript",
		"application/javascript",
		"application/x-javascript",
		"text/x-javascript",
	},
	MediaTypeImage: {
		"image/png",
		"image/jpeg",
		"image/gif",
		"image/webp",
		"image/avif",
		"image/bmp",
		"image/x-icon",
		"image/svg+xml",
	},
}

// ClassifyContentType parses a raw Content-Type header value and returns
// the broad resource class it belongs to. Returns MediaTypeUnknown for
// unrecognized, empty, or malformed types.
func ClassifyContentType(raw string) MediaTypeClass {
	mediaType := parseMediaType(raw)
	if mediaType == "" {
		return MediaTypeUnknown
	}
	for class, types := range SupportedMediaTypes {
		for _, t := range types {
			if mediaType == t {
				return class
			}
		}
	}
	return MediaTypeUnknown
}

// ValidateContentType checks whether rawContentType matches at least one of
// the allowedTypes. Both sides are normalized (parameters stripped, lowercased,
// whitespace trimmed) before comparison.
//
// An empty rawContentType is always rejected (returns ErrUnsupportedMediaType).
// An empty allowedTypes slice means "accept all" and always returns nil.
//
// Returns nil on match, or ErrUnsupportedMediaType with a descriptive Message
// on mismatch.
func ValidateContentType(rawContentType string, allowedTypes []string) error {
	if len(allowedTypes) == 0 {
		return nil
	}
	mediaType := parseMediaType(rawContentType)
	if mediaType == "" {
		return ErrUnsupportedMediaType
	}
	for _, allowed := range allowedTypes {
		if mediaType == allowed {
			return nil
		}
	}
	return ErrUnsupportedMediaType
}

// parseMediaType extracts the media-type/subtype portion from a full
// Content-Type header value, stripping parameters (charset, boundary, etc.)
// and whitespace. Returns the lowercased media type, or empty string for
// empty/malformed input.
//
// Examples:
//
//	"text/html; charset=utf-8"  → "text/html"
//	"Text/HTML"                 → "text/html"
//	"application/json "         → "application/json"
//	""                          → ""
func parseMediaType(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// Strip parameters: split on ';' and take the first part.
	if idx := strings.IndexByte(raw, ';'); idx >= 0 {
		raw = raw[:idx]
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	return strings.ToLower(raw)
}
