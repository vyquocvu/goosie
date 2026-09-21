package css

import (
	"testing"
)

func TestFontFaceParsing(t *testing.T) {
	src := `
@font-face {
	font-family: 'Technique';
	src: url(/fonts/neuropol.ttf);
}
@font-face {
	font-family: 'Roboto';
	src: url(/fonts/Roboto-Regular.ttf);
	font-weight: normal;
}
body { font-family: 'Roboto', sans-serif; }
`
	sheet := Parse(src)
	if len(sheet.FontFaces) != 2 {
		t.Fatalf("expected 2 font-face rules, got %d", len(sheet.FontFaces))
	}
	if len(sheet.Rules) != 1 {
		t.Fatalf("expected 1 regular rule, got %d", len(sheet.Rules))
	}
	
	ff := sheet.FontFaces[0]
	if len(ff.Declarations) != 2 {
		t.Fatalf("expected 2 declarations in first font-face, got %d", len(ff.Declarations))
	}
	if ff.Declarations[0].Property != "font-family" {
		t.Errorf("expected font-family, got %s", ff.Declarations[0].Property)
	}
	if ff.Declarations[1].Property != "src" {
		t.Errorf("expected src, got %s", ff.Declarations[1].Property)
	}
}
