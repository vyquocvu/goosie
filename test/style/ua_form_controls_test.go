package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/style"
)

func TestOptionAndOptgroupAreNotRendered(t *testing.T) {
	for _, tag := range []string{"option", "optgroup"} {
		st := styleFor(t, `<html><body><select><optgroup label="g"><option>One</option></optgroup></select></body></html>`, "", tag)
		if st.Display != style.DisplayNone {
			t.Errorf("%s display = %v, want DisplayNone", tag, st.Display)
		}
	}
}

func TestSelectIsAnInlineBlockControl(t *testing.T) {
	st := styleFor(t, `<html><body><select><option>One</option></select></body></html>`, "", "select")
	if st.Display != style.DisplayInlineBlock {
		t.Errorf("select display = %v, want DisplayInlineBlock", st.Display)
	}
}
