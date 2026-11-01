package engine

import "testing"

func TestMixedContentURL(t *testing.T) {
	cases := []struct {
		name  string
		base  string
		ref   string
		want  string
		mixed bool
	}{
		{"absolute downgrade", "https://example.test/page", "http://cdn.test/a.js", "http://cdn.test/a.js", true},
		{"uppercase scheme", "https://example.test/page", "HTTP://cdn.test/a.js", "http://cdn.test/a.js", true},
		{"secure absolute", "https://example.test/page", "https://cdn.test/a.js", "", false},
		{"relative stays secure", "https://example.test/page", "img.png", "", false},
		{"scheme relative stays secure", "https://example.test/page", "//cdn.test/img.png", "", false},
		{"insecure document is not mixed", "http://example.test/page", "http://cdn.test/a.js", "", false},
		{"data uri is not mixed", "https://example.test/page", "data:image/png;base64,AA", "", false},
		{"malformed base", "://bad", "http://cdn.test/a.js", "", false},
		{"padded reference", "https://example.test/page", "  http://cdn.test/a.js  ", "http://cdn.test/a.js", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, mixed := mixedContentURL(tc.base, tc.ref)
			if got != tc.want || mixed != tc.mixed {
				t.Fatalf("mixedContentURL(%q, %q) = (%q, %v), want (%q, %v)",
					tc.base, tc.ref, got, mixed, tc.want, tc.mixed)
			}
		})
	}
}
