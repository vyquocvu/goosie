package style_test

import "testing"

// qc:BR-330
func TestNegativeShorthandMarginIsNotAuto(t *testing.T) {
	cs := styleFor(t, `<html><body><div id="x">x</div></body></html>`, `div { margin: 0 -8px; }`, "div")
	if cs == nil {
		t.Fatal("missing computed div style")
	}
	if cs.MarginRight != -8 {
		t.Errorf("MarginRight = %v, want -8", cs.MarginRight)
	}
	if cs.MarginRightAuto || cs.MarginLeftAuto {
		t.Errorf("a literal negative margin must not set an auto flag: %+v", cs)
	}
}

// qc:BR-330
func TestThreeValueMarginShorthandTracksBlockAuto(t *testing.T) {
	cs := styleFor(t, `<html><body><div id="x">x</div></body></html>`, `div { margin: auto 1px auto; }`, "div")
	if cs == nil {
		t.Fatal("missing computed div style")
	}
	if !cs.MarginTopAuto || !cs.MarginBottomAuto {
		t.Errorf("top/bottom auto flags = %v/%v, want true/true", cs.MarginTopAuto, cs.MarginBottomAuto)
	}
	if cs.MarginLeftAuto || cs.MarginRightAuto {
		t.Errorf("inline margins must stay definite: %v/%v", cs.MarginLeftAuto, cs.MarginRightAuto)
	}
}
