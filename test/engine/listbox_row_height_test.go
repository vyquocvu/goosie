package engine_test

import (
	"fmt"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/paint"
)

// GOOSIE-LISTBOX-003: a listbox row is not one line box of the control's font.
// scripts/verify-br101-listbox-rowheight.js (Chromium 153.0.8010.12, 2026-11-06)
// measures the in-page form of the widget (<select size=N>) on this host and
// answers one row box of
//
//	floor-to-1/64(1.2 x font-size) + 1px (the option's own bottom padding)
//
// across EVERY font family (the UA default, Arial, "Times New Roman", system-ui
// and Helvetica all answer the same heights, so the row height is a theme rule,
// not font metrics) and across the font sizes
//
//	13px -> 16.59375   16px -> 20.1875   20px -> 25
//	24px -> 29.796875  32px -> 39.390625
//
// where each content height is 1.2 x font-size floored onto the 1/64px
// LayoutUnit grid (1.2 x 16 = 19.2 floors to 19.1875) plus the measured 1px
// option padding-bottom. An authored line-height does not participate: a
// control with line-height:30px still answers 20.1875 at 16px, so the rule is
// not the font's line box either. Goosie asked its control face for the normal
// line box (18px at 16px) and fell back to size*1.25 when no metrics were
// loaded; both spellings are replaced by the measured rule.

// rowHeightFixture is a select of the anchor's shape at a caller-chosen font
// size, with extra CSS the caller may override the family or line-height with.
func rowHeightFixture(fontSize int, extraCSS string) string {
	return fmt.Sprintf(`<html><head><style>html,body{margin:0}</style></head><body>`+
		`<select id="sel" name="pick" size="1" style="position:absolute;left:40px;top:60px;width:180px;`+
		`font-size:%dpx;%s">`, fontSize, extraCSS) +
		`<option value="a">Alpha</option><option value="b">Bravo</option><option value="c">Charlie</option>` +
		`</select></body></html>`
}

// openRowList raises the listbox of the fixture's select and returns its rows.
func openRowList(t *testing.T, html string) []paint.ListBoxRow {
	t.Helper()
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer s.Close()
	x, y := clickPoint(t, s, "sel")
	if !s.ClickAt(x, y) {
		t.Fatal("a press on a closed <select> did not open its listbox")
	}
	rows := s.SelectPopupRows()
	if len(rows) < 2 {
		t.Fatalf("the listbox opened with %d rows, want at least 2", len(rows))
	}
	return rows
}

// assertRowHeight checks that every row of the open listbox tiles the panel at
// the measured height. The tolerance is 1/512px: far tighter than the 1/64px
// grid that separates the table's values, loose enough for the float32 noise
// the panel placement adds (a 32px row has measured 39.999992 at panel offset).
func assertRowHeight(t *testing.T, rows []paint.ListBoxRow, want float32) {
	t.Helper()
	const eps = float32(1) / 512
	for i, r := range rows {
		h := r.Rect.Y1 - r.Rect.Y0
		if h < want-eps || h > want+eps {
			t.Errorf("row %d height = %v, want %v", i, h, want)
		}
	}
}

// qc:BR-101
func TestListboxRowHeightFollowsTheMeasuredThemeRule(t *testing.T) {
	for _, tc := range []struct {
		fontSize int
		want     float32
	}{
		{13, 16.59375},
		{16, 20.1875},
		{20, 25},
		{24, 29.796875},
		{32, 39.390625},
	} {
		rows := openRowList(t, rowHeightFixture(tc.fontSize, ""))
		assertRowHeight(t, rows, tc.want)
	}
}

// The row height is a theme rule, not font metrics: Chromium answers the same
// row box for every family it renders the listbox with.
//
// qc:BR-101
func TestListboxRowHeightIgnoresTheFontFamily(t *testing.T) {
	for _, family := range []string{"Arial", `"Times New Roman"`, `system-ui`} {
		rows := openRowList(t, rowHeightFixture(16, "font-family:"+family+";"))
		assertRowHeight(t, rows, 20.1875)
	}
}

// An authored line-height does not reach the option rows: the anchor measures
// line-height:30px answering the same 20.1875 as the unstyled control at 16px.
//
// qc:BR-101
func TestListboxRowHeightIgnoresAuthoredLineHeight(t *testing.T) {
	rows := openRowList(t, rowHeightFixture(16, "line-height:30px;"))
	assertRowHeight(t, rows, 20.1875)
}

// The rows tile the panel with the engine's 1px inset on each edge, so the
// three-row panel of the anchor's fixture measures the same 62.5625px box
// Chromium answers for <select size=3> at 16px (the anchor's RECTS line).
//
// qc:BR-101
func TestListboxPanelIsTheRowsPlusTwoOnePixelInsets(t *testing.T) {
	rows := openRowList(t, rowHeightFixture(16, ""))
	span := rows[len(rows)-1].Rect.Y1 - rows[0].Rect.Y0
	if got := span + 2; got != 62.5625 {
		t.Errorf("three-row panel spans rows plus %v, want 62.5625 (rows plus two 1px insets)", got)
	}
}
