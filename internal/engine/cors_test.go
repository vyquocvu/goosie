package engine

import "testing"

func TestFontCORSAllowed(t *testing.T) {
	star := map[string]string{"Access-Control-Allow-Origin": "*"}
	cases := []struct {
		name    string
		base    string
		src     string
		headers map[string]string
		want    bool
	}{
		{"same origin, no headers", "https://example.com/p", "https://example.com/f.woff", nil, true},
		{"same origin ignores wrong ACAO", "https://example.com/p", "https://example.com/f.woff", map[string]string{"Access-Control-Allow-Origin": "https://other.example"}, true},
		{"cross origin, no headers", "https://example.com/p", "https://cdn.example/f.woff", nil, false},
		{"cross origin wildcard", "https://example.com/p", "https://cdn.example/f.woff", star, true},
		{"cross origin exact", "https://example.com/p", "https://cdn.example/f.woff", map[string]string{"Access-Control-Allow-Origin": "https://example.com"}, true},
		{"cross origin mismatch", "https://example.com/p", "https://cdn.example/f.woff", map[string]string{"Access-Control-Allow-Origin": "https://evil.example"}, false},
		{"cross origin trailing slash tolerated", "https://example.com/p", "https://cdn.example/f.woff", map[string]string{"Access-Control-Allow-Origin": "https://example.com/"}, true},
		{"http page, https font is cross origin", "http://example.com/p", "https://example.com/f.woff", nil, false},
		{"subdomain is cross origin", "https://example.com/p", "https://static.example.com/f.woff", nil, false},
		{"different ports differ", "https://example.com:8443/p", "https://example.com/f.woff", nil, false},
		{"file base exempt", "file:///Users/me/page.html", "https://cdn.example/f.woff", nil, true},
		{"malformed base refuses", "://bad", "https://cdn.example/f.woff", star, false},
		{"malformed target refuses", "https://example.com/p", "http://[::1", star, false},
	}
	for _, tc := range cases {
		if got := fontCORSAllowed(tc.base, tc.src, tc.headers); got != tc.want {
			t.Errorf("%s: fontCORSAllowed = %v, want %v", tc.name, got, tc.want)
		}
	}
}
