package layout_test

import (
	"testing"
)

// TestFlexColumnItemDoesNotDoubleShift guards the place pass of a column flex
// container: each item's box and its inline descendants must advance by the
// item's own main-axis extent, once. A declared-height item used to have its
// box shifted twice (assigned an absolute Y and then shifted again by the same
// delta), so the box landed roughly 2x too far down while its text stayed put.
func TestFlexColumnItemDoesNotDoubleShift(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex; flex-direction: column; width: 278px;">
  <div style="height: 200px;"></div>
  <h3 style="margin: 0;">Title One</h3>
  <p style="margin: 0;">Description text here.</p>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 2 {
		t.Fatalf("found %d divs, want 2 (column + figure)", len(divs))
	}
	fig := divs[1]
	if !almostEqual(fig.H, 200) {
		t.Fatalf("fig.H = %v, want 200", fig.H)
	}
	h3 := findByTag(arena, "h3")
	p := findByTag(arena, "p")
	if h3 == nil || p == nil {
		t.Fatal("missing h3 or p")
	}
	// The figure occupies [0, 200); the h3 box must start right after it.
	if !almostEqual(h3.Y, 200) {
		t.Errorf("h3.Y = %v, want 200 (must not double-shift below the figure)", h3.Y)
	}
	// The p follows the h3's own measured height, not the doubled figure span.
	if !(p.Y >= h3.Y && p.Y < 260) {
		t.Errorf("p.Y = %v, want just after h3 (in [%v, 260))", p.Y, h3.Y)
	}
}

// TestFlexColumnCenteredItemSizesToItsContent covers the cross axis of a column
// flex container. Only `align-items: stretch` fills the line, so a centered
// item's width is its fit-content size: max-content clamped to the container.
// Leaving auto-width items at the full container width - which the centre branch
// skipped because it only ran for a declared width - made every centered block
// span the container and pin its text to the start edge. Measured against
// Chromium at a 400px container: "short" is 32px wide at X=184, a 120px child
// makes its parent 120px wide at X=140, and text that overflows 400px keeps the
// full width and wraps.
func TestFlexColumnCenteredItemSizesToItsContent(t *testing.T) {
	t.Run("text item takes its max-content width", func(t *testing.T) {
		arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex; flex-direction: column; align-items: center; width: 400px;">
  <div>short</div>
</div>
</body></html>`, 800)
		divs := findAllByTag(arena, "div")
		if len(divs) != 2 {
			t.Fatalf("found %d divs, want 2", len(divs))
		}
		it := divs[1]
		if !(it.W > 0 && it.W < 400) {
			t.Fatalf("item W = %v, want a fit-content width below the 400px container", it.W)
		}
		if !almostEqual(2*it.X+it.W, 400) {
			t.Errorf("X = %v, W = %v, want the box centred in 400px", it.X, it.W)
		}
	})

	t.Run("item with a block child takes the child width", func(t *testing.T) {
		arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex; flex-direction: column; align-items: center; width: 400px;">
  <div style="margin: 0;"><div style="width: 120px;">fixed 120</div></div>
</div>
</body></html>`, 800)
		divs := findAllByTag(arena, "div")
		if len(divs) != 3 {
			t.Fatalf("found %d divs, want 3", len(divs))
		}
		it, inner := divs[1], divs[2]
		if !almostEqual(it.W, 120) {
			t.Fatalf("item W = %v, want 120 (its widest line is the fixed-width child)", it.W)
		}
		if !almostEqual(it.X, 140) {
			t.Errorf("X = %v, want 140", it.X)
		}
		if !almostEqual(inner.X, 140) {
			t.Errorf("child X = %v, want 140 (must move with the shrunken item)", inner.X)
		}
	})

	t.Run("text wider than the container keeps the full width", func(t *testing.T) {
		arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex; flex-direction: column; align-items: center; width: 400px;">
  <div>wrap wrap wrap wrap wrap wrap wrap wrap wrap wrap wrap wrap wrap wrap wrap wrap wrap wrap</div>
</div>
</body></html>`, 800)
		divs := findAllByTag(arena, "div")
		if len(divs) != 2 {
			t.Fatalf("found %d divs, want 2", len(divs))
		}
		it := divs[1]
		if !almostEqual(it.W, 400) {
			t.Fatalf("item W = %v, want 400 (max-content overflows the container)", it.W)
		}
		if !almostEqual(it.X, 0) {
			t.Errorf("X = %v, want 0", it.X)
		}
		if !(it.H > 18) {
			t.Errorf("H = %v, want the content re-wrapped onto more than one line", it.H)
		}
	})
}
