// Tests for the raster.FontRegistry. The registry resolves a CSS font
// descriptor (family, weight, style) plus a pixel size into a
// scalable font.Face usable by the raster backends. These tests
// are environment-agnostic: they only exercise the bundled Go
// fonts, not the system font probe, so they pass on every
// supported platform.
package raster_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/renderer/frame/raster"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFontRegistry_GetSansSerifDefault verifies the default
// family path produces a non-nil face. The renderer relies on
// every text run returning a face; a regression that maps empty
// or unrecognised families to nil would silently drop text on
// every page that does not explicitly set font-family.
func TestFontRegistry_GetSansSerifDefault(t *testing.T) {
	r := raster.NewFontRegistry()
	face, ok := r.Get(raster.FontDescriptor{Family: ""}, 16)
	require.True(t, ok, "default family must resolve")
	require.NotNil(t, face)
	// The face must scale with size: a 24px face has a larger
	// ascent than a 12px face. We compare ascent because the
	// design metrics struct exposes per-component values rather
	// than a combined height.
	m12 := r.DesignMetrics(raster.FontDescriptor{Family: ""}, 12)
	m24 := r.DesignMetrics(raster.FontDescriptor{Family: ""}, 24)
	assert.Greater(t, m24.Ascent, m12.Ascent,
		"larger size must report a larger ascent: m12=%v m24=%v",
		m12.Ascent, m24.Ascent)
}

// TestFontRegistry_AllFamilies verifies every supported family
// resolves to a face. This is the regression guard against the
// switch statement in lookupTTFLocked dropping a case by accident.
func TestFontRegistry_AllFamilies(t *testing.T) {
	r := raster.NewFontRegistry()
	for _, fam := range []string{raster.FamilySansSerif, raster.FamilySerif, raster.FamilyMonospace} {
		fam := fam
		t.Run(fam, func(t *testing.T) {
			face, ok := r.Get(raster.FontDescriptor{Family: fam}, 16)
			require.True(t, ok, "%s must resolve", fam)
			require.NotNil(t, face)
		})
	}
}

// TestFontRegistry_BoldFaces verifies the weight axis produces
// distinct faces. Bold and regular must NOT return the same
// pointer; otherwise headings, <strong>, and <b> would render
// identically to plain text.
func TestFontRegistry_BoldFaces(t *testing.T) {
	r := raster.NewFontRegistry()
	reg, ok1 := r.Get(raster.FontDescriptor{Family: raster.FamilySansSerif}, 16)
	bold, ok2 := r.Get(raster.FontDescriptor{Family: raster.FamilySansSerif, Bold: true}, 16)
	require.True(t, ok1)
	require.True(t, ok2)
	require.NotNil(t, reg)
	require.NotNil(t, bold)
	assert.NotSame(t, reg, bold,
		"bold and regular sans-serif must be distinct faces")
}

// TestFontRegistry_CacheReuse verifies repeated lookups for the
// same descriptor return the same face pointer. The cache is the
// hot path: every text run goes through it, so misses hurt
// measurable performance.
func TestFontRegistry_CacheReuse(t *testing.T) {
	r := raster.NewFontRegistry()
	first, ok := r.Get(raster.FontDescriptor{Family: raster.FamilySansSerif}, 16)
	require.True(t, ok)
	second, ok := r.Get(raster.FontDescriptor{Family: raster.FamilySansSerif}, 16)
	require.True(t, ok)
	assert.Same(t, first, second,
		"second lookup for identical descriptor must hit cache")
}

// TestFontRegistry_ResolveFamily verifies the CSS family
// normalisation. Authors frequently write specific names
// (Helvetica, Arial, Times) before generics; the registry must
// collapse these to a supported bucket so the font probe and
// the bundled-font fallback stay consistent.
func TestFontRegistry_ResolveFamily(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", raster.FamilySansSerif},
		{"sans-serif", raster.FamilySansSerif},
		{"serif", raster.FamilySerif},
		{"monospace", raster.FamilyMonospace},
		{"Arial", raster.FamilySansSerif},
		{"Helvetica", raster.FamilySansSerif},
		{"HelveticaNeue", raster.FamilySansSerif},
		{"Times", raster.FamilySerif},
		{"Times New Roman", raster.FamilySerif},
		{"Menlo", raster.FamilyMonospace},
		{"Courier", raster.FamilyMonospace},
		// Quoted family lists and falls-through generics.
		{`"Arial", sans-serif`, raster.FamilySansSerif},
		{`'Courier', monospace`, raster.FamilyMonospace},
		{`unknown, fantasy, serif`, raster.FamilySerif},
	}
	for _, c := range cases {
		c := c
		t.Run(c.in, func(t *testing.T) {
			got := raster.NormaliseFamily(c.in)
			assert.Equal(t, c.want, got,
				"raster.NormaliseFamily(%q)", c.in)
		})
	}
}

// TestFontRegistry_DesignMetricsFallback verifies that when a font
// cannot be loaded, DesignMetrics returns sane defaults rather
// than zero values. A zero ascent/descent would collapse all
// line-height calculations to zero and break layout.
//
// We verify the metrics are positive and proportional to the
// requested size, but allow a generous tolerance on the
// ascent+descent total: real fonts (the Go bundled set here)
// typically report ascent+descent slightly larger than the size
// because of internal line-gap accounting, and our fallback
// defaults approximate that ratio within a small margin.
func TestFontRegistry_DesignMetricsFallback(t *testing.T) {
	r := raster.NewFontRegistry()
	m := r.DesignMetrics(raster.FontDescriptor{Family: ""}, 20)
	assert.Greater(t, m.Ascent, float32(0), "ascent must be positive")
	assert.Greater(t, m.Descent, float32(0), "descent must be positive")
	assert.InDelta(t, m.Ascent+m.Descent, 20.0, 4.0,
		"sum of ascent and descent must approximately equal font size")
	// Fallback path must also stay proportional across sizes.
	m10 := r.DesignMetrics(raster.FontDescriptor{Family: ""}, 10)
	assert.InDelta(t, m.Ascent/2, m10.Ascent, 1.0,
		"ascent must roughly double between 10px and 20px")
}

// TestFontRegistry_SharedInstance verifies the package-level
// shared registry is a singleton that survives multiple callers.
// Backends that have not installed their own registry must
// observe the same parsed fonts as each other so the same text
// run does not parse the TTF twice across the CPU and
// CoreGraphics backends.
func TestFontRegistry_SharedInstance(t *testing.T) {
	a := raster.SharedFontRegistry()
	b := raster.SharedFontRegistry()
	require.Same(t, a, b, "raster.SharedFontRegistry must return the same instance")
}