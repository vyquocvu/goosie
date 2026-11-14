package layout_test

import (
	"testing"
)

func TestReplacedImgKeepsStylePadding(t *testing.T) {
	a := layoutAtViewport(t,
		`<html><body><img src="x.png" style="padding-left:16px;padding-top:16px;"></body></html>`,
		"", 800, 600)
	found := false
	for i := range a.Objects {
		o := &a.Objects[i]
		if o.Node != nil && o.Node.Data == "img" {
			found = true
			if o.PaddingLeft != 16 || o.PaddingTop != 16 {
				t.Errorf("img padding = (%v,%v), want (16,16)", o.PaddingLeft, o.PaddingTop)
			}
		}
	}
	if !found {
		t.Fatal("no img laid out")
	}
}
