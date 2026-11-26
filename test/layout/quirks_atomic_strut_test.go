package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

const quirksAtomicStrutCSS = `
html, body { margin: 0; padding: 0; }
body { font: 16px sans-serif; }
.row { width: 100px; }
.chip { display: inline-block; width: 40px; height: 4px; }
`

const quirksAtomicBlockRows = `
<div class=row id=bare><span id=bare-chip class=chip></span></div>
<div class=row id=top><span id=top-chip class=chip style="vertical-align:top"></span></div>
<div class=row id=multiple><span id=multiple-a class=chip></span><span id=multiple-b class=chip></span></div>
<div class=row id=wrapped style="width:40px"><span id=wrapped-a class=chip></span><span id=wrapped-b class=chip></span></div>
`

const quirksAtomicInlineRows = `
<div class=row id=bare><span><span id=bare-chip class=chip></span></span></div>
<div class=row id=top><span><span id=top-chip class=chip style="vertical-align:top"></span></span></div>
<div class=row id=text>text<span id=text-chip class=chip></span></div>
<div class=row id=multiple><span><span id=multiple-a class=chip></span><span id=multiple-b class=chip></span></span></div>
<div class=row id=lines><span id=lines-a class=chip></span><br><span id=lines-b class=chip style="vertical-align:top"></span><br>text<span id=lines-c class=chip></span></div>
<div class=row id=strut style="line-height:26px">text<span id=strut-chip class=chip></span></div>
`

type measuredStrutRow struct {
	id       string
	y, h     float32
	chipIDs  []string
	chipTops []float32
}

func quirksStrutSession(t *testing.T, body string, quirks bool) *layout.Arena {
	t.Helper()
	doctype := "<!doctype html>"
	if quirks {
		doctype = ""
	}
	html := doctype + `<html><head><style>` + quirksAtomicStrutCSS + `</style></head><body>` + body + `</body></html>`
	doc := domtest.Parse(html)
	if doc.QuirksMode != quirks {
		t.Fatalf("document quirks mode = %v, want %v", doc.QuirksMode, quirks)
	}
	var sheets []*css.Stylesheet
	var walk func(*dom.Node)
	walk = func(n *dom.Node) {
		if n.Element() && n.Data == "style" {
			sheets = append(sheets, css.ParseForViewport(n.TextContent(), 800))
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(&doc.Node)
	styles := style.Resolve(doc, sheets, nil)
	arena := layout.Build(doc, styles, nil)
	if arena.QuirksMode != quirks {
		t.Fatalf("layout quirks mode = %v, want %v", arena.QuirksMode, quirks)
	}
	arena.Metrics = chromiumLineMetrics{}
	layout.Block(arena, layout.ObjectID(1), 800, 0)
	return arena
}

func assertMeasuredStrutRows(t *testing.T, arena *layout.Arena, rows []measuredStrutRow) {
	t.Helper()
	for _, want := range rows {
		row := findByID(arena, want.id)
		if row == nil {
			t.Fatalf("#%s has no layout object", want.id)
		}
		if !almostEqual(row.Y, want.y) || !almostEqual(row.BorderH(), want.h) {
			t.Errorf("#%s Y=%g BorderH=%g, want Y=%g H=%g (Chromium)", want.id, row.Y, row.BorderH(), want.y, want.h)
		}
		if len(want.chipIDs) != len(want.chipTops) {
			t.Fatalf("#%s has %d chip IDs and %d expected offsets", want.id, len(want.chipIDs), len(want.chipTops))
		}
		for i, chipID := range want.chipIDs {
			chip := findByID(arena, chipID)
			if chip == nil {
				t.Fatalf("#%s has no layout object", chipID)
			}
			if got := chip.Y - row.Y; !almostEqual(got, want.chipTops[i]) {
				t.Errorf("#%s sits %g below its row, want %g (Chromium)", chipID, got, want.chipTops[i])
			}
		}
	}
}

func TestQuirksAtomicOnlyBlockRowsDropTheStrut(t *testing.T) {
	backCompat := []measuredStrutRow{
		{id: "bare", y: 0, h: 4, chipIDs: []string{"bare-chip"}, chipTops: []float32{0}},
		{id: "top", y: 4, h: 4, chipIDs: []string{"top-chip"}, chipTops: []float32{0}},
		{id: "multiple", y: 8, h: 4, chipIDs: []string{"multiple-a", "multiple-b"}, chipTops: []float32{0, 0}},
		{id: "wrapped", y: 12, h: 8, chipIDs: []string{"wrapped-a", "wrapped-b"}, chipTops: []float32{0, 4}},
	}
	standards := []measuredStrutRow{
		{id: "bare", y: 0, h: 18, chipIDs: []string{"bare-chip"}, chipTops: []float32{10}},
		{id: "top", y: 18, h: 18, chipIDs: []string{"top-chip"}, chipTops: []float32{0}},
		{id: "multiple", y: 36, h: 18, chipIDs: []string{"multiple-a", "multiple-b"}, chipTops: []float32{10, 10}},
		{id: "wrapped", y: 54, h: 36, chipIDs: []string{"wrapped-a", "wrapped-b"}, chipTops: []float32{10, 28}},
	}
	t.Run("backcompat", func(t *testing.T) {
		assertMeasuredStrutRows(t, quirksStrutSession(t, quirksAtomicBlockRows, true), backCompat)
	})
	t.Run("standards", func(t *testing.T) {
		assertMeasuredStrutRows(t, quirksStrutSession(t, quirksAtomicBlockRows, false), standards)
	})
}

func TestQuirksAtomicOnlyInlineRowsDropTheStrut(t *testing.T) {
	backCompat := []measuredStrutRow{
		{id: "bare", y: 0, h: 4, chipIDs: []string{"bare-chip"}, chipTops: []float32{0}},
		{id: "top", y: 4, h: 4, chipIDs: []string{"top-chip"}, chipTops: []float32{0}},
		{id: "text", y: 8, h: 18, chipIDs: []string{"text-chip"}, chipTops: []float32{10}},
		{id: "multiple", y: 26, h: 4, chipIDs: []string{"multiple-a", "multiple-b"}, chipTops: []float32{0, 0}},
		{id: "lines", y: 30, h: 26, chipIDs: []string{"lines-a", "lines-b", "lines-c"}, chipTops: []float32{0, 4, 18}},
		{id: "strut", y: 56, h: 26, chipIDs: []string{"strut-chip"}, chipTops: []float32{14}},
	}
	standards := []measuredStrutRow{
		{id: "bare", y: 0, h: 18, chipIDs: []string{"bare-chip"}, chipTops: []float32{10}},
		{id: "top", y: 18, h: 18, chipIDs: []string{"top-chip"}, chipTops: []float32{0}},
		{id: "text", y: 36, h: 18, chipIDs: []string{"text-chip"}, chipTops: []float32{10}},
		{id: "multiple", y: 54, h: 18, chipIDs: []string{"multiple-a", "multiple-b"}, chipTops: []float32{10, 10}},
		{id: "lines", y: 72, h: 54, chipIDs: []string{"lines-a", "lines-b", "lines-c"}, chipTops: []float32{10, 18, 46}},
		{id: "strut", y: 126, h: 26, chipIDs: []string{"strut-chip"}, chipTops: []float32{14}},
	}
	t.Run("backcompat", func(t *testing.T) {
		assertMeasuredStrutRows(t, quirksStrutSession(t, quirksAtomicInlineRows, true), backCompat)
	})
	t.Run("standards", func(t *testing.T) {
		assertMeasuredStrutRows(t, quirksStrutSession(t, quirksAtomicInlineRows, false), standards)
	})
}
