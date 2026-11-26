package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/layout"
)

func TestFlexImagePercentHeightUsesContainingHeight(t *testing.T) {
	arena := sessionH(t, `<html><body style="margin:0">
<div style="display:flex;width:340px;height:606px">
<picture><img width="543" height="814" style="height:100%;max-width:none;width:auto"></picture>
</div>
</body></html>`, 1280, 1024)
	layout.Inline(arena, layout.ObjectID(1))

	img := findByTag(arena, "img")
	if img == nil {
		t.Fatal("img not found")
	}
	if !almostEqual(img.H, 606) {
		t.Errorf("img.H = %v, want 606 (100%% of the containing height)", img.H)
	}
	wantW := float32(606) * float32(543) / float32(814)
	if !almostEqual(img.W, wantW) {
		t.Errorf("img.W = %v, want %v (606 at the 543x814 attribute ratio)", img.W, wantW)
	}
}
