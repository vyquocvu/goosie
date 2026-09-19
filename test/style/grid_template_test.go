package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/style"
)

// Wikipedia's Vector 2022 page shell declares its grid with the `grid-template`
// shorthand and spaces it with a bare `column-gap`. Expanding only the longhand
// names left the column template empty and the gap on both axes, so the page
// divided itself evenly instead of giving the sidebar its 12.25rem.
func TestGridTemplateShorthandAndAxisGaps(t *testing.T) {
	s := styleFor(t, `<html><body style="margin:0"><section class="g"></section></body></html>`,
		`.g{display:grid;column-gap:24px;grid-template:min-content 1fr / 12.25rem minmax(0,1fr)}`, "section")
	if s == nil {
		t.Fatal("section style not found")
	}
	if s.GridTemplateRows != "min-content 1fr" {
		t.Errorf("GridTemplateRows = %q, want %q", s.GridTemplateRows, "min-content 1fr")
	}
	if s.GridTemplateColumns != "12.25rem minmax(0,1fr)" {
		t.Errorf("GridTemplateColumns = %q, want %q", s.GridTemplateColumns, "12.25rem minmax(0,1fr)")
	}
	if s.ColumnGap != 24 {
		t.Errorf("ColumnGap = %v, want 24", s.ColumnGap)
	}
	if s.RowGap != 0 {
		t.Errorf("RowGap = %v, want 0 (column-gap never touches the other axis)", s.RowGap)
	}
}

// `gap` is the shorthand for both axes, so it has to set them both.
func TestGapShorthandSetsBothAxes(t *testing.T) {
	s := styleFor(t, `<html><body style="margin:0"><section class="g"></section></body></html>`,
		`.g{display:flex;gap:1rem}`, "section")
	if s == nil {
		t.Fatal("section style not found")
	}
	if s.RowGap != 16 || s.ColumnGap != 16 {
		t.Errorf("RowGap=%v ColumnGap=%v, want 16 on both axes", s.RowGap, s.ColumnGap)
	}
}

// `float` blockifies the box it lands on, so an inline element carrying it takes
// a box of its own and reaches the side it was thrown to instead of joining its
// siblings' run of text.
func TestFloatBlockifiesItsBox(t *testing.T) {
	s := styleFor(t, `<html><body style="margin:0"><span class="f"></span></body></html>`,
		`.f{float:left}`, "span")
	if s == nil {
		t.Fatal("span style not found")
	}
	if s.Display != style.DisplayBlock {
		t.Errorf("span display = %v, want block", s.Display)
	}
}
