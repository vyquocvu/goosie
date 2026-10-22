package style_test

import (
	"testing"
)

func TestMotionPathAndPaletteProperties(t *testing.T) {
	st := styleFor(t, `<html><body>
<div id="target" style="font-palette: --Brand; offset-path: path('M 0 0 L 100 0'); offset-distance: 50px; offset-rotate: auto;">Content</div>
</body></html>`, "", "div")

	if st == nil {
		t.Fatal("target style not found")
	}
	if st.FontPalette != "--Brand" {
		t.Errorf("FontPalette = %q, want --Brand", st.FontPalette)
	}
	if st.OffsetPath != "path('M 0 0 L 100 0')" {
		t.Errorf("OffsetPath = %q, want path('M 0 0 L 100 0')", st.OffsetPath)
	}
	if st.OffsetDistance != 50 {
		t.Errorf("OffsetDistance = %v, want 50", st.OffsetDistance)
	}
	if st.OffsetRotate != "auto" {
		t.Errorf("OffsetRotate = %q, want auto", st.OffsetRotate)
	}
}

func TestOffsetShorthand(t *testing.T) {
	st := styleFor(t, `<html><body>
<div id="target" style="offset: ray(45deg) 75px;">Content</div>
</body></html>`, "", "div")

	if st == nil {
		t.Fatal("target style not found")
	}
	if st.OffsetPath != "ray(45deg)" {
		t.Errorf("OffsetPath = %q, want ray(45deg)", st.OffsetPath)
	}
	if st.OffsetDistance != 75 {
		t.Errorf("OffsetDistance = %v, want 75", st.OffsetDistance)
	}
}
