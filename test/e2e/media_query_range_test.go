//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMediaQueryRangeGoosieVsBrowser verifies Media Queries Level 4 range
// syntax ((width <= 1000px), (width < 1200px)) matches Chromium at the
// widths where the rules flip: at 1000px the sidebar must hide and the
// article goes full width; at 1280px the desktop layout (fixed calc width +
// visible sidebar) applies. This is the iana.org/about regression scenario.
func TestMediaQueryRangeGoosieVsBrowser(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)
	fixturePath := filepath.Join(cwd, "fixtures", "media_query_range.html")

	cases := []struct {
		name   string
		width  int
		height int
	}{
		{"compact_1000", 1000, 800},
		{"desktop_1280", 1280, 800},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page := newPage(t)
			defer page.Close()
			// Structural parity (line counts, sidebar visibility, article
			// width) was verified against Chromium DOM probes when this
			// fixture was added, and the compact/desktop media-query flip is
			// covered by unit tests (TestMediaQueryRangeSyntaxIanaBehavior).
			// The residual pixel delta is per-glyph font-advance and
			// anti-aliasing divergence, the documented engine-wide typography
			// gap; this fixture is text-only, so it carries the same kind of
			// threshold as the suite's _media (0.30) and text-heavy fixtures.
			config := VisualTestConfig{
				DiffThreshold:  0.30,
				OutputDir:      filepath.Join("testdata", "results"),
				ViewportWidth:  tc.width,
				ViewportHeight: tc.height,
			}
			CompareGoosieVsBrowser(t, page, fixturePath, "media_query_range_"+tc.name, config)
		})
	}
}
