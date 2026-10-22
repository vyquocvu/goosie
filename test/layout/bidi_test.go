package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/layout"
)

func TestRTLWordPlacement(t *testing.T) {
	// In an 800px wide container with direction: rtl, text aligns to the right and words flow right-to-left
	arena := session(t, `<html><body style="margin: 0;">
<div id="target" style="width: 800px; direction: rtl; font-size: 16px;">
  <span>First</span>
  <span>Second</span>
</div>
</body></html>`, 800)

	var first, second *layout.Object
	for i := range arena.Objects {
		o := &arena.Objects[i]
		if o.Node != nil && o.Node.Text() {
			if o.Node.DataContent == "First" {
				first = o
			} else if o.Node.DataContent == "Second" {
				second = o
			}
		}
	}

	if first == nil || second == nil {
		t.Fatalf("first=%v second=%v text objects not found", first, second)
	}

	// In RTL, "First" is placed to the right of "Second" (first.X > second.X)
	if first.X <= second.X {
		t.Errorf("expected first word (%v) to be to the right of second word (%v) in RTL layout", first.X, second.X)
	}
}
