package paint_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

func controlCommands(t *testing.T, body string) []paint.DisplayCmd {
	t.Helper()
	html := `<!doctype html><html><body style="margin:0">` + body + `</body></html>`
	sess, err := engine.NewSession(html, nil, 800)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() { sess.Close() })
	dl, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	return dl.Build(1).All()
}

func commandGlyphs(cmds []paint.DisplayCmd) string {
	var b strings.Builder
	for _, cmd := range cmds {
		if cmd.Kind != paint.CmdText {
			continue
		}
		for _, g := range cmd.Text.Glyphs {
			b.WriteRune(g.Rune)
		}
	}
	return b.String()
}

func TestColorInputPaintsItsSwatch(t *testing.T) {
	cmds := controlCommands(t, `<input type="color" value="#ff0000">`)
	var swatches int
	for _, cmd := range cmds {
		if cmd.Kind == paint.CmdFill && cmd.Color == frame.RGB(255, 0, 0) && cmd.Rect.X1-cmd.Rect.X0 > 4 && cmd.Rect.Y1-cmd.Rect.Y0 > 4 {
			swatches++
		}
	}
	if swatches == 0 {
		t.Fatal("color input painted no #ff0000 swatch")
	}
}

func TestEmptyColorInputPaintsBlack(t *testing.T) {
	cmds := controlCommands(t, `<input type="color">`)
	for _, cmd := range cmds {
		if cmd.Kind == paint.CmdFill && cmd.Color == frame.RGB(0, 0, 0) && cmd.Rect.X1-cmd.Rect.X0 > 4 {
			return
		}
	}
	t.Fatal("empty color input painted no default black swatch")
}

func TestFileInputPaintsChooseLabel(t *testing.T) {
	got := commandGlyphs(controlCommands(t, `<input type="file">`))
	if !strings.Contains(got, "Choose File") || !strings.Contains(got, "No file chosen") {
		t.Fatalf("file input glyphs = %q", got)
	}
}

func TestDateInputPaintsValueOrPlaceholder(t *testing.T) {
	valued := commandGlyphs(controlCommands(t, `<input type="date" value="2026-10-25">`))
	if !strings.Contains(valued, "2026-10-25") {
		t.Fatalf("date value glyphs = %q", valued)
	}
	empty := commandGlyphs(controlCommands(t, `<input type="date">`))
	if !strings.Contains(empty, "yyyy-mm-dd") {
		t.Fatalf("empty date glyphs = %q", empty)
	}
	if strings.Contains(valued, "yyyy-mm-dd") {
		t.Fatal("a dated input also painted the empty placeholder")
	}
}

func TestTextInputPaintsItsValue(t *testing.T) {
	got := commandGlyphs(controlCommands(t, `<input type="text" value="hi">`))
	if !strings.Contains(got, "hi") {
		t.Fatalf("text input glyphs = %q", got)
	}
}
