package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/style"
)

// TestFlexItemDisplayIsBlockified guards the rule that a flex or grid item has
// its inline-level display blockified (Flexbox §4, Grid §8.1). A `<span>` in a
// column flex container that stays inline is measured across no width at all, so
// its words stack one per line and the container inherits a height built out of
// them - which is how theuselessweb's fixed footer came to be ~330px tall where
// Chromium draws ~100px.
func TestFlexItemDisplayIsBlockified(t *testing.T) {
	cases := []struct {
		name, html, tag string
		want            style.Display
	}{
		{"span in flex", `<div style="display:flex"><span>x</span></div>`, "span", style.DisplayBlock},
		{"span in inline-flex", `<div style="display:inline-flex"><span>x</span></div>`, "span", style.DisplayBlock},
		{"span in grid", `<div style="display:grid"><span>x</span></div>`, "span", style.DisplayBlock},
		{"inline-block item", `<div style="display:flex"><span style="display:inline-block">x</span></div>`, "span", style.DisplayBlock},
		{"inline-flex item", `<div style="display:flex"><span style="display:inline-flex">x</span></div>`, "span", style.DisplayFlex},
		{"absolutely positioned child is no item", `<div style="display:flex"><span style="position:absolute">x</span></div>`, "span", style.DisplayInline},
		{"span outside a flex container stays inline", `<div><span>x</span></div>`, "span", style.DisplayInline},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := styleFor(t, `<html><body>`+tc.html+`</body></html>`, "", tc.tag)
			if s.Display != tc.want {
				t.Errorf("span Display = %v, want %v", s.Display, tc.want)
			}
		})
	}
}
