package paint_test

import (
	"fmt"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
)

// GOOSIE-LISTBOX-001's visual half, and the only one a headless host can give:
// the listbox is rasterized by the same tile path the shipped browser uses
// (raster.RasterizeTile over a Session.PaintChecked display list with real font
// metrics), and the ink a press produces is compared with the ink the same page
// paints closed. A closed select's native popup is not observable in a reference
// browser on this host - scripts/verify-br101-listbox-viewport.js diffs a
// before/after page.screenshot around a click on a select at y=540 of a 600px
// viewport and measures CHANGED [40,540,219,560], the control's own rect - so
// these oracles pin what the panel exists to guarantee instead of reproducing
// pixels no reference can supply: the rows that reach the tiles are the rows the
// engine handed out, and they are inside the viewport.

// lbListboxFixture is the anchor script's select - left 40, width 180, 16px font -
// at a chosen top, holding n options with `selected` authored selected.
func lbListboxFixture(top float32, n, selected int) string {
	html := fmt.Sprintf(`<html><head><style>html,body{margin:0}</style></head><body>`+
		`<select id="sel" style="position:absolute;left:40px;top:%gpx;width:180px;font-size:16px">`, top)
	for i := 0; i < n; i++ {
		mark := ""
		if i == selected {
			mark = " selected"
		}
		html += fmt.Sprintf(`<option value="v%d"%s>opt%d</option>`, i, mark, i)
	}
	return html + fmt.Sprintf(`</select><div style="position:absolute;left:0;top:%gpx;width:1px;height:1px"></div></body></html>`, 3000+top)
}

// lbInk is one closed-vs-open rasterization of a listbox.
type lbInk struct {
	Open    *frame.Bitmap
	Changed map[[2]int]struct{}
	Rows    []paint.ListBoxRow
	Panel   frame.Rect
	Box     frame.Rect
}

// lbRasterInk renders a page, presses its select, renders it again, and reports
// the pixels that differ. The press also focuses the control, so the control's
// own box may repaint; oracles say which regions must not change.
func lbRasterInk(t *testing.T, top float32, n, selected int) lbInk {
	t.Helper()
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("raster.NewFonts: %v", err)
	}
	s, err := engine.NewSession(lbListboxFixture(top, n, selected), nil, 800,
		engine.WithMetrics(fonts), engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	box := lbDeviceBox(t, s)
	closed := lbRasterTile(t, s, fonts)
	x, y := lbControlCentre(t, s)
	if !s.ClickAt(x, y) {
		t.Fatal("a press on a closed <select> did not open its listbox")
	}
	open := lbRasterTile(t, s, fonts)
	rows := s.SelectPopupRows()
	if len(rows) == 0 {
		t.Fatal("the listbox opened with no rows")
	}
	return lbInk{
		Open:    open,
		Changed: lbDiff(closed, open),
		Rows:    rows,
		Panel:   lbPanelFill(t, s),
		Box:     box,
	}
}

func lbRasterTile(t *testing.T, s *engine.Session, fonts *raster.Fonts) *frame.Bitmap {
	t.Helper()
	dl, err := s.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	bitmap := frame.NewBitmap(800, 600)
	if err := raster.RasterizeTile(dl.Build(1).Publish(), frame.Rect4(0, 0, 800, 600), bitmap,
		fonts, raster.NewGlyphAtlas(1<<20, fonts), frame.RGB(255, 255, 255)); err != nil {
		t.Fatalf("RasterizeTile: %v", err)
	}
	return bitmap
}

// lbDiff is the set of device pixels a press changed - the popup's ink.
func lbDiff(a, b *frame.Bitmap) map[[2]int]struct{} {
	out := map[[2]int]struct{}{}
	for y := 0; y < int(b.H); y++ {
		for x := 0; x < int(b.W); x++ {
			if a.At(x, y) != b.At(x, y) {
				out[[2]int{x, y}] = struct{}{}
			}
		}
	}
	return out
}

