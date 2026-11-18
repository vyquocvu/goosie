package paint_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

type runEvidence struct {
	textRect   frame.Rect
	textColor  frame.Color
	lastGlyphX int32
	fills      []paint.DisplayCmd
}

func paintRun(t *testing.T, body string, authorCSS []string) runEvidence {
	t.Helper()
	html := `<!doctype html><html><body style="margin:0">` + body + `</body></html>`
	sess, err := engine.NewSession(html, authorCSS, 800)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() { sess.Close() })
	dl, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	var out runEvidence
	found := false
	for _, cmd := range dl.Build(1).All() {
		if cmd.Kind == paint.CmdText && !found {
			var sb strings.Builder
			for _, g := range cmd.Text.Glyphs {
				sb.WriteRune(g.Rune)
			}
			if sb.String() == "HELLOWORLD" {
				out.textRect = cmd.Rect
				out.textColor = cmd.Text.Color
				for _, g := range cmd.Text.Glyphs {
					if g.X > out.lastGlyphX {
						out.lastGlyphX = g.X
					}
				}
				found = true
			}
		}
		if cmd.Kind == paint.CmdFill && cmd.Rect.H() <= 8 {
			out.fills = append(out.fills, cmd)
		}
	}
	if !found {
		t.Fatal("HELLOWORLD text run was not painted")
	}
	return out
}

func decorationFills(ev runEvidence, col frame.Color) []frame.Rect {
	var hits []frame.Rect
	for _, cmd := range ev.fills {
		if cmd.Color == col && cmd.Rect.Y0 >= ev.textRect.Y0 && cmd.Rect.X0 == ev.textRect.X0 {
			hits = append(hits, cmd.Rect)
		}
	}
	return hits
}

func (ev runEvidence) report(t *testing.T) {
	t.Helper()
	t.Logf("textRect=%v lastGlyphX=%d", ev.textRect, ev.lastGlyphX)
	for _, cmd := range ev.fills {
		t.Logf("fill %v color=%v", cmd.Rect, cmd.Color)
	}
}

// qc:BR-324
func TestDecorationLineUsesDeclaredColorFromShorthand(t *testing.T) {
	ev := paintRun(t, `<p style="font-size:20px">HELLOWORLD</p>`,
		[]string{`p { text-decoration: underline solid rgb(255, 0, 0); }`})
	ev.report(t)
	hits := decorationFills(ev, frame.RGB(255, 0, 0))
	if len(hits) != 1 {
		t.Fatalf("decoration fills in rgb(255,0,0) = %d, want 1", len(hits))
	}
	if ev.textColor == frame.RGB(255, 0, 0) {
		t.Errorf("text run colour is already rgb(255,0,0), the fill proves nothing")
	}
	if got := hits[0].X1 - hits[0].X0; got > ev.lastGlyphX+20-hits[0].X0 || got <= 0 {
		t.Errorf("decoration width = %d, want within the text advance ending near %d", got, ev.lastGlyphX+20)
	}
}

// qc:BR-324
func TestDecorationColorLonghandPaints(t *testing.T) {
	ev := paintRun(t, `<p style="font-size:20px">HELLOWORLD</p>`,
		[]string{`p { text-decoration: underline; text-decoration-color: rgb(0, 0, 255); }`})
	ev.report(t)
	if hits := decorationFills(ev, frame.RGB(0, 0, 255)); len(hits) != 1 {
		t.Fatalf("decoration fills in rgb(0,0,255) = %d, want 1", len(hits))
	}
}

// qc:BR-324
func TestDecorationColorFallsBackToTextColour(t *testing.T) {
	ev := paintRun(t, `<p style="font-size:20px">HELLOWORLD</p>`,
		[]string{`p { text-decoration: underline; color: rgb(200, 100, 50); }`})
	ev.report(t)
	if ev.textColor != frame.RGB(200, 100, 50) {
		t.Fatalf("text run colour = %v, want rgb(200,100,50)", ev.textColor)
	}
	if hits := decorationFills(ev, frame.RGB(200, 100, 50)); len(hits) != 1 {
		t.Fatalf("decoration fills in the current colour = %d, want 1", len(hits))
	}
}

// qc:BR-324
func TestControlDecorationSpansTextNotField(t *testing.T) {
	ev := paintRun(t, `<input value="HELLOWORLD">`,
		[]string{`input { width: 240px; font-size: 16px; text-decoration: underline solid rgb(255, 0, 0); }`})
	ev.report(t)
	hits := decorationFills(ev, frame.RGB(255, 0, 0))
	if len(hits) != 1 {
		t.Fatalf("decoration fills in rgb(255,0,0) = %d, want 1", len(hits))
	}
	if w := hits[0].X1 - hits[0].X0; w >= 200 {
		t.Errorf("decoration width = %d, want the text advance, not the 240px field", w)
	}
	if hits[0].X1 <= ev.lastGlyphX {
		t.Errorf("decoration ends at %d, before the last glyph at %d", hits[0].X1, ev.lastGlyphX)
	}
}

// qc:BR-324
func TestPlaceholderDecorationSpansTextNotField(t *testing.T) {
	ev := paintRun(t, `<input placeholder="HELLOWORLD">`,
		[]string{`input { width: 240px; font-size: 16px; text-decoration: underline solid rgb(255, 0, 0); }`})
	ev.report(t)
	hits := decorationFills(ev, frame.RGB(255, 0, 0))
	if len(hits) != 1 {
		t.Fatalf("decoration fills in rgb(255,0,0) = %d, want 1", len(hits))
	}
	if w := hits[0].X1 - hits[0].X0; w >= 200 {
		t.Errorf("decoration width = %d, want the text advance, not the 240px field", w)
	}
}

// qc:BR-324
func TestDecorationShorthandResetsPaintedThicknessBySourceOrder(t *testing.T) {
	ev := paintRun(t, `<p style="font-size:16px">HELLOWORLD</p>`,
		[]string{`p { text-decoration-thickness: 3px; text-decoration: underline solid rgb(255, 0, 0); }`})
	ev.report(t)
	hits := decorationFills(ev, frame.RGB(255, 0, 0))
	if len(hits) != 1 {
		t.Fatalf("decoration fills in rgb(255,0,0) = %d, want 1", len(hits))
	}
	if got := hits[0].Y1 - hits[0].Y0; got != 1 {
		t.Errorf("bar height after the shorthand = %d px, want the auto 1 px at a 16px font (Chromium paints one row at dy=+16 here)", got)
	}

	ev = paintRun(t, `<p style="font-size:16px">HELLOWORLD</p>`,
		[]string{`p { text-decoration: underline solid rgb(255, 0, 0); text-decoration-thickness: 3px; }`})
	ev.report(t)
	hits = decorationFills(ev, frame.RGB(255, 0, 0))
	if len(hits) != 1 {
		t.Fatalf("decoration fills in rgb(255,0,0) = %d, want 1", len(hits))
	}
	if got := hits[0].Y1 - hits[0].Y0; got != 3 {
		t.Errorf("bar height for a longhand declared after the shorthand = %d px, want 3 (Chromium computes 3px here and inks one saturated device row at dy=+17; the three-row bar is GOOSIE-DECOLONGHAND-001)", got)
	}
}
