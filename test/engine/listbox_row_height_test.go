package engine_test

import (
	"fmt"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/paint"
)

func rowHeightFixture(fontSize int, extraCSS string) string {
	return fmt.Sprintf(`<html><head><style>html,body{margin:0}</style></head><body>`+
		`<select id="sel" name="pick" size="1" style="position:absolute;left:40px;top:60px;width:180px;`+
		`font-size:%dpx;%s">`, fontSize, extraCSS) +
		`<option value="a">Alpha</option><option value="b">Bravo</option><option value="c">Charlie</option>` +
		`</select></body></html>`
}

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

// qc:BR-101
func TestListboxRowHeightIgnoresTheFontFamily(t *testing.T) {
	for _, family := range []string{"Arial", `"Times New Roman"`, `system-ui`} {
		rows := openRowList(t, rowHeightFixture(16, "font-family:"+family+";"))
		assertRowHeight(t, rows, 20.1875)
	}
}

// qc:BR-101
func TestListboxRowHeightIgnoresAuthoredLineHeight(t *testing.T) {
	rows := openRowList(t, rowHeightFixture(16, "line-height:30px;"))
	assertRowHeight(t, rows, 20.1875)
}

// qc:BR-101
func TestListboxPanelIsTheRowsPlusTwoOnePixelInsets(t *testing.T) {
	rows := openRowList(t, rowHeightFixture(16, ""))
	span := rows[len(rows)-1].Rect.Y1 - rows[0].Rect.Y0
	if got := span + 2; got != 62.5625 {
		t.Errorf("three-row panel spans rows plus %v, want 62.5625 (rows plus two 1px insets)", got)
	}
}
