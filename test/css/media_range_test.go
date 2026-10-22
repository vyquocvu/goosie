package css_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

func TestMediaRuleSplitting(t *testing.T) {
	cond := "(width <= 800px) or (width >= 1000px)"
	parts := css.SplitOutsideParens(cond, " or ")

	if len(parts) != 2 {
		t.Errorf("Expected 2 parts for %q, got %d: %v", cond, len(parts), parts)
	}

	cond2 := "(400px <= width < 1000px) and (orientation)"
	parts2 := css.SplitOutsideParens(cond2, " and ")

	if len(parts2) != 2 {
		t.Errorf("Expected 2 parts for %q, got %d: %v", cond2, len(parts2), parts2)
	}
}

func TestCSSMediaQueryWithRangeSyntax(t *testing.T) {
	cssText := `
@media (width <= 800px) {
    .mobile-only { display: block; }
}
@media (width >= 1000px) {
    .desktop-only { display: flex; }
}
`

	sheet := css.Parse(cssText)
	if sheet == nil {
		t.Fatalf("Failed to parse CSS with @media range syntax")
	}

	// At default viewport (1280px), desktop-only should match and mobile-only should not
	foundDesktop := false
	foundMobile := false
	for _, rule := range sheet.Rules {
		for _, sel := range rule.SelectorStrs {
			if sel == ".desktop-only" {
				foundDesktop = true
			}
			if sel == ".mobile-only" {
				foundMobile = true
			}
		}
	}
	if !foundDesktop {
		t.Errorf("Expected .desktop-only rule to match at 1280px")
	}
	if foundMobile {
		t.Errorf("Expected .mobile-only rule to NOT match at 1280px")
	}
}

func TestCSSMediaQueryRangeEvaluation(t *testing.T) {
	cssText := `
@media (width <= 800px) {
    .mobile { display: block; }
}
@media (400px <= width < 1000px) {
    .tablet { display: block; }
}
@media (width >= 1000px) {
    .desktop { display: block; }
}
`

	// At 500px: mobile (<=800) matches, tablet (400<=w<1000) matches, desktop (>=1000) does not
	s500 := css.ParseForViewport(cssText, 500)
	classes500 := ruleClasses(s500)
	if !classes500[".mobile"] || !classes500[".tablet"] || classes500[".desktop"] {
		t.Errorf("At 500px: want mobile+tablet, got %v", classes500)
	}

	// At 900px: tablet (400<=w<1000) matches, mobile and desktop do not
	s900 := css.ParseForViewport(cssText, 900)
	classes900 := ruleClasses(s900)
	if classes900[".mobile"] || !classes900[".tablet"] || classes900[".desktop"] {
		t.Errorf("At 900px: want tablet only, got %v", classes900)
	}

	// At 1200px: desktop (>=1000) matches, mobile and tablet do not
	s1200 := css.ParseForViewport(cssText, 1200)
	classes1200 := ruleClasses(s1200)
	if classes1200[".mobile"] || classes1200[".tablet"] || !classes1200[".desktop"] {
		t.Errorf("At 1200px: want desktop only, got %v", classes1200)
	}
}

func ruleClasses(sheet *css.Stylesheet) map[string]bool {
	m := make(map[string]bool)
	if sheet == nil {
		return m
	}
	for _, rule := range sheet.Rules {
		for _, sel := range rule.SelectorStrs {
			m[sel] = true
		}
	}
	return m
}

func TestMediaConditionParsing(t *testing.T) {
	testCases := []string{
		"(width <= 800px)",
		"(width >= 400px)",
		"(width < 600px)",
		"(width > 300px)",
		"(400px <= width < 1000px)",
		"(300px < width < 800px)",
		"(min-width: 400px)",
		"(max-width: 800px)",
	}

	for _, tc := range testCases {
		t.Run(tc, func(t *testing.T) {
			cssText := tc + " { body { color: red; } }"
			sheet := css.Parse(cssText)
			if sheet == nil {
				t.Errorf("Failed to parse condition %q", tc)
			}
		})
	}
}
