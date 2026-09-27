package style

import (
	"github.com/vyquocvu/goosie/internal/css"
	"testing"
)

func TestClipProperty(t *testing.T) {
	src := `.test { clip:rect(1px,1px,1px,1px); }`
	sheet := css.Parse(src)
	if len(sheet.Rules) == 0 {
		t.Fatal("no rules parsed")
	}
	cs := DefaultStyle()
	for _, d := range sheet.Rules[0].Declarations {
		applyProperty(&cs, d.Property, d.Value, d.Parsed, 16, nil)
	}
	if !cs.HasClip {
		t.Errorf("HasClip should be true, got false")
	}
	if cs.ClipTop != 1 || cs.ClipRight != 1 || cs.ClipBottom != 1 || cs.ClipLeft != 1 {
		t.Errorf("clip rect = (%v,%v,%v,%v), want (1,1,1,1)", cs.ClipTop, cs.ClipRight, cs.ClipBottom, cs.ClipLeft)
	}
}
