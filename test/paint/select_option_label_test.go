package paint_test

import (
	"testing"
)

// What a control shows for an option is decided by the label content attribute,
// not by its text content, and page script cannot read the rendered string - so
// these rules are anchored on the accessibility names Chromium exposes for the
// same markup, which is the closest observable to "what the user sees".
// scripts/verify-br101-option-dom.js on this host (Chromium 1.63, 2026-11-04):
//
//	AX label-vs-text   <option selected label="IIIII">WWWWW
//	                   -> combobox value "IIIII", option name "IIIII"
//	AX label shapes    <option selected label="  My   Label  ">WWWWW
//	                   -> combobox value "My   Label", option name " My Label "
//	                   <option selected label="">WWWWW    -> combobox value "WWWWW"
//	                   <option selected label="   ">WWWWW -> combobox value absent
//	AX value-only      <option selected value="V">        -> option name ""
//
// So: a non-empty label attribute wins over the text, an *empty* attribute is
// the absent one, a whitespace-only attribute is not (it shows nothing rather
// than the text), and a value attribute never names the option.
//
// Goosie shows one normalised form on every surface - the closed box, the
// listbox rows and the accessibility name - because they all read
// (*dom.Node).OptionLabel. Chromium's own two surfaces disagree with each other
// on edge whitespace ("My   Label" against " My Label "); that difference is
// recorded in docs/qa-open-defects.json rather than reproduced here.

// qc:BR-101
func TestSelectPaintsLabelAttributeOverTextContent(t *testing.T) {
	got := paintedSelectText(t, `<select><option selected label="IIIII">WWWWW</option></select>`)
	if got != "IIIII" {
		t.Fatalf("painted select label = %q, want %q: a label attribute is what the control shows", got, "IIIII")
	}
}

// qc:BR-101
func TestSelectPaintsLabelAttributeWithWhitespaceCollapsed(t *testing.T) {
	got := paintedSelectText(t, `<select><option selected label="  My   Label  ">WWWWW</option></select>`)
	if got != "My Label" {
		t.Fatalf("painted select label = %q, want %q", got, "My Label")
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

// The listbox rows are drawn from the same owner as the closed box, so a row
// cannot show one string while the control shows another.
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
