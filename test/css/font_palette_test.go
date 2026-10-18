package css_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

func TestFontPaletteValuesRuleParsing(t *testing.T) {
	input := `
@font-palette-values --BrandPalette {
	font-family: "CoolFont";
	base-palette: 2;
	override-colors: 0 #ff0000, 1 #00ff00;
}
`
	sheet := css.Parse(input)
	if len(sheet.FontPalettes) != 1 {
		t.Fatalf("expected 1 font-palette-values rule, got %d", len(sheet.FontPalettes))
	}

	fp := sheet.FontPalettes[0]
	if fp.Name != "--BrandPalette" {
		t.Errorf("expected name '--BrandPalette', got %q", fp.Name)
	}
	if fp.FontFamily != "CoolFont" {
		t.Errorf("expected font-family 'CoolFont', got %q", fp.FontFamily)
	}
	if fp.BasePalette != "2" {
		t.Errorf("expected base-palette '2', got %q", fp.BasePalette)
	}
	if fp.OverrideColors[0] != "#ff0000" {
		t.Errorf("expected color 0 to be #ff0000, got %q", fp.OverrideColors[0])
	}
	if fp.OverrideColors[1] != "#00ff00" {
		t.Errorf("expected color 1 to be #00ff00, got %q", fp.OverrideColors[1])
	}
}
