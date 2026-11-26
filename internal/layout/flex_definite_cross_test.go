package layout

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/style"
)

func TestFlexDefiniteCross(t *testing.T) {
	a := &Arena{ViewportH: 600}

	fromMinHeight := &Object{Style: &style.ComputedStyle{Height: -1, MinHeight: 600}, Parent: 0, DefiniteH: -1}
	if !flexDefiniteCross(a, fromMinHeight, 600) {
		t.Error("a used height produced by a definite min-height must be definite")
	}

	contentDriven := &Object{Style: &style.ComputedStyle{Height: -1, MinHeight: 20}, Parent: 0, DefiniteH: -1}
	if flexDefiniteCross(a, contentDriven, 100) {
		t.Error("content taller than min-height must stay indefinite")
	}

	auto := &Object{Style: &style.ComputedStyle{Height: -1, MinHeight: 0}, Parent: 0, DefiniteH: -1}
	if flexDefiniteCross(a, auto, 600) {
		t.Error("an arbitrary positive contentH must not make an auto-height container definite")
	}

	stretched := &Object{Style: &style.ComputedStyle{Height: -1, MinHeight: 0}, Parent: 0, DefiniteH: 200}
	if !flexDefiniteCross(a, stretched, 200) {
		t.Error("a stretch-assigned DefiniteH must be definite")
	}

	paddedOverflow := &Object{
		Style:  &style.ComputedStyle{Height: -1, MinHeight: 600, BoxSizing: style.BoxSizingBorderBox},
		Parent: 0, DefiniteH: -1, PaddingTop: 40, PaddingBottom: 40,
	}
	if flexDefiniteCross(a, paddedOverflow, 560) {
		t.Error("a border-box min-height must be compared after subtracting padding")
	}
}
