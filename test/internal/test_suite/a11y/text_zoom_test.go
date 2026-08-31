package a11y

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/css"
)

// TestTextZoom_RelativeSizeKeywords verifies that the CSS parser
// accepts the user-zoom relative size keywords defined in CSS Values
// Level 3 — `larger` and `smaller` — which the browser uses to
// implement the user-text-zoom shortcut (Ctrl-+ / Ctrl--).
//
// The contract under test is: these keywords are accepted as the
// value of `font-size`, so the engine can route them through the
// relative-size-step table at the styling stage.
func TestTextZoom_RelativeSizeKeywords(t *testing.T) {
	cases := map[string]string{
		"larger":  ".x { font-size: larger; }",
		"smaller": ".x { font-size: smaller; }",
	}
	for kw, input := range cases {
		kw, input := kw, input
		t.Run(kw, func(t *testing.T) {
			sheet, err := css.NewParser(input).Parse()
			assert.NoError(t, err)
			assert.Len(t, sheet.Rules, 1)
			assert.Len(t, sheet.Rules[0].Declarations, 1)
			d := sheet.Rules[0].Declarations[0]
			assert.Equal(t, "font-size", d.Property)
			assert.Equal(t, kw, d.Value, "the relative size keyword must survive parsing intact")
		})
	}
}

// TestTextZoom_RelativeLengths verifies that the CSS parser accepts
// em / rem relative length units on font-size, which is the
// foundation of user-zoom and accessibility-respecting layouts.
func TestTextZoom_RelativeLengths(t *testing.T) {
	cases := []string{
		"1rem", "2rem", "0.5rem",
		"1em", "1.5em", "2em",
	}
	for _, v := range cases {
		v := v
		t.Run(v, func(t *testing.T) {
			input := ".x { font-size: " + v + "; }"
			sheet, err := css.NewParser(input).Parse()
			assert.NoError(t, err)
			assert.Len(t, sheet.Rules[0].Declarations, 1)
			assert.Equal(t, v, sheet.Rules[0].Declarations[0].Value)
		})
	}
}

// TestTextZoom_PercentSizes verifies percent-based font sizes — the
// engine must accept these and treat them as relative to the parent's
// computed font-size at the styling stage.
func TestTextZoom_PercentSizes(t *testing.T) {
	for _, v := range []string{"100%", "125%", "150%", "200%", "75%"} {
		v := v
		t.Run(v, func(t *testing.T) {
			input := ".x { font-size: " + v + "; }"
			sheet, err := css.NewParser(input).Parse()
			assert.NoError(t, err)
			assert.Equal(t, v, sheet.Rules[0].Declarations[0].Value)
		})
	}
}

// TestTextZoom_ViewportUnits verifies viewport-relative units (vw,
// vh, vmin, vmax) on font-size. These are essential for responsive
// zoom layouts where the user's browser-zoom and the page-zoom must
// combine predictably.
func TestTextZoom_ViewportUnits(t *testing.T) {
	for _, v := range []string{"1vw", "2vh", "1vmin", "2vmax"} {
		v := v
		t.Run(v, func(t *testing.T) {
			input := ".x { font-size: " + v + "; }"
			sheet, err := css.NewParser(input).Parse()
			assert.NoError(t, err)
			assert.Equal(t, v, sheet.Rules[0].Declarations[0].Value)
		})
	}
}

// TestTextZoom_AbsoluteKeywords verifies the seven CSS absolute-size
// keywords. These map to fixed pixel sizes and are unaffected by
// user-text-zoom in most engines, but must parse correctly because
// they appear frequently in author stylesheets.
func TestTextZoom_AbsoluteKeywords(t *testing.T) {
	keywords := []string{
		"xx-small", "x-small", "small", "medium",
		"large", "x-large", "xx-large",
	}
	for _, kw := range keywords {
		kw := kw
		t.Run(kw, func(t *testing.T) {
			input := ".x { font-size: " + kw + "; }"
			sheet, err := css.NewParser(input).Parse()
			assert.NoError(t, err)
			assert.Equal(t, kw, sheet.Rules[0].Declarations[0].Value)
		})
	}
}

