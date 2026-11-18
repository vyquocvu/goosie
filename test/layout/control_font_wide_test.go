package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/layout"
)

const controlFontWideHTML = `<!DOCTYPE html><html><head><style>
html, body { margin: 0; padding: 0; }
body { font: 16px sans-serif; }
.row { width: 600px; }
.inh input, .inh textarea { font: inherit; }
.ini input { font: initial; }
</style></head><body>
<div class="row" id="g0"><input id="plain" type=text></div>
<div class="row inh" id="g1"><input id="inh" type=text></div>
<div class="row ini" id="g2"><input id="ini" type=text></div>
<div class="row inh" id="g3"><textarea id="inhArea" rows=2 cols=20></textarea></div>
<div class="row" id="g4"><textarea id="plainArea" rows=2 cols=20></textarea></div>
</body></html>`

func controlFontWideArena(t *testing.T) *layout.Arena {
	t.Helper()
	doc := domtest.Parse(controlFontWideHTML)
	sheet := css.ParseForViewport(`
html, body { margin: 0; padding: 0; }
body { font: 16px sans-serif; }
.row { width: 600px; }
.inh input, .inh textarea { font: inherit; }
.ini input { font: initial; }`, 800)
	styles := style.Resolve(doc, []*css.Stylesheet{sheet}, nil)
	arena := layout.Build(doc, styles, nil)
	arena.Metrics = controlLineMetrics{}
	layout.Block(arena, layout.ObjectID(1), 800, 0)
	return arena
}

// qc:BR-323
func TestControlFontWideKeywordMovesTheControlsOwnLine(t *testing.T) {
	arena := controlFontWideArena(t)
	for _, c := range []struct {
		id         string
		w, h       float32
		checkWidth bool
		why        string
	}{
		{"plain", 153, 21, true, "the UA control face alone: 13.3333px gives a 15px line and 6px of chrome"},
		{"inh", 153, 24, true, "`font: inherit` gives the field the document's 16px, so the line is 18 and the box 24 - the reference's width is 174 because its field content tracks the character width, which is GOOSIE-CONTROLWIDTH-001"},
		{"ini", 153, 24, true, "`font: initial` is 16px Times and sizes the same line"},
		{"inhArea", 182, 42, false, "a textarea with the document font is two 18px lines plus 6px of chrome, which is the reference's 42"},
		{"plainArea", 182, 36, false, "the UA monospace field is two 15px lines plus 6px"},
	} {
		w, h := controlUABorderBox(t, arena, c.id)
		if !almostEqual(h, c.h) {
			t.Errorf("#%s border box H = %v, want %v (%s)", c.id, h, c.h, c.why)
		}
		if c.checkWidth && !almostEqual(w, c.w) {
			t.Errorf("#%s border box W = %v, want %v (%s)", c.id, w, c.w, c.why)
		}
	}
}
