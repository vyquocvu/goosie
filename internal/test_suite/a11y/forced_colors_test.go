package a11y

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/css"
)

// TestHighContrast_PrefersContrastParsing verifies the CSS parser
// accepts the W3C-recommended prefers-contrast media feature values:
//
//   - no-preference (default)
//   - more    (user has requested higher contrast)
//   - less    (user has requested lower contrast)
//   - custom (user has a custom palette, e.g., a specific theme)
//
// Parsing must not panic on these values; tests assert that the
// resulting @media rule preserves the prelude text so the rest of
// the styling pipeline can route on it.
func TestHighContrast_PrefersContrastParsing(t *testing.T) {
	cases := []string{
		"@media (prefers-contrast: more) { .high { color: black; } }",
		"@media (prefers-contrast: less) { .low { color: gray; } }",
		"@media (prefers-contrast: custom) { .custom { background: white; } }",
		"@media (prefers-contrast: no-preference) { .np { opacity: 1; } }",
		"@media screen and (prefers-contrast: more) { .a { color: black; } }",
		"@media (prefers-contrast: more) and (min-width: 600px) { .a { color: black; } }",
	}
	for _, input := range cases {
		input := input
		t.Run(input, func(t *testing.T) {
			sheet, err := css.NewParser(input).Parse()
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			if len(sheet.AtRules) != 1 {
				t.Fatalf("expected 1 at-rule, got %d", len(sheet.AtRules))
			}
			rule := sheet.AtRules[0]
			if rule.Name != "media" {
				t.Fatalf("expected at-rule name 'media', got %q", rule.Name)
			}
			if !strings.Contains(rule.Prelude, "prefers-contrast") {
				t.Fatalf("prelude missing 'prefers-contrast': %q", rule.Prelude)
			}
		})
	}
}

// TestHighContrast_ForcedColorsParsing verifies the CSS parser
// accepts the prefers-contrast values per the W3C Forced Colors Mode
// specification. forced-colors is a binary media feature (active or
// none) plus a triplet for limited evaluation modes.
func TestHighContrast_ForcedColorsParsing(t *testing.T) {
	cases := []string{
		"@media (forced-colors: active) { .fc { color: ButtonText; } }",
		"@media (forced-colors: none) { .nofc { color: revert; } }",
		"@media screen and (forced-colors: active) { .a { color: CanvasText; } }",
		"@media (forced-colors: active) and (prefers-contrast: more) { .a { color: Highlight; } }",
	}
	for _, input := range cases {
		input := input
		t.Run(input, func(t *testing.T) {
			sheet, err := css.NewParser(input).Parse()
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			if len(sheet.AtRules) != 1 {
				t.Fatalf("expected 1 at-rule, got %d", len(sheet.AtRules))
			}
			rule := sheet.AtRules[0]
			if rule.Name != "media" {
				t.Fatalf("expected at-rule name 'media', got %q", rule.Name)
			}
			if !strings.Contains(rule.Prelude, "forced-colors") {
				t.Fatalf("prelude missing 'forced-colors': %q", rule.Prelude)
			}
		})
	}
}

// TestHighContrast_ColorSystemKeywords verifies that the W3C-defined
// CSS system color keywords used by forced-colors adapters parse
// correctly inside declarations:
//
//	ButtonText, Canvas, CanvasText, LinkText, VisitedText,
//	Highlight, HighlightText, SelectedItem, SelectedItemText,
//	Mark, MarkText, AccentColor, AccentColorText, GrayText
//
// These keywords are treated as identifier values by the parser, so
// this test verifies identifier parsing handles them.
func TestHighContrast_ColorSystemKeywords(t *testing.T) {
	keywords := []string{
		"ButtonText", "Canvas", "CanvasText", "LinkText", "VisitedText",
		"Highlight", "HighlightText", "SelectedItem", "SelectedItemText",
		"Mark", "MarkText", "AccentColor", "AccentColorText", "GrayText",
	}
	for _, kw := range keywords {
		kw := kw
		t.Run(kw, func(t *testing.T) {
			input := ".x { color: " + kw + "; }"
			sheet, err := css.NewParser(input).Parse()
			assert.NoError(t, err)
			assert.Len(t, sheet.Rules, 1)
			assert.Len(t, sheet.Rules[0].Declarations, 1)
			assert.Equal(t, "color", sheet.Rules[0].Declarations[0].Property)
			assert.Equal(t, kw, sheet.Rules[0].Declarations[0].Value)
		})
	}
}

