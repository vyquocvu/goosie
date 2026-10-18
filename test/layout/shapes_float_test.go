package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/layout"
)

func TestFloatShapeOutsideContour(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0; font-size: 16px; line-height: 20px;">
<div style="width: 500px;">
  <div id="float" style="float: left; width: 100px; height: 100px; shape-outside: circle(50px at 50px 50px);"></div>
  <span>TopWord</span><br>
  <span>MiddleWord</span>
</div>
</body></html>`, 500)

	var topWord, midWord *layout.Object
	for i := range arena.Objects {
		o := &arena.Objects[i]
		if o.Node != nil && o.Node.Text() {
			if o.Node.DataContent == "TopWord" {
				topWord = o
			} else if o.Node.DataContent == "MiddleWord" {
				midWord = o
			}
		}
	}

	if topWord == nil || midWord == nil {
		t.Fatalf("topWord=%v, midWord=%v not found", topWord, midWord)
	}

	// Because of circular shape-outside, TopWord (at y ~ 0..20) intrudes less than MiddleWord (at y ~ 40..60),
	// so topWord.X should be strictly less than midWord.X.
	if topWord.X >= midWord.X {
		t.Errorf("expected topWord.X (%v) to be less than midWord.X (%v) due to circular contour", topWord.X, midWord.X)
	}
}
