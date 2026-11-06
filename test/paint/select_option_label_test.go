package paint_test

import (
	"testing"
)

// What a control shows for an option is decided by the label content attribute,
// not by its text content, and page script cannot read the rendered string - so
// these rules are anchored on measurements of what the two engines actually
// paint. scripts/verify-br101-option-dom.js on this host (Chromium 1.63,
// 2026-11-04), lines "AX label-vs-text", "AX label shapes" and "AX value-only
// shapes", fixes what the label attribute is at all:
//
//	label="IIIII"         -> option name "IIIII", combobox value "IIIII"
//	label=""              -> combobox value "WWWWW"   (the empty attribute is the absent one)
//	label="   "           -> combobox value absent     (blank, NOT the text)
//	value="V", no text    -> option name ""            (a value never names an option)
//
// The painted spelling of the closed control is measured by
// scripts/verify-optionax-paint.js (Chromium 1.63, 2026-11-06, dpr 2, dark
// columns inside the control band, gaps classified against baseline labels
// b0..b3 "MyLabel"/"My Label"/"My  Label"/"My   Label" whose maxGap answers
// 1.5/5/9/13 CSS px): the label attribute paints its interior whitespace
// VERBATIM with the edge run trimmed - label="  My   Label  " paints
// maxGap=13 span=62.5, byte-identical to the baseline b3, and label="  XY"
// paints its first ink at the same offset as the unshifted baselines - while
// the text content paints fully normalised (text "  My   Label  " answers
// span=55 maxGap=5, identical to b1). U+000B joins the trimmed edge set
// (label="\vMy\vLabel\v" answers span=55.5, one interior vertical tab's
// advance over b1) but an interior run is kept verbatim
// (label="\vMy\v\vLabel" answers span=60.5 maxGap=10.5, two interior
// advances over b0). That is exactly (*dom.Node).OptionDisplayStyle, the
// spelling the accessibility tree already announces as the combobox value.
//
// The listbox rows are NOT part of that measurement: the popup is a native
// surface outside every headless probe here (GOOSIE-LISTBOX-002), so rows keep
// reading (*dom.Node).OptionLabel and their normalised spelling is pinned by
// TestListboxRowsPaintTheLabelAttribute below rather than claimed as
// Chromium's.

// qc:BR-101
func TestSelectPaintsLabelAttributeOverTextContent(t *testing.T) {
	got := paintedSelectText(t, `<select><option selected label="IIIII">WWWWW</option></select>`)
	if got != "IIIII" {
		t.Fatalf("painted select label = %q, want %q: a label attribute is what the control shows", got, "IIIII")
	}
}

// The interior whitespace of a label attribute is painted verbatim, edges
// trimmed - Chromium 153.0.8010.12 answers label="  My   Label  " with the
// same ink span and word gap as the authored "My   Label" baseline
// (scripts/verify-optionax-paint.js: q1 span=62.5 maxGap=13 = b3, first ink
// unshifted), while the fully normalised spelling would answer b1's gap of 5.
//
// qc:BR-139
func TestSelectPaintsLabelAttributeInteriorVerbatim(t *testing.T) {
	got := paintedSelectText(t, `<select><option selected label="  My   Label  ">WWWWW</option></select>`)
	if got != "My   Label" {
		t.Fatalf("painted select label = %q, want %q (the interior run paints verbatim, edges trimmed)", got, "My   Label")
	}
}

// U+000B is trimmed off the display edges but an interior run stays: Chromium
// answers label="\vMy\vLabel\v" one interior advance over the "My Label"
// baseline and label="\vMy\v\vLabel" two interior advances over "MyLabel"
// (scripts/verify-optionax-paint.js: q6 span=55.5, q7 span=60.5 maxGap=10.5),
// so painting either edge tab or collapsing the interior run is wrong.
//
// qc:BR-139
func TestSelectPaintsDisplayEdgesTrimmedAndInteriorVerbatim(t *testing.T) {
	got := paintedSelectText(t, "<select><option selected label=\"\vMy\vLabel\v\">WWWWW</option></select>")
	if got != "My\vLabel" {
		t.Fatalf("painted select label = %q, want %q (edge vertical tabs trimmed, interior kept)", got, "My\vLabel")
	}
}

// An empty label attribute is the absent one: Chromium shows the text content.
//
// qc:BR-101
func TestSelectPaintsTextWhenLabelAttributeIsEmpty(t *testing.T) {
	got := paintedSelectText(t, `<select><option selected label="">WWWWW</option></select>`)
	if got != "WWWWW" {
		t.Fatalf("painted select label = %q, want %q (empty attribute falls back to text)", got, "WWWWW")
	}
}

// A whitespace-only label attribute is not the absent one: the control shows
// nothing, and must not fall back to the text content it is overriding.
//
// qc:BR-101
func TestSelectPaintsNothingForWhitespaceOnlyLabel(t *testing.T) {
	got := paintedSelectText(t, `<select><option selected label="   ">WWWWW</option></select>`)
	if got != "" {
		t.Fatalf("painted select label = %q, want no text (label=%q wins over the text)", got, "   ")
	}
}

// The listbox rows are drawn from (*dom.Node).OptionLabel, the normalised
// spelling, while the closed control above paints the measured display
// spelling: the popup is a native surface outside every headless probe here
// (GOOSIE-LISTBOX-002), so the rows' spelling is pinned as Goosie's own answer
// instead of being claimed as Chromium's. Rows and control deliberately
// disagree on a label attribute's interior whitespace; the option's accessible
// name, the form value and the row text are still three separately owned
// surfaces.
//
// qc:BR-101
func TestListboxRowsPaintTheLabelAttribute(t *testing.T) {
	html := `<html><head><style>` +
		`html,body{margin:0}` +
		`#sel{position:absolute;left:40px;top:60px;width:180px;font-size:16px}` +
		`</style></head><body>` +
		`<select id="sel">` +
		`<option id="a" value="a" label="  One   One  ">AAAAA</option>` +
		`<option id="b" value="b" selected label="">BBBBB</option>` +
		`<option id="c" value="c" label="   ">CCCCC</option>` +
		`</select></body></html>`
	s, rows := openListbox(t, html)
	cmds := popupCommands(t, s)
	below := textRunsBelow(cmds, rows[0].Rect.Y0-1)
	// Row c paints no glyphs, so two runs carry the three rows' strings.
	if got := runStrings(below); len(got) != 2 {
		t.Fatalf("the listbox drew %v below the control, want two runs (the third row has no text)", got)
	}
	if got := runString(below[0]); got != "One One" {
		t.Errorf("row 0 drew %q, want %q (label attribute, not the text content)", got, "One One")
	}
	if got := runString(below[1]); got != "BBBBB" {
		t.Errorf("row 1 drew %q, want %q (empty label attribute falls back to text)", got, "BBBBB")
	}
}
