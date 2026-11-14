package net_test

import (
	"strings"
	"testing"

	transport "github.com/vyquocvu/goosie/internal/net"
)

// qc:BR-005
func TestNormalizeURLConvertsIDNToTheHostDNSCanResolve(t *testing.T) {
	unicodeURL, err := transport.NormalizeURL("https://пример.рф")
	if err != nil {
		t.Fatalf("NormalizeURL rejected a valid IDN host: %v", err)
	}
	if want := "https://xn--e1afmkfd.xn--p1ai"; unicodeURL != want {
		t.Fatalf("NormalizeURL(unicode) = %q, want %q", unicodeURL, want)
	}
	punycodeURL, err := transport.NormalizeURL("https://xn--e1afmkfd.xn--p1ai")
	if err != nil {
		t.Fatalf("NormalizeURL rejected the punycode form: %v", err)
	}
	if unicodeURL != punycodeURL {
		t.Fatalf("the same host normalizes to two different URLs: %q vs %q", unicodeURL, punycodeURL)
	}

	cases := []struct {
		name, input, want string
	}{
		{"uppercase IDN folds to the same host", "https://ПРИМЕР.РФ", "https://xn--e1afmkfd.xn--p1ai"},
		{"scheme-less IDN host navigates", "пример.рф", "https://xn--e1afmkfd.xn--p1ai"},
		{"port survives the conversion", "пример.рф:8443/page", "https://xn--e1afmkfd.xn--p1ai:8443/page"},
		{"unicode path is percent-encoded", "https://пример.рф/путь", "https://xn--e1afmkfd.xn--p1ai/%D0%BF%D1%83%D1%82%D1%8C"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := transport.NormalizeURL(tc.input)
			if err != nil {
				t.Fatalf("NormalizeURL(%q): %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("NormalizeURL(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// qc:BR-005
func TestNormalizeURLRejectsAnInvalidIDN(t *testing.T) {
	oversized := "https://" + strings.Repeat("ü", 60) + ".example"
	if got, err := transport.NormalizeURL(oversized); err == nil {
		t.Fatalf("NormalizeURL accepted an over-long IDN label as %q", got)
	}
}
