package layout_test

import (
	"testing"
)

func TestFindViewTransitions(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<header style="view-transition-name: site-header; width: 400px; height: 60px;">Header</header>
<div style="view-transition-name: hero-img; width: 200px; height: 150px; margin-top: 10px;">Hero</div>
<footer style="view-transition-name: none; width: 400px; height: 50px;">Footer</footer>
</body></html>`, 800)

	vt := arena.FindViewTransitions()
	if len(vt) != 2 {
		t.Fatalf("expected 2 view transitions, got %d", len(vt))
	}
	if vt[0].Name != "site-header" {
		t.Errorf("vt[0].Name = %q, want site-header", vt[0].Name)
	}
	if !almostEqual(vt[0].X1-vt[0].X0, 400) || !almostEqual(vt[0].Y1-vt[0].Y0, 60) {
		t.Errorf("vt[0] size = (%v, %v), want (400, 60)", vt[0].X1-vt[0].X0, vt[0].Y1-vt[0].Y0)
	}
	if vt[1].Name != "hero-img" {
		t.Errorf("vt[1].Name = %q, want hero-img", vt[1].Name)
	}
}
