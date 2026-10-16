package css_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

// TestParseBorderImageSlice pins TRBL expansion, percentage marks, fill in
// any position, and rejection of negatives and overlong lists.
func TestParseBorderImageSlice(t *testing.T) {
	bi, ok := css.ParseBorderImageSlice("40% 30% 20% 10%")
	if !ok {
		t.Fatal("valid slice rejected")
	}
	if bi.Slice != [4]float32{40, 30, 20, 10} {
		t.Errorf("Slice = %v", bi.Slice)
	}
	for i := range bi.SlicePct {
		if !bi.SlicePct[i] {
			t.Errorf("SlicePct[%d] = false, want percent", i)
		}
	}
	bi, ok = css.ParseBorderImageSlice("fill 24")
	if !ok || !bi.Fill || bi.Slice[0] != 24 || bi.SlicePct[0] {
		t.Errorf("fill slice = %+v ok=%v", bi, ok)
	}
	for _, v := range []string{"10 20 30 40 50", "-5", "10px", "fill fill 10"} {
		if _, ok := css.ParseBorderImageSlice(v); ok {
			t.Errorf("ParseBorderImageSlice(%q) accepted, want invalid", v)
		}
	}
}

// TestParseBorderImageWidthRepeat pins width units and repeat modes.
func TestParseBorderImageWidthRepeat(t *testing.T) {
	bi, ok := css.ParseBorderImageWidth("1px 5px 10px 15px")
	if !ok {
		t.Fatal("valid width rejected")
	}
	for i := range bi.WidthU {
		if bi.WidthU[i] != css.BiWidthLength {
			t.Errorf("WidthU[%d] = %v, want length", i, bi.WidthU[i])
		}
	}
	bi, ok = css.ParseBorderImageWidth("1")
	if !ok || bi.WidthU[0] != css.BiWidthNumber {
		t.Errorf("number width = %+v ok=%v", bi, ok)
	}
	bi, ok = css.ParseBorderImageWidth("auto")
	if !ok || bi.WidthU[0] != css.BiWidthAuto {
		t.Errorf("auto width = %+v ok=%v", bi, ok)
	}
	r, ok := css.ParseBorderImageRepeat("round space")
	if !ok || r.RepeatX != css.BiRepeatRound || r.RepeatY != css.BiRepeatSpace || !r.HasRepeat {
		t.Errorf("repeat = %+v ok=%v", r, ok)
	}
	if _, ok := css.ParseBorderImageWidth("-1px"); ok {
		t.Error("negative width accepted")
	}
}

// TestParseBorderImageShorthand pins section splitting: source and slice
// before the slash, width between, outset after, repeat anywhere.
func TestParseBorderImageShorthand(t *testing.T) {
	src, sl, w, o, rep, ok := css.ParseBorderImageShorthand(`url("a.png") 30 30 / 20px / 10px round`)
	if !ok {
		t.Fatal("valid shorthand rejected")
	}
	if src != `url("a.png")` || sl != "30 30" || w != "20px" || o != "10px" || rep != "round" {
		t.Errorf("split = %q %q %q %q %q", src, sl, w, o, rep)
	}
}
