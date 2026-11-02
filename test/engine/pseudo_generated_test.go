package engine_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/paint"
)

// displayText concatenates the glyph runes of every text command so the test
// can assert on what actually reaches the display list rather than on resolved
// style alone.
func displayText(dl *paint.LayerDL) string {
	var b strings.Builder
	for _, cmd := range dl.All() {
		if cmd.Kind != paint.CmdText {
			continue
		}
		for _, g := range cmd.Text.Glyphs {
			b.WriteRune(g.Rune)
		}
	}
	return b.String()
}

func hasFill(dl *paint.LayerDL, w, h int32, r, g, bl uint8) bool {
	for _, cmd := range dl.All() {
		if cmd.Kind != paint.CmdFill {
			continue
		}
		c := cmd.Color
		dw := cmd.Rect.X1 - cmd.Rect.X0
		dh := cmd.Rect.Y1 - cmd.Rect.Y0
		if c.R() == r && c.G() == g && c.B() == bl &&
			dw >= w && dw <= w+1 && dh >= h && dh <= h+1 {
			return true
		}
	}
	return false
}

// qc:BR-075
func TestGeneratedContentAppearsOnlyWhenContentIsNotNone(t *testing.T) {
	css.ClearInteractiveState()
	t.Cleanup(css.ClearInteractiveState)

	sess, err := engine.NewSession(`<html><head><style>
		body { margin: 0; font: 16px sans-serif; }
		#gen::before { content: "BEFORE"; }
		#gen::after { content: "AFTER"; }
		#none::before { content: none; display: block; width: 40px; height: 10px; background-color: rgb(255,0,0); }
		#empty::before { content: ""; display: block; width: 40px; height: 10px; background-color: rgb(0,128,0); }
	</style></head><body>
		<p id="gen"></p><p id="none"></p><p id="empty"></p>
	</body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sess.Close)

	list, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatal(err)
	}
	dl := list.Build(1)
	text := displayText(dl)
	if !strings.Contains(text, "BEFORE") || !strings.Contains(text, "AFTER") {
		t.Fatalf("generated ::before/::after text missing from display list: %q", text)
	}
	if pos := strings.Index(text, "BEFORE"); pos < 0 || !strings.Contains(text[pos:], "AFTER") {
		t.Fatalf("::before must paint before ::after: %q", text)
	}
	if hasFill(dl, 40, 10, 255, 0, 0) {
		t.Fatal("content:none generated a box with its other declarations applied")
	}
	if !hasFill(dl, 40, 10, 0, 128, 0) {
		t.Fatal(`content:"" must still generate an empty box (clearfix behaviour)`)
	}
}

// qc:BR-075
func TestGeneratedContentFollowsPseudoState(t *testing.T) {
	css.ClearInteractiveState()
	t.Cleanup(css.ClearInteractiveState)

	sess, err := engine.NewSession(`<html><head><style>
		body { margin: 0; font: 16px sans-serif; }
		#host { display: block; width: 200px; height: 40px; }
		#host::after { content: "IDLE"; }
		#host:hover::after { content: "HOVERED"; }
	</style></head><body><div id="host"></div></body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sess.Close)

	host := sess.Doc.ElementByID("host")
	if host == nil {
		t.Fatal("host element missing")
	}

	list, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatal(err)
	}
	if text := displayText(list.Build(1)); !strings.Contains(text, "IDLE") || strings.Contains(text, "HOVERED") {
		t.Fatalf("before hover the pseudo must render IDLE only: %q", text)
	}

	for n := host; n != nil && n.Element(); n = n.Parent {
		css.SetHover(n, true)
	}
	if err := sess.RestyleAndReflow(800); err != nil {
		t.Fatal(err)
	}
	list, err = sess.PaintChecked(1)
	if err != nil {
		t.Fatal(err)
	}
	if text := displayText(list.Build(1)); !strings.Contains(text, "HOVERED") || strings.Contains(text, "IDLE") {
		t.Fatalf("hovering must swap the generated content to HOVERED: %q", text)
	}

	css.ClearInteractiveState()
	if err := sess.RestyleAndReflow(800); err != nil {
		t.Fatal(err)
	}
	list, err = sess.PaintChecked(1)
	if err != nil {
		t.Fatal(err)
	}
	if text := displayText(list.Build(1)); !strings.Contains(text, "IDLE") || strings.Contains(text, "HOVERED") {
		t.Fatalf("unhovering must restore IDLE: %q", text)
	}
}
