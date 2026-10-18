package style_test

import (
	"testing"
)

func TestTextWrapAndLineClampProperties(t *testing.T) {
	st := styleFor(t, `<html><body>
<div id="target" style="text-wrap: balance; line-clamp: 3; view-transition-name: main-header;">Hello</div>
</body></html>`, "", "div")

	if st == nil {
		t.Fatal("target style not found")
	}
	if st.TextWrap != "balance" {
		t.Errorf("TextWrap = %q, want %q", st.TextWrap, "balance")
	}
	if st.LineClamp != 3 {
		t.Errorf("LineClamp = %d, want 3", st.LineClamp)
	}
	if st.ViewTransitionName != "main-header" {
		t.Errorf("ViewTransitionName = %q, want %q", st.ViewTransitionName, "main-header")
	}
}

func TestWebkitLineClampAlias(t *testing.T) {
	st := styleFor(t, `<html><body>
<div id="target" style="-webkit-line-clamp: 2;">Content</div>
</body></html>`, "", "div")

	if st == nil {
		t.Fatal("target style not found")
	}
	if st.LineClamp != 2 {
		t.Errorf("LineClamp = %d, want 2", st.LineClamp)
	}
}
