package net

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateContentType(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		allowed     []string
		wantErr     bool
		errContains string
	}{
		// Normal cases
		{
			name:    "exact match text/html",
			raw:     "text/html",
			allowed: []string{"text/html"},
		},
		{
			name:    "exact match text/css",
			raw:     "text/css",
			allowed: []string{"text/css"},
		},
		{
			name:    "exact match image/png",
			raw:     "image/png",
			allowed: []string{"image/png"},
		},
		{
			name:    "match among multiple allowed",
			raw:     "image/jpeg",
			allowed: []string{"image/png", "image/jpeg", "image/gif"},
		},
		// Parameters
		{
			name:    "with charset parameter",
			raw:     "text/html; charset=utf-8",
			allowed: []string{"text/html"},
		},
		{
			name:    "with boundary parameter",
			raw:     "multipart/form-data; boundary=----WebKitFormBoundary",
			allowed: []string{"multipart/form-data"},
		},
		{
			name:    "with multiple parameters",
			raw:     "text/html; charset=utf-8; boundary=abc",
			allowed: []string{"text/html"},
		},
		// Case insensitivity
		{
			name:    "uppercase subtype",
			raw:     "Text/HTML",
			allowed: []string{"text/html"},
		},
		{
			name:    "mixed case",
			raw:     "TEXT/HTML",
			allowed: []string{"text/html"},
		},
		{
			name:    "uppercase with charset",
			raw:     "Text/HTML; charset=UTF-8",
			allowed: []string{"text/html"},
		},
		// Whitespace
		{
			name:    "leading and trailing whitespace",
			raw:     "  text/html  ",
			allowed: []string{"text/html"},
		},
		{
			name:    "whitespace around parameters",
			raw:     "text/html ; charset=utf-8",
			allowed: []string{"text/html"},
		},
		// Empty/malformed
		{
			name:        "empty raw content type",
			raw:         "",
			allowed:     []string{"text/html"},
			wantErr:     true,
			errContains: "unsupported media type",
		},
		{
			name:        "whitespace only",
			raw:         "   ",
			allowed:     []string{"text/html"},
			wantErr:     true,
			errContains: "unsupported media type",
		},
		{
			name:        "semicolon only",
			raw:         ";",
			allowed:     []string{"text/html"},
			wantErr:     true,
			errContains: "unsupported media type",
		},
		// Mismatch
		{
			name:        "image/png not in html allowed",
			raw:         "image/png",
			allowed:     []string{"text/html"},
			wantErr:     true,
			errContains: "unsupported media type",
		},
		{
			name:        "application/json not allowed",
			raw:         "application/json",
			allowed:     []string{"text/html", "text/css"},
			wantErr:     true,
			errContains: "unsupported media type",
		},
		// Empty allowed = accept all
		{
			name:    "empty allowed accepts everything",
			raw:     "text/html",
			allowed: []string{},
		},
		{
			name:    "empty allowed accepts unknown types",
			raw:     "application/x-custom",
			allowed: []string{},
		},
		// Nil allowed = accept all
		{
			name:    "nil allowed accepts everything",
			raw:     "text/html",
			allowed: nil,
		},
		// Vendor-prefixed
		{
			name:    "vendor-prefixed image type accepted",
			raw:     "image/vnd.microsoft.icon",
			allowed: []string{"image/vnd.microsoft.icon"},
		},
		{
			name:        "vendor-prefixed not in allowed list",
			raw:         "image/vnd.microsoft.icon",
			allowed:     []string{"image/png", "image/jpeg"},
			wantErr:     true,
			errContains: "unsupported media type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateContentType(tt.raw, tt.allowed)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ValidateContentType(%q, %v) returned nil, want error", tt.raw, tt.allowed)
				}
				if !errors.Is(err, ErrUnsupportedMediaType) {
					t.Errorf("error = %v, want ErrUnsupportedMediaType", err)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
				}
			} else {
				if err != nil {
					t.Fatalf("ValidateContentType(%q, %v) returned error: %v", tt.raw, tt.allowed, err)
				}
			}
		})
	}
}

func TestParseMediaType(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"text/html", "text/html"},
		{"text/html; charset=utf-8", "text/html"},
		{"Text/HTML", "text/html"},
		{"TEXT/HTML; charset=UTF-8", "text/html"},
		{"  text/html  ", "text/html"},
		{"text/html ; charset=utf-8", "text/html"},
		{"", ""},
		{"   ", ""},
		{";", ""},
		{" ; ", ""},
		{"text/html; charset=utf-8; boundary=abc", "text/html"},
		{"application/json", "application/json"},
		{"image/svg+xml", "image/svg+xml"},
		{"image/png", "image/png"},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got := parseMediaType(tt.raw)
			if got != tt.want {
				t.Errorf("parseMediaType(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestClassifyContentType(t *testing.T) {
	tests := []struct {
		raw  string
		want MediaTypeClass
	}{
		{"text/html", MediaTypeHTML},
		{"text/html; charset=utf-8", MediaTypeHTML},
		{"Text/HTML", MediaTypeHTML},
		{"text/css", MediaTypeCSS},
		{"text/css; charset=utf-8", MediaTypeCSS},
		{"text/javascript", MediaTypeJavaScript},
		{"application/javascript", MediaTypeJavaScript},
		{"application/x-javascript", MediaTypeJavaScript},
		{"text/x-javascript", MediaTypeJavaScript},
		{"image/png", MediaTypeImage},
		{"image/jpeg", MediaTypeImage},
		{"image/gif", MediaTypeImage},
		{"image/webp", MediaTypeImage},
		{"image/avif", MediaTypeImage},
		{"image/bmp", MediaTypeImage},
		{"image/x-icon", MediaTypeImage},
		{"image/svg+xml", MediaTypeImage},
		{"application/json", MediaTypeUnknown},
		{"application/octet-stream", MediaTypeUnknown},
		{"", MediaTypeUnknown},
		{"   ", MediaTypeUnknown},
		{"video/mp4", MediaTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got := ClassifyContentType(tt.raw)
			if got != tt.want {
				t.Errorf("ClassifyContentType(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestMediaTypeClassString(t *testing.T) {
	tests := []struct {
		class MediaTypeClass
		want  string
	}{
		{MediaTypeHTML, "html"},
		{MediaTypeCSS, "css"},
		{MediaTypeJavaScript, "javascript"},
		{MediaTypeImage, "image"},
		{MediaTypeUnknown, "unknown"},
		{MediaTypeClass(99), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.class.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func BenchmarkValidateContentType(b *testing.B) {
	allowed := []string{"text/html", "text/css", "image/png", "image/jpeg", "image/gif"}
	raw := "text/html; charset=utf-8"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ValidateContentType(raw, allowed)
	}
}

func BenchmarkParseMediaType(b *testing.B) {
	raw := "text/html; charset=utf-8; boundary=abc"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = parseMediaType(raw)
	}
}

func BenchmarkClassifyContentType(b *testing.B) {
	raw := "text/html; charset=utf-8"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ClassifyContentType(raw)
	}
}