// TestTextZoom_FontSizeAdjust verifies font-size-adjust is accepted
// at the parser level. This property is critical for accessibility
// because users with low-vision preferences often select fallback
// fonts; font-size-adjust ensures x-height uniformity across the
// font fallback chain.
func TestTextZoom_FontSizeAdjust(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"number", ".x { font-size-adjust: 0.5; }"},
		{"none", ".x { font-size-adjust: none; }"},
		{"from-parent", ".x { font-size-adjust: from-font; }"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			sheet, err := css.NewParser(c.value).Parse()
			assert.NoError(t, err)
			assert.Equal(t, "font-size-adjust", sheet.Rules[0].Declarations[0].Property)
		})
	}
}

// TestTextZoom_MinMaxConstraintProperties verifies the user-agent
// minimum / maximum font-size guard properties. The browser uses
// these to enforce that even the smallest user setting keeps text
// legible for low-vision users.
func TestTextZoom_MinMaxConstraintProperties(t *testing.T) {
	for _, decl := range []string{
		"min-font-size: 12px",
		"max-font-size: 96px",
	} {
		decl := decl
		t.Run(decl, func(t *testing.T) {
			input := ".x { " + decl + "; }"
			sheet, err := css.NewParser(input).Parse()
			assert.NoError(t, err)
			declarations := sheet.Rules[0].Declarations
			assert.Len(t, declarations, 1)
		})
	}
}

// TestTextZoom_CompoundZoomStylesheet simulates an end-to-end
// stylesheet a low-vision user (or assistive technology) might
// inject at runtime to enlarge all text. The point of this test is
// regression coverage for the parser under realistic zoom-related
// declarations.
func TestTextZoom_CompoundZoomStylesheet(t *testing.T) {
	input := `
		html { font-size: 18px; }
		body { font-size: 1rem; line-height: 1.6; }
		.larger { font-size: larger; }
		.percentage { font-size: 125%; }
		.viewport { font-size: 2vw; }
		.adjust  { font-size-adjust: from-font; }
		@media (prefers-contrast: more) {
			.high-contrast-text { font-size: larger; font-weight: 700; }
		}
		@media (prefers-reduced-motion: reduce) {
			* { transition: none !important; }
		}
	`
	sheet, err := css.NewParser(input).Parse()
	assert.NoError(t, err)
	// 6 regular rules + 2 @media at-rules
	assert.Len(t, sheet.Rules, 6)
	assert.Len(t, sheet.AtRules, 2)

	// The prefers-contrast media query is preserved verbatim.
	hcRule := sheet.AtRules[0]
	assert.True(t, strings.Contains(hcRule.Prelude, "prefers-contrast"))

	// The prefers-reduced-motion media query is preserved verbatim.
	prmRule := sheet.AtRules[1]
	assert.True(t, strings.Contains(prmRule.Prelude, "prefers-reduced-motion"))
}

// TestTextZoom_PrefersReducedMotion verifies the parser also
// accepts the prefers-reduced-motion media query. Although this is
// motion rather than visual zoom, it is part of the same
// accessibility media-feature family and routes through the same
// evaluator surface.
func TestTextZoom_PrefersReducedMotion(t *testing.T) {
	cases := []string{
		"@media (prefers-reduced-motion: reduce) { .x { transition: none; } }",
		"@media (prefers-reduced-motion: no-preference) { .x { animation: spin 1s; } }",
	}
	for _, input := range cases {
		input := input
		t.Run(input, func(t *testing.T) {
			sheet, err := css.NewParser(input).Parse()
			assert.NoError(t, err)
			assert.Len(t, sheet.AtRules, 1)
			assert.True(t, strings.Contains(sheet.AtRules[0].Prelude, "prefers-reduced-motion"))
		})
	}
}