func lbControlCentre(t *testing.T, s *engine.Session) (float32, float32) {
	t.Helper()
	x0, y0, x1, y1 := lbCSSBox(t, s)
	return (x0 + x1) / 2, (y0 + y1) / 2
}

func lbCSSBox(t *testing.T, s *engine.Session) (x0, y0, x1, y1 float32) {
	t.Helper()
	for i := range s.Arena.Objects {
		obj := &s.Arena.Objects[i]
		if obj.Node != nil && obj.Node.GetAttribute("id") == "sel" {
			return obj.BorderRect()
		}
	}
	t.Fatal("select#sel not found in the arena")
	return 0, 0, 0, 0
}

func lbDeviceBox(t *testing.T, s *engine.Session) frame.Rect {
	t.Helper()
	x0, y0, x1, y1 := lbCSSBox(t, s)
	return frame.RectF4(x0, y0, x1, y1).ToDevice(1)
}

// lbPanelFill is the device rect the open listbox's panel fill covers, read back
// out of the display list rather than trusted from the engine's row bounds: the
// rows sit 1px inside the panel, and the border strips paint that 1px, so ink
// has to be compared with the rect paint was actually handed.
func lbPanelFill(t *testing.T, s *engine.Session) frame.Rect {
	t.Helper()
	dl, err := s.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	var panel *frame.Rect
	for _, cmd := range dl.Build(1).All() {
		if cmd.Kind != paint.CmdFill || cmd.Color != paint.ListboxPanelColor {
			continue
		}
		if panel != nil {
			t.Fatalf("the open listbox paints %d panels, want one", 2)
		}
		r := cmd.Rect
		panel = &r
	}
	if panel == nil {
		t.Fatal("the open listbox paints no panel fill")
	}
	return *panel
}

// stray returns up to eight changed pixels that lie in neither the panel nor the
// control's box - ink the engine never promised.
func (ink lbInk) stray() [][2]int {
	var bad [][2]int
	for _, p := range lbSorted(ink.Changed) {
		x, y := p[0], p[1]
		if lbInside(x, y, ink.Panel) || lbInside(x, y, ink.Box) {
			continue
		}
		bad = append(bad, [2]int{x, y})
		if len(bad) >= 8 {
			break
		}
	}
	return bad
}

