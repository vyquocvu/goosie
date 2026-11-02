package net_test

import (
	"strings"
	"testing"

	transport "github.com/vyquocvu/goosie/internal/net"
)

// TestNormalizeURLConvertsIDNToTheHostDNSCanResolve pins the one place the
// address bar, the request, and DNS have to agree. A Unicode name is the name
// the user typed; punycode is the name the resolver can ask for. Normalizing to
// anything else - or rejecting the name after the address bar accepted it -
// means the page the browser claims to show and the host it actually dialed are
// two different things.
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

// TestNormalizeURLRejectsAnInvalidIDN is the other half of consistency: a name
// the conversion cannot make dialable has to be an error here, before a request
// is built, rather than a name passed to a resolver that will mangle or refuse
// it later.
// qc:BR-005
func TestNormalizeURLRejectsAnInvalidIDN(t *testing.T) {
	// Sixty U+00FC code points encode to a punycode label longer than DNS's 63
	// byte limit. The ASCII spelling is what has to be measured, so the raw
	// byte length of the input says nothing about whether it is valid.
	oversized := "https://" + strings.Repeat("ü", 60) + ".example"
	if got, err := transport.NormalizeURL(oversized); err == nil {
		t.Fatalf("NormalizeURL accepted an over-long IDN label as %q", got)
	}
}
