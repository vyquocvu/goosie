package layout_test

import (
	"testing"
)

func TestVideoIntrinsicSize(t *testing.T) {
	arena := session(t, `<html><body><video></video></body></html>`, 800)
	v := findByTag(arena, "video")
	if v == nil {
		t.Fatal("video not found")
	}
	if v.W != 300 || v.H != 150 {
		t.Errorf("video default size = %vx%v, want 300x150", v.W, v.H)
	}
}

func TestVideoCustomAttributes(t *testing.T) {
	arena := session(t, `<html><body><video width="480" height="270"></video></body></html>`, 800)
	v := findByTag(arena, "video")
	if v == nil {
		t.Fatal("video not found")
	}
	if v.W != 480 || v.H != 270 {
		t.Errorf("video explicit size = %vx%v, want 480x270", v.W, v.H)
	}
}

func TestAudioWithControlsHasSize(t *testing.T) {
	arena := session(t, `<html><body><audio controls></audio></body></html>`, 800)
	a := findByTag(arena, "audio")
	if a == nil {
		t.Fatal("audio not found")
	}
	if a.W != 300 || a.H != 32 {
		t.Errorf("audio with controls size = %vx%v, want 300x32", a.W, a.H)
	}
}

func TestAudioWithoutControlsHidden(t *testing.T) {
	arena := session(t, `<html><body><audio></audio></body></html>`, 800)
	a := findByTag(arena, "audio")
	// By default audio:not([controls]) is display:none, so it doesn't get a layout box
	if a != nil && a.W > 0 && a.H > 0 {
		t.Errorf("audio without controls should not have visible size, got %vx%v", a.W, a.H)
	}
}
