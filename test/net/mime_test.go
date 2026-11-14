package net_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/net"
)

func TestStyleSheetRefused(t *testing.T) {
	cases := []struct {
		contentType string
		want        bool
	}{
		{"text/css", false},
		{"TEXT/CSS; charset=utf-8", false},
		{"", false},
		{"text/plain", false},
		{"application/octet-stream", false},
		{"text/html; charset=UTF-8", true},
		{"application/xhtml+xml", true},
		{"application/xml", true},
		{"text/xml", true},
		{"image/svg+xml", true},
		{"font/woff2", true},
		{"application/javascript", true},
		{"text/javascript", true},
		{"application/json", true},
	}
	for _, c := range cases {
		if got := net.StyleSheetRefused(c.contentType); got != c.want {
			t.Errorf("StyleSheetRefused(%q) = %v, want %v", c.contentType, got, c.want)
		}
	}
}