func lbSorted(m map[[2]int]struct{}) [][2]int {
	out := make([][2]int, 0, len(m))
	for p := range m {
		out = append(out, p)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && (out[j][1] < out[j-1][1] || (out[j][1] == out[j-1][1] && out[j][0] < out[j-1][0])); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func lbInside(x, y int, r frame.Rect) bool {
	return int32(x) >= r.X0 && int32(x) < r.X1 && int32(y) >= r.Y0 && int32(y) < r.Y1
}

// A select pushed against the viewport bottom has no room below the control, so
// its rows must reach the tiles above it: a press may not leave ink under the
// control's bottom edge, which is exactly where an unplaced panel painted rows
// nobody could see.
//
// qc:BR-101
func TestFlippedListboxRasterizesAboveTheControl(t *testing.T) {
	ink := lbRasterInk(t, 540, 12, 3)
	if len(ink.Changed) == 0 {
		t.Fatal("a press on the select changed no pixel at all: the listbox never reached the tile rasterizer")
	}
	if bad := ink.stray(); len(bad) > 0 {
		t.Errorf("the open listbox inked pixels outside its panel %v and the control %v: %v", ink.Panel, ink.Box, bad)
	}
	if ink.Panel.Y1 > ink.Box.Y0 {
		t.Errorf("the rasterized rows reach y %v, below the control's top edge %v: the panel opened downward, off screen",
			ink.Panel.Y1, ink.Box.Y0)
	}
	if ink.Panel.Y0 < 0 || ink.Panel.Y1 > 600 {
		t.Errorf("the rasterized panel spans y [%v,%v], off the 600px viewport", ink.Panel.Y0, ink.Panel.Y1)
	}
	// Every row the pointer can hit is a row the tile path inked, sampled at the
	// row's right end where no glyph goes.
	for i, r := range ink.Rows {
		d := r.Rect.ToDevice(1)
		p := [2]int{int(d.X1) - 4, int((d.Y0 + d.Y1) / 2)}
		if _, ok := ink.Changed[p]; !ok {
			t.Errorf("row %d (%s) changes nothing at %v: the panel does not hold the row", i, r.Label, p)
		}
	}
}

// A 40-option list cannot be shown whole in a 600px viewport, so what the tile
// path must ink is the window: the rows the engine handed out, the highlight on
// the selected one, and nothing below the panel where the dropped rows would
// have gone.
//
// qc:BR-101
func TestWindowedListboxRasterizesExactlyTheRowsItShows(t *testing.T) {
	const options = 40
	ink := lbRasterInk(t, 200, options, 33)
	if len(ink.Rows) >= options {
		t.Fatalf("the engine handed out all %d rows for a list that cannot fit; want a window", len(ink.Rows))
	}
	if bad := ink.stray(); len(bad) > 0 {
		t.Errorf("the windowed listbox inked pixels outside its panel %v and the control %v: %v", ink.Panel, ink.Box, bad)
	}
	if ink.Panel.Y0 < 0 || ink.Panel.Y1 > 600 {
		t.Errorf("the windowed panel spans y [%v,%v], off the 600px viewport", ink.Panel.Y0, ink.Panel.Y1)
	}

	selRow := -1
	for i, r := range ink.Rows {
		if r.Selected {
			selRow = i
		}
	}
	if selRow < 0 {
		t.Fatalf("the window shows %v..%v and hides the selected option", ink.Rows[0].Label, ink.Rows[len(ink.Rows)-1].Label)
	}
	d := ink.Rows[selRow].Rect.ToDevice(1)
	if got := ink.Open.At(int(d.X1)-4, int((d.Y0+d.Y1)/2)); got != paint.ListboxHighlightColor {
		t.Errorf("the selected row's background rasterized as %v, want the highlight %v", got, paint.ListboxHighlightColor)
	}
	for i, r := range ink.Rows {
		if i == selRow {
			continue
		}
		rd := r.Rect.ToDevice(1)
		if got := ink.Open.At(int(rd.X1)-4, int((rd.Y0+rd.Y1)/2)); got != paint.ListboxPanelColor {
			t.Errorf("row %d (%s) background rasterized as %v, want the panel %v", i, r.Label, got, paint.ListboxPanelColor)
		}
	}
	// Below the panel the page keeps painting its own background: an option the
	// window dropped must not have been inked at all.
	for _, p := range lbSorted(ink.Changed) {
		if p[1] >= int(ink.Panel.Y1) && !lbInside(p[0], p[1], ink.Box) {
			t.Fatalf("ink at %v, below the panel that ends at y %v: the listbox painted rows it is not showing", p, ink.Panel.Y1)
		}
	}
}

// The everyday shape: a short list under a control near the top of the page has
// all the room it needs below, so the panel must ink downward and a press must
// not turn it upward.
//
// qc:BR-101
func TestListboxThatFitsBelowRasterizesBelowTheControl(t *testing.T) {
	ink := lbRasterInk(t, 60, 12, 3)
	if len(ink.Rows) != 12 {
		t.Fatalf("the listbox inked %d rows for 12 options with room below, want all 12", len(ink.Rows))
	}
	if ink.Panel.Y0 < ink.Box.Y1 {
		t.Errorf("the panel spans y [%v,%v] for a control whose bottom edge is %v: it opened upward where the whole list fits below",
			ink.Panel.Y0, ink.Panel.Y1, ink.Box.Y1)
	}
	if ink.Panel.Y0 < 0 || ink.Panel.Y1 > 600 {
		t.Errorf("the panel spans y [%v,%v], off the 600px viewport", ink.Panel.Y0, ink.Panel.Y1)
	}
	if bad := ink.stray(); len(bad) > 0 {
		t.Errorf("the listbox inked pixels outside its panel %v and the control %v: %v", ink.Panel, ink.Box, bad)
	}
	selRow := -1
	for i, r := range ink.Rows {
		if r.Selected {
			selRow = i
		}
		if d := r.Rect.ToDevice(1); !lbInside(int(d.X1)-4, int((d.Y0+d.Y1)/2), ink.Panel) {
			t.Errorf("row %d (%s) %v lies outside the painted panel %v", i, r.Label, d, ink.Panel)
		}
	}
	if selRow < 0 {
		t.Fatalf("none of the %d rows inked below the control carries the selection", len(ink.Rows))
	}
	d := ink.Rows[selRow].Rect.ToDevice(1)
	if got := ink.Open.At(int(d.X1)-4, int((d.Y0+d.Y1)/2)); got != paint.ListboxHighlightColor {
		t.Errorf("the selected row's background rasterized as %v, want the highlight %v", got, paint.ListboxHighlightColor)
	}
	for _, p := range lbSorted(ink.Changed) {
		if p[1] < int(ink.Box.Y0) && !lbInside(p[0], p[1], ink.Box) {
			t.Fatalf("ink at %v, above the control whose top edge is %v: the listbox is not showing rows there", p, ink.Box.Y0)
		}
	}
}

// A long list under a control with more room above than below is the case neither
// of the fixtures above reaches: the window has to flip upward and stay inside
// the viewport, and the row carrying the selection has to be the one inked with
// the highlight.
//
// qc:BR-101
func TestWindowedListboxFlipsUpwardWhenTheRoomIsAbove(t *testing.T) {
	const options = 40
	ink := lbRasterInk(t, 400, options, 33)
	if len(ink.Rows) >= options {
		t.Fatalf("the window holds %d of %d options in a 600px viewport", len(ink.Rows), options)
	}
	if ink.Panel.Y1 > ink.Box.Y0 {
		t.Errorf("the panel spans y [%v,%v] for a control whose top edge is %v: it opened downward, where the room is smaller",
			ink.Panel.Y0, ink.Panel.Y1, ink.Box.Y0)
	}
	if ink.Panel.Y0 < 0 || ink.Panel.Y1 > 600 {
		t.Errorf("the flipped window spans y [%v,%v], off the 600px viewport", ink.Panel.Y0, ink.Panel.Y1)
	}
	if bad := ink.stray(); len(bad) > 0 {
		t.Errorf("the flipped window inked pixels outside its panel %v and the control %v: %v", ink.Panel, ink.Box, bad)
	}

	selRow := -1
	for i, r := range ink.Rows {
		if r.Selected {
			selRow = i
		}
	}
	if selRow < 0 {
		t.Fatalf("the flipped window shows %v..%v and hides the selected option", ink.Rows[0].Label, ink.Rows[len(ink.Rows)-1].Label)
	}
	d := ink.Rows[selRow].Rect.ToDevice(1)
	if got := ink.Open.At(int(d.X1)-4, int((d.Y0+d.Y1)/2)); got != paint.ListboxHighlightColor {
		t.Errorf("the selected row's background rasterized as %v, want the highlight %v", got, paint.ListboxHighlightColor)
	}
	// Below the control's top edge only the control itself may have repainted: a
	// row of the flipped window must not be inked into the space under it.
	for _, p := range lbSorted(ink.Changed) {
		if p[1] >= int(ink.Box.Y0) && !lbInside(p[0], p[1], ink.Box) {
			t.Fatalf("ink at %v, at or below the control's top edge %v: the flipped window painted past the control",
				p, ink.Box.Y0)
		}
	}
}
