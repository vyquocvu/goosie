package layout_test

import (
	"testing"
)

// TestTextWrapsBesideFloat guards float intrusion into line breaking: lines
// overlapping a float lay out in the shortened span beside it instead of the
// full content width. Before, the inline pass never saw floats, so a paragraph
// wrapping a 120px float measured 4 full-width lines (72px) where Chromium
// lays 6 shortened ones (108px) - which is what collapsed the floats parity
// fixture to 83.13%.
func TestTextWrapsBesideFloat(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="width: 400px; padding: 15px;">
  <div style="float: left; width: 120px; height: 80px; margin-right: 15px;">Float</div>
  <p>Alpha text wraps around the floated element on the right side. The float takes up space on the left and the inline content flows around it naturally.</p>
</div>
</body></html>`, 800)

	paras := findAllByTag(arena, "p")
	if len(paras) != 1 {
		t.Fatalf("found %d paragraphs, want 1", len(paras))
	}
	if paras[0].H <= 90 {
		t.Errorf("p.H = %v, want more than 90 (wrapped lines beside the float, not full-width ones)", paras[0].H)
	}
	first := findWordObject(arena, "Alpha")
	if first == nil {
		t.Fatal("first word not found")
	}
	// Content starts at X 15; the float holds 15..135 with its right margin to
	// 150, so the first line starts at or past 150.
	if first.X < 148 {
		t.Errorf("first word X = %v, want >= 148 (right of the float)", first.X)
	}
}

// TestTextWrapsBesideRightFloat guards the mirror side: a right float pulls
// line ends left while lines start at the content edge.
func TestTextWrapsBesideRightFloat(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="width: 400px; padding: 15px;">
  <div style="float: right; width: 120px; height: 80px; margin-left: 15px;">Float</div>
  <p>Alpha text wraps around the floated element on the left side. The float takes up space on the right and the inline content flows around it naturally.</p>
</div>
</body></html>`, 800)

	paras := findAllByTag(arena, "p")
	if len(paras) != 1 {
		t.Fatalf("found %d paragraphs, want 1", len(paras))
	}
	if paras[0].H <= 90 {
		t.Errorf("p.H = %v, want more than 90 (wrapped lines beside the float)", paras[0].H)
	}
	first := findWordObject(arena, "Alpha")
	if first == nil {
		t.Fatal("first word not found")
	}
	if !almostEqual(first.X, 15) {
		t.Errorf("first word X = %v, want 15 (lines start at the content edge past a right float)", first.X)
	}
}
