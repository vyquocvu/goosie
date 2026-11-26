package layout_test

import (
	"testing"
)

func TestFlexStretchDefinesHeightForDescendantPercentage(t *testing.T) {
	arena := sessionH(t, `<html><body style="margin:0">
<div style="display:flex;width:340px;height:200px;align-items:stretch">
  <div id="media" style="flex:0 0 115px"><div id="child" style="height:100%;width:50px"></div></div>
  <div style="flex:1 1 auto"></div>
</div>
</body></html>`, 800, 600)

	media := findByID(arena, "media")
	child := findByID(arena, "child")
	if media == nil || child == nil {
		t.Fatalf("media=%v child=%v, want both boxes", media, child)
	}
	if !almostEqual(media.H, 200) {
		t.Errorf("media.H = %v, want stretched 200", media.H)
	}
	if !almostEqual(child.H, 200) {
		t.Errorf("child.H = %v, want 100%% of the stretched media height", child.H)
	}
}