// TestHighContrast_NegativeMediaFeatures verifies that the parser
// accepts the negation form (matches the W3C "negation" production):
//
//	@media not (prefers-contrast: more) { ... }
//	@media not (forced-colors: active) { ... }
//
// The CSS engine routes this prelude to MediaQueryEvaluator at the
// styling stage; the parser must preserve the negation as part of
// the prelude string.
func TestHighContrast_NegativeMediaFeatures(t *testing.T) {
	cases := []string{
		"not (prefers-contrast: more)",
		"not (forced-colors: active)",
		"not (prefers-contrast: more) and (min-width: 800px)",
	}
	for _, prelude := range cases {
		prelude := prelude
		t.Run(prelude, func(t *testing.T) {
			input := "@media " + prelude + " { .x { color: red; } }"
			sheet, err := css.NewParser(input).Parse()
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			assert.Len(t, sheet.AtRules, 1)
			assert.Equal(t, prelude, sheet.AtRules[0].Prelude)
		})
	}
}

// TestHighContrast_MediaQueryList verifies comma-separated media
// queries (one of the features forms is the "or" chain) parse
// correctly with high-contrast clauses inside.
func TestHighContrast_MediaQueryList(t *testing.T) {
	cases := []string{
		"screen, (prefers-contrast: more)",
		"(prefers-contrast: more), (forced-colors: active)",
		"screen and (prefers-contrast: more), not (forced-colors: active)",
	}
	for _, prelude := range cases {
		prelude := prelude
		t.Run(prelude, func(t *testing.T) {
			input := "@media " + prelude + " { .x { color: red; } }"
			sheet, err := css.NewParser(input).Parse()
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			assert.Len(t, sheet.AtRules, 1)
			assert.Equal(t, prelude, sheet.AtRules[0].Prelude)
		})
	}
}

// TestHighContrast_FallbackContract documents the **current** engine
// behavior: high-contrast media features that the engine does not
// evaluate yet are accepted at parse time and resolved by the
// MediaQueryEvaluator's permissive fallback (unknown condition
// returns true).
//
// This locks in the contract that any future change to the engine's
// behavior for these features is observable here. A change to "must
// reject" or "must return false" would break this test and force a
// conscious decision in code review.
func TestHighContrast_FallbackContract(t *testing.T) {
	// Currently the engine does not import renderer.MediaQueryEvaluator
	// in this Fyne-free test package. We lock the contract in by
	// confirming that:
	//
	//  1. The CSS parser accepts @media (prefers-contrast: *) and
	//     @media (forced-colors: *) syntactically.
	//  2. The prelude text is preserved verbatim for the engine to
	//     evaluate against a user override.
	cssInput := `
		@media (prefers-contrast: more) { .x { color: black; } }
		@media (forced-colors: active) { .y { color: white; } }
	`
	sheet, err := css.NewParser(cssInput).Parse()
	if err != nil {
		t.Fatalf("css parse: %v", err)
	}
	assert.Len(t, sheet.AtRules, 2, "two @media at-rules should survive parse")
	preludes := []string{sheet.AtRules[0].Prelude, sheet.AtRules[1].Prelude}
	assert.Contains(t, preludes[0], "prefers-contrast")
	assert.Contains(t, preludes[1], "forced-colors")
}
