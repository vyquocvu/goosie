package layout_test

import (
	"testing"
)

// TestOutOfFlowPercentageWidthIsNotAuto guards placeOutOfFlow's width branch.
// `auto` and a percentage are both encoded as negative sentinels in
// ComputedStyle.Width, so testing `s.Width < 0` made `width:50%` on a fixed or
// absolute box fall through to shrink-to-fit sizing and collapse to its
// content width. Full-width fixed footers and nav bars are the common real-world
// shape that hits this.
func TestOutOfFlowPercentageWidthIsNotAuto(t *testing.T) {
	t.Run("percent width resolves against the containing block", func(t *testing.T) {
		arena := session(t, `<html><body style="margin: 0;">
<div style="position: fixed; left: 0; bottom: 0; width: 50%;">footer line one</div>
</body></html>`, 800)
		d := findByTag(arena, "div")
		if d == nil {
			t.Fatal("missing div")
		}
		if !almostEqual(d.W, 400) {
			t.Fatalf("W = %v, want 400 (50%% of an 800px viewport)", d.W)
		}
		if !almostEqual(d.X, 0) {
			t.Errorf("X = %v, want 0", d.X)
		}
	})

	t.Run("border-box percent width subtracts padding", func(t *testing.T) {
		arena := session(t, `<html><body style="margin: 0;">
<div style="position: fixed; left: 0; bottom: 0; width: 100%; box-sizing: border-box; padding: 12px 20px;">footer</div>
</body></html>`, 800)
		d := findByTag(arena, "div")
		if d == nil {
			t.Fatal("missing div")
		}
		if !almostEqual(d.W, 760) {
			t.Fatalf("W = %v, want 760 (800 minus 2x20px padding)", d.W)
		}
	})

	// A default content-box `width: 100%` is the containing block's width and
	// nothing more: the padding rides outside it. placeOutOfFlow lays the box out
	// by handing blockInto a measure width of `resolved width + padding`, which a
	// percentage then resolves against all over again, so the box grows its own
	// padding and takes its children with it.
	t.Run("content-box percent width excludes padding", func(t *testing.T) {
		arena := session(t, `<html><body style="margin: 0;">
<div style="position: fixed; left: 0; bottom: 0; width: 100%; padding: 12px;"><div>footer</div></div>
</body></html>`, 800)
		outer := findByTag(arena, "div")
		if outer == nil {
			t.Fatal("missing div")
		}
		if !almostEqual(outer.W, 800) {
			t.Fatalf("W = %v, want 800 (100%% of the viewport, padding outside it)", outer.W)
		}
	})
}
