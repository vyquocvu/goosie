package main

import (
	"strings"
	"testing"
)

// qc:BR-003
func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "google.com domain",
			input:    "google.com",
			expected: "https://google.com",
		},
		{
			name:     "example.com domain",
			input:    "example.com",
			expected: "https://example.com",
		},
		{
			name:     "IP address",
			input:    "192.168.1.1",
			expected: "https://192.168.1.1",
		},
		{
			name:     "localhost with port",
			input:    "localhost:8080",
			expected: "https://localhost:8080",
		},
		{
			name:     "domain with hyphens - single label, search",
			input:    "my-site",
			expected: "https://www.google.com/search?q=my-site",
		},
		{
			name:     "multiple hyphens, no dot - search",
			input:    "test-domain-name",
			expected: "https://www.google.com/search?q=test-domain-name",
		},
		{
			name:     "github.io domain",
			input:    "github.io",
			expected: "https://github.io",
		},
		{
			name:     "example.org domain",
			input:    "example.org",
			expected: "https://example.org",
		},
		{
			name:     "search query with spaces",
			input:    "how to make coffee",
			expected: "https://www.google.com/search?q=how+to+make+coffee",
		},
		{
			name:     "weather search",
			input:    "weather today",
			expected: "https://www.google.com/search?q=weather+today",
		},
		{
			name:     "hello world search",
			input:    "hello world",
			expected: "https://www.google.com/search?q=hello+world",
		},
		{
			name:     "what is goosie browser",
			input:    "what is goosie browser",
			expected: "https://www.google.com/search?q=what+is+goosie+browser",
		},
		{
			name:     "HTTPS URL unchanged",
			input:    "https://github.com",
			expected: "https://github.com",
		},
		{
			name:     "HTTP URL unchanged",
			input:    "http://example.com",
			expected: "http://example.com",
		},
		{
			name:     "file URL unchanged",
			input:    "file:///path/to/file.html",
			expected: "file:///path/to/file.html",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "search query with tab character",
			input:    "search\tquery",
			expected: "https://www.google.com/search?q=search%09query",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeURL(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeURL(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizeAddressUsesProfileSearchEngine(t *testing.T) {
	got := normalizeAddress("tìm kiếm goosie", "https://search.example/find?term={query}")
	if got != "https://search.example/find?term=t%C3%ACm+ki%E1%BA%BFm+goosie" {
		t.Fatalf("normalizeAddress() = %q", got)
	}
	if got := normalizeAddress("https://example.com/path?q=x", "https://search.example/?q={query}"); got != "https://example.com/path?q=x" {
		t.Fatalf("explicit URL changed by search preference: %q", got)
	}
}

// TestNormalizeURLSearchQueryToDomainEdgeCases tests edge cases that could be
// interpreted as either domains or search queries.
func TestNormalizeURLSearchQueryToDomainEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		desc     string
	}{
		{
			name:     "single word without dot - search query",
			input:    "golang",
			expected: "https://www.google.com/search?q=golang",
			desc:     "Single labels are not dialable hostnames; browsers search them",
		},
		{
			name:     "word with numbers, no dot - search query",
			input:    "example123",
			expected: "https://www.google.com/search?q=example123",
			desc:     "Words without a dot are not dialable hostnames",
		},
		{
			name:     "two words joined by hyphen - search query",
			input:    "hello-world",
			expected: "https://www.google.com/search?q=hello-world",
			desc:     "Hyphenated single labels are not dialable hostnames",
		},
		{
			name:     "three words separated by space - search query",
			input:    "hello world page",
			expected: "https://www.google.com/search?q=hello+world+page",
			desc:     "Multiple words separated by spaces are search queries",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeURL(tt.input)
			if result != tt.expected {
				t.Errorf("%s\nInput: %q\nResult:   %q\nExpected: %q", tt.desc, tt.input, result, tt.expected)
			}
		})
	}
}

