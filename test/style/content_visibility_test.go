package style_test

import (
	"testing"
)

func TestContentVisibilityProperties(t *testing.T) {
	st := styleFor(t, `<html><body>
<div id="target" style="content-visibility: hidden; contain-intrinsic-size: 250px 180px;"></div>
</body></html>`, "", "div")

	if st == nil {
		t.Fatal("target style not found")
	}
	if st.ContentVisibility != "hidden" {
		t.Errorf("ContentVisibility = %q, want %q", st.ContentVisibility, "hidden")
	}
	if st.ContainIntrinsicWidth != 250 {
		t.Errorf("ContainIntrinsicWidth = %v, want 250", st.ContainIntrinsicWidth)
	}
	if st.ContainIntrinsicHeight != 180 {
		t.Errorf("ContainIntrinsicHeight = %v, want 180", st.ContainIntrinsicHeight)
	}
}

func TestContentVisibilitySingleIntrinsicSize(t *testing.T) {
	st := styleFor(t, `<html><body>
<div id="target" style="content-visibility: auto; contain-intrinsic-size: 400px;"></div>
</body></html>`, "", "div")

	if st == nil {
		t.Fatal("target style not found")
	}
	if st.ContentVisibility != "auto" {
		t.Errorf("ContentVisibility = %q, want %q", st.ContentVisibility, "auto")
	}
	if st.ContainIntrinsicWidth != 400 || st.ContainIntrinsicHeight != 400 {
		t.Errorf("IntrinsicSize = %v x %v, want 400 x 400", st.ContainIntrinsicWidth, st.ContainIntrinsicHeight)
	}
}
