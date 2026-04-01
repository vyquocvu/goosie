package main

import (
	"strings"
	"testing"

	ghtml "golang.org/x/net/html"
)

func TestExtractInlineScripts(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		expected []string
	}{
		{
			name: "single inline script",
			html: `<html><body><script>console.log("hello");</script></body></html>`,
			expected: []string{`console.log("hello");`},
		},
		{
			name: "multiple inline scripts",
			html: `<html><body>
				<script>var a = 1;</script>
				<script>var b = 2;</script>
			</body></html>`,
			expected: []string{"var a = 1;", "var b = 2;"},
		},
		{
			name: "external script is ignored",
			html: `<html><body><script src="app.js"></script></body></html>`,
			expected: nil,
		},
		{
			name: "mixed inline and external",
			html: `<html><body>
				<script src="external.js"></script>
				<script>var x = 42;</script>
			</body></html>`,
			expected: []string{"var x = 42;"},
		},
		{
			name: "no scripts",
			html: `<html><body><p>Hello</p></body></html>`,
			expected: nil,
		},
		{
			name: "empty script tag",
			html: `<html><body><script></script></body></html>`,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractInlineScripts(tt.html)
			if len(got) != len(tt.expected) {
				t.Errorf("expected %d scripts, got %d: %v", len(tt.expected), len(got), got)
				return
			}
			for i, script := range got {
				if !strings.Contains(script, strings.TrimSpace(tt.expected[i])) {
					t.Errorf("script[%d]: expected to contain %q, got %q", i, tt.expected[i], script)
				}
			}
		})
	}
}

func TestExtractExternalScriptSrcs(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		expected []string
	}{
		{
			name:     "single external script",
			html:     `<html><body><script src="app.js"></script></body></html>`,
			expected: []string{"app.js"},
		},
		{
			name: "multiple external scripts",
			html: `<html><head>
				<script src="lib.js"></script>
				<script src="app.js"></script>
			</head></html>`,
			expected: []string{"lib.js", "app.js"},
		},
		{
			name:     "inline script ignored",
			html:     `<html><body><script>var x = 1;</script></body></html>`,
			expected: nil,
		},
		{
			name:     "no scripts",
			html:     `<html><body><p>Hello</p></body></html>`,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := parseHTMLForTest(tt.html)
			if err != nil {
				t.Fatalf("failed to parse HTML: %v", err)
			}
			got := extractExternalScriptSrcs(doc)
			if len(got) != len(tt.expected) {
				t.Errorf("expected %d srcs, got %d: %v", len(tt.expected), len(got), got)
				return
			}
			for i, src := range got {
				if src != tt.expected[i] {
					t.Errorf("src[%d]: expected %q, got %q", i, tt.expected[i], src)
				}
			}
		})
	}
}

// parseHTMLForTest is a helper that calls ghtml.Parse for test use.
func parseHTMLForTest(htmlContent string) (*ghtml.Node, error) {
	return ghtml.Parse(strings.NewReader(htmlContent))
}