// TestNormalizeURLSpecialCharacters tests URLs with special characters.
func TestNormalizeURLSpecialCharacters(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "search with special chars - plus",
			input:    "HTML & CSS",
			expected: "https://www.google.com/search?q=HTML+%26+CSS",
		},
		{
			name:     "search with special chars - question mark",
			input:    "what is golang?",
			expected: "https://www.google.com/search?q=what+is+golang%3F",
		},
		{
			name:     "search with special chars - ampersand",
			input:    "Goosie Browser development",
			expected: "https://www.google.com/search?q=Goosie+Browser+development",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeURL(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeURL(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestNormalizeURLContainsDot checks if input contains a dot for domain detection.
func TestNormalizeURLContainsDot(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "URL with path - not a search query",
			input:    "example.com/path/to/page",
			expected: "https://example.com/path/to/page",
		},
		{
			name:     "domain with www",
			input:    "www.example.com",
			expected: "https://www.example.com",
		},
		{
			name:     "international domain",
			input:    "пример.рф",
			expected: "https://пример.рф",
		},
		{
			name:     "search with period-like punctuation",
			input:    "u.s.a history",
			expected: "https://www.google.com/search?q=u.s.a+history",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeURL(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeURL(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestNormalizeURLEdgeCases tests additional edge cases.
func TestNormalizeURLEdgeCases(t *testing.T) {
	// Test inputs that contain both dots AND spaces (should be search queries)
	edgeCaseTests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "search query with domain-like structure",
			input:    "check out example.com today",
			expected: "https://www.google.com/search?q=check+out+example.com+today",
		},
		{
			name:     "search with URL pattern",
			input:    "I visited https://github.com last week",
			expected: "https://www.google.com/search?q=I+visited+https%3A%2F%2Fgithub.com+last+week",
		},
	}

	for _, tt := range edgeCaseTests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeURL(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeURL(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestNormalizeAddressAmbiguousInputs pins the host-vs-search decisions for
// scheme-less address-bar values: only unambiguously host-shaped input
// (dotted names, IPs, localhost, bracketed IPv6, optional valid ports)
// navigates; everything else searches through the profile engine.
// qc:BR-003
func TestNormalizeAddressAmbiguousInputs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"single label word searches", "cats", "https://www.google.com/search?q=cats"},
		{"dotted host navigates", "example.com", "https://example.com"},
		{"dotted host with path navigates", "a.b.co/path", "https://a.b.co/path"},
		{"ipv4 navigates", "192.168.1.1", "https://192.168.1.1"},
		{"ipv4 with port and path navigates", "127.0.0.1:8000/metrics", "https://127.0.0.1:8000/metrics"},
		{"localhost navigates", "localhost", "https://localhost"},
		{"localhost with port navigates", "localhost:8080", "https://localhost:8080"},
		{"localhost is case insensitive", "LocalHost:3000", "https://LocalHost:3000"},
		{"bracketed ipv6 with port navigates", "[::1]:8080", "https://[::1]:8080"},
		{"bare ipv6 without brackets searches", "::1", "https://www.google.com/search?q=%3A%3A1"},
		// Decision: any whitespace makes the value a search phrase, even when
		// the head looks like a host:port (matches mainstream browsers).
		{"host with space in path searches", "example.com:8080/a b", "https://www.google.com/search?q=example.com%3A8080%2Fa+b"},
		{"host with percent-encoded space in path navigates", "example.com:8080/a%20b", "https://example.com:8080/a%20b"},
		{"non numeric port searches", "example.com:notaport/x", "https://www.google.com/search?q=example.com%3Anotaport%2Fx"},
		{"out of range port searches", "example.com:99999", "https://www.google.com/search?q=example.com%3A99999"},
		{"partial ip searches", "1.2.3", "https://www.google.com/search?q=1.2.3"},
		{"path without host searches", "/etc/hosts", "https://www.google.com/search?q=%2Fetc%2Fhosts"},
		{"mailto searches", "mailto:someone@example.com", "https://www.google.com/search?q=mailto%3Asomeone%40example.com"},
		{"underscore in host searches", "my_domain.com", "https://www.google.com/search?q=my_domain.com"},
		{"trailing dot searches", "golang.", "https://www.google.com/search?q=golang."},
		{"leading dot searches", ".golang", "https://www.google.com/search?q=.golang"},
		{"empty port searches", "example.com:", "https://www.google.com/search?q=example.com%3A"},
		{"unicode single word searches", "日本語", "https://www.google.com/search?q=%E6%97%A5%E6%9C%AC%E8%AA%9E"},
		{"ampersand in single token searches", "a&b", "https://www.google.com/search?q=a%26b"},
		{"question mark in single label searches", "go?lang", "https://www.google.com/search?q=go%3Flang"},
		{"plus in single label searches", "c++", "https://www.google.com/search?q=c%2B%2B"},
		{"idn host navigates", "пример.рф", "https://пример.рф"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeURL(tt.input); got != tt.expected {
				t.Errorf("normalizeURL(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// TestNormalizeAddressSingleWordUsesProfileTemplate checks that a single-word
// query is searched through the profile's engine, not just the default one.
func TestNormalizeAddressSingleWordUsesProfileTemplate(t *testing.T) {
	template := "https://search.example/find?term={query}"
	if got := normalizeAddress("golang", template); got != "https://search.example/find?term=golang" {
		t.Errorf("profile template not used for single word: %q", got)
	}
	// A malformed template must still produce a safe default-engine search.
	if got := normalizeAddress("cats", "not a template"); got != "https://www.google.com/search?q=cats" {
		t.Errorf("invalid template fallback = %q, want default-engine search", got)
	}
	// Host-shaped input must never be routed to the search engine.
	if got := normalizeAddress("example.com:8080/x", template); got != "https://example.com:8080/x" {
		t.Errorf("host-shaped input sent to search engine: %q", got)
	}
}

// BenchmarkNormalizeURL benchmarks the normalizeURL function with various inputs.
func BenchmarkNormalizeURL(b *testing.B) {
	testCases := []string{
		"google.com",
		"example.com",
		"192.168.1.1",
		"https://github.com",
		"how to make coffee",
		"weather today",
		"",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, tc := range testCases {
			_ = normalizeURL(tc)
		}
	}
}

// TestNormalizeURLPerformance checks performance on long search queries.
func TestNormalizeURLPerformance(t *testing.T) {
	longQuery := strings.Repeat("test ", 100) + "final"
	result := normalizeURL(longQuery)
	expectedPrefix := "https://www.google.com/search?q="
	if !strings.HasPrefix(result, expectedPrefix) {
		t.Errorf("Long query was not converted to search URL")
	}
	if len(result) > len(longQuery)+50 {
		t.Logf("Long query expansion from %d to %d characters", len(longQuery), len(result))
	}
}

func TestSmartOmniboxDisambiguation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "trimmed URL",
			input:    "   https://golang.org   ",
			expected: "https://golang.org",
		},
		{
			name:     "trimmed domain",
			input:    "  google.com  ",
			expected: "https://google.com",
		},
		{
			name:     "trimmed search query",
			input:    "  weather in hanoi  ",
			expected: "https://www.google.com/search?q=weather+in+hanoi",
		},
		{
			name:     "case-insensitive HTTPS scheme",
			input:    "HTTPS://EXAMPLE.COM/PATH",
			expected: "https://EXAMPLE.COM/PATH",
		},
		{
			name:     "case-insensitive HTTP scheme",
			input:    "Http://MyServer.local",
			expected: "http://MyServer.local",
		},
		{
			name:     "case-insensitive file scheme",
			input:    "FILE:///etc/hosts",
			expected: "file:///etc/hosts",
		},
		{
			name:     "vietnamese unicode search query",
			input:    "thời tiết hà nội",
			expected: "https://www.google.com/search?q=th%E1%BB%9Di+ti%E1%BA%BFt+h%C3%A0+n%E1%BB%99i",
		},
		{
			name:     "single word with question mark (search)",
			input:    "golang?",
			expected: "https://www.google.com/search?q=golang%3F",
		},
		{
			name:     "programming symbol search (c++)",
			input:    "c++",
			expected: "https://www.google.com/search?q=c%2B%2B",
		},
		{
			name:     "math query (100%)",
			input:    "100%",
			expected: "https://www.google.com/search?q=100%25",
		},
		{
			name:     "equation query (5+5)",
			input:    "5+5",
			expected: "https://www.google.com/search?q=5%2B5",
		},
		{
			name:     "domain with path and query parameters",
			input:    "example.com/search?q=test&lang=en",
			expected: "https://example.com/search?q=test&lang=en",
		},
		{
			name:     "newline in search input",
			input:    "search\nquery",
			expected: "https://www.google.com/search?q=search%0Aquery",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeURL(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeURL(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
