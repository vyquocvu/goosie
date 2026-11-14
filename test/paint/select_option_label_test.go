package paint_test

import (
	"testing"
)

// qc:BR-101
func TestSelectPaintsLabelAttributeOverTextContent(t *testing.T) {
	got := paintedSelectText(t, `<select><option selected label="IIIII">WWWWW</option></select>`)
	if got != "IIIII" {
		t.Fatalf("painted select label = %q, want %q: a label attribute is what the control shows", got, "IIIII")
	}
}

// qc:BR-139
func TestSelectPaintsLabelAttributeInteriorVerbatim(t *testing.T) {
	got := paintedSelectText(t, `<select><option selected label="  My   Label  ">WWWWW</option></select>`)
	if got != "My   Label" {
		t.Fatalf("painted select label = %q, want %q (the interior run paints verbatim, edges trimmed)", got, "My   Label")
	}
}

// qc:BR-139
func TestSelectPaintsDisplayEdgesTrimmedAndInteriorVerbatim(t *testing.T) {
	got := paintedSelectText(t, "<select><option selected label=\"\vMy\vLabel\v\">WWWWW</option></select>")
	if got != "My\vLabel" {
		t.Fatalf("painted select label = %q, want %q (edge vertical tabs trimmed, interior kept)", got, "My\vLabel")
	}
}

// qc:BR-101
func TestSelectPaintsTextWhenLabelAttributeIsEmpty(t *testing.T) {
	got := paintedSelectText(t, `<select><option selected label="">WWWWW</option></select>`)
	if got != "WWWWW" {
		t.Fatalf("painted select label = %q, want %q (empty attribute falls back to text)", got, "WWWWW")
	}
}

// qc:BR-101
func TestSelectPaintsNothingForWhitespaceOnlyLabel(t *testing.T) {
	got := paintedSelectText(t, `<select><option selected label="   ">WWWWW</option></select>`)
	if got != "" {
		t.Fatalf("painted select label = %q, want no text (label=%q wins over the text)", got, "   ")
	}
}

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
