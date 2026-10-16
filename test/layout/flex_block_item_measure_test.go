package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/layout"
)

// A flex item that holds its text inside a padded block child - a nav cell
// wrapping its link - is sized from that child's whole box. Measuring only the
// trailing flank took the label's width plus one side's padding, so the cell
// wrapped its own text and the bar lost a line of height on every page that
// builds a menu out of list items.
const paddedFlexNavHTML = `<html><body style="margin:0">
	<ul style="display:flex;justify-content:space-between;margin:0;list-style:none;width:90%">
		<li><a href="#" style="display:block;padding:.5rem 2rem">Start Here</a></li>
		<li><a href="#" style="display:block;padding:.5rem 2rem">Examples</a></li>
	</ul>
</body></html>`

func TestFlexItemWithBlockChildMeasuresBothFlanksOfItsPadding(t *testing.T) {
	arena := session(t, paddedFlexNavHTML, 1280)
	layout.Inline(arena, layout.ObjectID(1))
	items := findAllByTag(arena, "li")
	links := findAllByTag(arena, "a")
	start := findWordObject(arena, "Start")
	here := findWordObject(arena, "Here")
	if len(items) < 2 || len(links) < 2 || start == nil || here == nil {
		t.Fatalf("missing boxes: li=%d a=%d Start=%v Here=%v", len(items), len(links), start != nil, here != nil)
	}
	link := links[0]
	// The label's own extent, spaces included, measured off the placed words
	// rather than summed word widths: the test should say "the cell holds its
	// text and both flanks of its padding", not restate the word metrics.
	contentW := here.X + here.W - start.X
	want := contentW + link.PaddingLeft + link.PaddingRight
	if diff := items[0].W - want; diff < -1 || diff > 1 {
		t.Errorf("first cell W=%g, want %g (its label plus the link's padding on both sides)", items[0].W, want)
	}
	if here.Y != start.Y {
		t.Errorf("the label wrapped inside its own cell: Start Y=%g Here Y=%g", start.Y, here.Y)
	}
}
