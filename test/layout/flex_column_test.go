package layout_test

import (
	"testing"
)

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
	if !almostEqual(h3.Y, 200) {
		t.Errorf("h3.Y = %v, want 200 (must not double-shift below the figure)", h3.Y)
	}
	if !(p.Y >= h3.Y && p.Y < 260) {
		t.Errorf("p.Y = %v, want just after h3 (in [%v, 260))", p.Y, h3.Y)
	}
}

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
