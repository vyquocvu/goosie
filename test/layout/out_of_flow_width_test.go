package layout_test

import (
	"testing"
)

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
