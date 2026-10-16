package main

import "testing"

func TestResolveHref(t *testing.T) {
	cases := []struct {
		name       string
		base, href string
		want       string
	}{
		{"root path", "https://example.com/wiki/Numbers", "/numbers", "https://example.com/numbers"},
		{"sibling path", "https://example.com/wiki/Numbers", "numbers", "https://example.com/wiki/numbers"},
		{"dotdot path", "https://example.com/wiki/sub/page", "../top", "https://example.com/wiki/top"},
		{"absolute passes through", "https://example.com/a", "https://other.org/b", "https://other.org/b"},
		{"scheme relative", "https://example.com/a", "//other.org/b", "https://other.org/b"},
		{"query only", "https://example.com/a", "?q=1", "https://example.com/a?q=1"},
		{"no base keeps href", "", "/numbers", "/numbers"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolveHref(c.base, c.href); got != c.want {
				t.Fatalf("resolveHref(%q, %q) = %q, want %q", c.base, c.href, got, c.want)
			}
		})
	}
}
