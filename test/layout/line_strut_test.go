package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/layout"
)

// A line box is never shorter than its block container's strut - the box the
// container's own font and line-height would make. Grid by Example's stylesheet
// carries `* { line-height: 26px }`, which also lands on the link wrapping its
// 40px title, so measuring a line only from its runs collapsed the heading's
// first line to 26px and pulled the whole page up by half its height.
const strutHTML = `<html><body style="margin:0;line-height:26px">
	<h1 style="font-size:40px;line-height:52px;margin:0"><a href="#" style="line-height:26px">Grid by Example</a></h1>
	<p style="margin:0">Everything you need</p>
</body></html>`

func TestLineBoxIsAtLeastAsTallAsItsStrut(t *testing.T) {
	arena := session(t, strutHTML, 1280)
	layout.Inline(arena, layout.ObjectID(1))
	h1 := findByTag(arena, "h1")
	p := findByTag(arena, "p")
	title := findWordObject(arena, "Grid")
	if h1 == nil || p == nil || title == nil {
		t.Fatalf("missing boxes: h1=%v p=%v title=%v", h1 != nil, p != nil, title != nil)
	}
	if h1.H != 52 {
		t.Errorf("h1 H=%g, want the 52px line-height the heading itself declares", h1.H)
	}
	if p.Y != h1.Y+52 {
		t.Errorf("the paragraph starts at Y=%g, want it below a 52px heading at Y=%g", p.Y, h1.Y)
	}
	if title.Y < h1.Y || title.Y+title.H > h1.Y+h1.H+1 {
		t.Errorf("the title run at Y=%g H=%g left the heading at Y=%g H=%g", title.Y, title.H, h1.Y, h1.H)
	}
}
