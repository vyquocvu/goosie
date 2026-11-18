package paint_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

type placeholderEvidence struct {
	field     frame.Rect
	run       frame.Rect
	baseline  int32
	runColor  frame.Color
	runOp     float32
	runSeq    int32
	lastGlyph int32
	fills     []paint.DisplayCmd
}

func paintPlaceholderRun(t *testing.T, body string, authorCSS []string) placeholderEvidence {
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
	cmds := dl.Build(1).All()
	var out placeholderEvidence
	for _, cmd := range cmds {
		if cmd.Kind == paint.CmdFill && cmd.Color == frame.RGB(255, 255, 255) && cmd.Rect.W() == placeholderFieldBoxWidth && cmd.Rect.H() == placeholderFieldBoxHeight {
			out.field = cmd.Rect
		}
	}
	if out.field.Empty() {
		t.Fatal("the control's own field box was not painted")
	}
	for _, cmd := range cmds {
		switch cmd.Kind {
		case paint.CmdText:
			var sb strings.Builder
			for _, g := range cmd.Text.Glyphs {
				sb.WriteRune(g.Rune)
			}
			if sb.String() != "HELLOWORLD" {
				continue
			}
			if !out.run.Empty() {
				t.Fatal("more than one text run carries HELLOWORLD")
			}
			out.run = cmd.Rect
			out.baseline = cmd.Text.Glyphs[0].Y
			out.runColor = cmd.Text.Color
			out.runOp = cmd.Opacity
			out.runSeq = cmd.Seq
			for _, g := range cmd.Text.Glyphs {
				if g.X > out.lastGlyph {
					out.lastGlyph = g.X
				}
			}
		case paint.CmdFill:
			if cmd.Rect.Intersection(out.field).Empty() {
				continue
			}
			out.fills = append(out.fills, cmd)
		}
	}
	if out.run.Empty() {
		t.Fatal("placeholder text run was not painted")
	}
	return out
}

func (ev placeholderEvidence) report(t *testing.T) {
	t.Helper()
	t.Logf("field=%v run=%v runColor=%v lastGlyphX=%d", ev.field, ev.run, ev.runColor, ev.lastGlyph)
	for _, cmd := range ev.fills {
		t.Logf("fill %v color=%v op=%v", cmd.Rect, cmd.Color, cmd.Opacity)
	}
}

func (ev placeholderEvidence) seqOf(r frame.Rect) int32 {
	for _, cmd := range ev.fills {
		if cmd.Rect == r {
			return cmd.Seq
		}
	}
	return -1
}

func fillsIn(ev placeholderEvidence, col frame.Color) []frame.Rect {
	var hits []frame.Rect
	for _, cmd := range ev.fills {
		if cmd.Color == col {
			hits = append(hits, cmd.Rect)
		}
	}
	return hits
}

func (ev placeholderEvidence) hasFill(col frame.Color, r frame.Rect) bool {
	for _, cmd := range ev.fills {
		if cmd.Color == col && cmd.Rect == r {
			return true
		}
	}
	return false
}

func (ev placeholderEvidence) overflowsField(col frame.Color) []frame.Rect {
	var hits []frame.Rect
	for _, cmd := range ev.fills {
		if cmd.Color != col || cmd.Rect.Intersection(ev.field) == cmd.Rect {
			continue
		}
		hits = append(hits, cmd.Rect)
	}
	return hits
}

const (
	placeholderFieldWidth     = 145
	placeholderFieldLeft      = 4
	placeholderFieldTop       = 3
	placeholderFieldRight     = 149
	placeholderFieldBottom    = 19
	placeholderFontSizePx     = 16
	placeholderFieldBoxWidth  = 153
	placeholderFieldBoxHeight = 22
)

// qc:BR-325
func TestPlaceholderOwnDecorationUnderlineSpansTextAdvance(t *testing.T) {
	ev := paintPlaceholderRun(t, `<input placeholder="HELLOWORLD">`,
		[]string{`input::placeholder { text-decoration: underline solid rgb(255, 0, 0); }`})
	ev.report(t)
	hits := fillsIn(ev, frame.RGB(255, 0, 0))
	if len(hits) != 1 {
		t.Fatalf("decoration fills in rgb(255,0,0) = %d, want 1", len(hits))
	}
	bar := hits[0]
	if bar.X0 != ev.run.X0 {
		t.Errorf("bar starts at x=%d, want the run start %d", bar.X0, ev.run.X0)
	}
	if bar.X1 <= ev.lastGlyph {
		t.Errorf("bar ends at x=%d, before the last glyph at %d", bar.X1, ev.lastGlyph)
	}
	if w := bar.X1 - bar.X0; w >= placeholderFieldWidth {
		t.Errorf("bar width = %d, want the text advance, not the %dpx field content box", w, placeholderFieldWidth)
	}
	if bar.Y0 <= ev.baseline {
		t.Errorf("bar top row y=%d, want it below the %dpx baseline at y=%d", bar.Y0, placeholderFontSizePx, ev.baseline)
	}
	if bar.Y1 > ev.field.Y1 {
		t.Errorf("bar bottom edge y=%d, want it inside the field box ending at y=%d", bar.Y1, ev.field.Y1)
	}
}

// qc:BR-325
func TestPlaceholderUnstyledKeepsDefaultInkAndNoBox(t *testing.T) {
	ev := paintPlaceholderRun(t, `<input placeholder="HELLOWORLD">`, nil)
	ev.report(t)
	if ev.runColor != frame.RGB(117, 117, 117) {
		t.Errorf("placeholder ink = %v, want UA default rgb(117,117,117)", ev.runColor)
	}
	if len(ev.fills) != 1 {
		t.Fatalf("fills inside the field = %d, want only the control's own white background", len(ev.fills))
	}
	if ev.fills[0].Color != frame.RGB(255, 255, 255) {
		t.Errorf("the only fill is %v, want the control's white background", ev.fills[0].Color)
	}
}

// qc:BR-325
func TestPlaceholderOwnDecorationWithoutColourUsesPlaceholderGrey(t *testing.T) {
	ev := paintPlaceholderRun(t, `<input placeholder="HELLOWORLD">`,
		[]string{`input::placeholder { text-decoration: underline; }`})
	ev.report(t)
	grey := fillsIn(ev, frame.RGB(117, 117, 117))
	if len(grey) != 1 {
		t.Fatalf("decoration fills in rgb(117,117,117) = %d, want 1", len(grey))
	}
	if fillsIn(ev, frame.RGB(0, 0, 0)) != nil {
		t.Errorf("the bar painted in black instead of the placeholder colour")
	}
	if grey[0].Y0 <= ev.baseline {
		t.Errorf("bar top row y=%d, want it below the %dpx baseline at y=%d", grey[0].Y0, placeholderFontSizePx, ev.baseline)
	}
}

// qc:BR-325
func TestPlaceholderBackgroundSpansFieldBeforeTheRun(t *testing.T) {
	green := frame.RGB(0, 255, 0)
	ev := paintPlaceholderRun(t, `<input placeholder="HELLOWORLD">`,
		[]string{`input::placeholder { background-color: rgb(0, 255, 0); }`})
	ev.report(t)
	hits := fillsIn(ev, green)
	if len(hits) != 1 {
		t.Fatalf("green fills inside the field = %d, want 1", len(hits))
	}
	bg := hits[0]
	want := frame.Rect4(placeholderFieldLeft, placeholderFieldTop, placeholderFieldRight, placeholderFieldBottom)
	if bg != want {
		t.Errorf("background = %v, want the field content box %v", bg, want)
	}
	if w := bg.X1 - bg.X0; w <= ev.run.X1-ev.run.X0 {
		t.Errorf("background width = %d, want wider than the %dpx text advance", w, ev.run.X1-ev.run.X0)
	}
	if over := ev.overflowsField(green); over != nil {
		t.Errorf("the pseudo background grew outside the field box %v: %+v", ev.field, over)
	}
	if seq := ev.seqOf(bg); seq >= ev.runSeq {
		t.Errorf("background was appended after the text run (seq %d, run seq %d), want paint order background then ink", seq, ev.runSeq)
	}
}

// qc:BR-325
func TestPlaceholderBackgroundFillsBorderBoxUnderTheBorder(t *testing.T) {
	green := frame.RGB(0, 255, 0)
	blue := frame.RGB(0, 0, 255)
	ev := paintPlaceholderRun(t, `<input placeholder="HELLOWORLD">`,
		[]string{`input::placeholder { background-color: rgb(0, 255, 0); border: 2px solid rgb(0, 0, 255); padding: 2px; }`})
	ev.report(t)
	hits := fillsIn(ev, green)
	if len(hits) != 1 {
		t.Fatalf("green fills inside the field = %d, want 1", len(hits))
	}
	bg := hits[0]
	want := frame.Rect4(placeholderFieldLeft, placeholderFieldTop, placeholderFieldRight, placeholderFieldBottom)
	if bg != want {
		t.Errorf("background = %v, want the whole border box %v, not shrunk by the declared border and padding", bg, want)
	}
	edges := []frame.Rect{
		frame.Rect4(placeholderFieldLeft, placeholderFieldTop, placeholderFieldRight, placeholderFieldTop+2),
		frame.Rect4(placeholderFieldLeft, placeholderFieldBottom-2, placeholderFieldRight, placeholderFieldBottom),
		frame.Rect4(placeholderFieldLeft, placeholderFieldTop, placeholderFieldLeft+2, placeholderFieldBottom),
		frame.Rect4(placeholderFieldRight-2, placeholderFieldTop, placeholderFieldRight, placeholderFieldBottom),
	}
	bgSeq := ev.seqOf(bg)
	for _, r := range edges {
		if !ev.hasFill(blue, r) {
			t.Errorf("missing blue edge %v", r)
			continue
		}
		seq := ev.seqOf(r)
		if seq < bgSeq {
			t.Errorf("blue edge %v (seq %d) was appended before the background (seq %d), so the border cannot tint over it", r, seq, bgSeq)
		}
	}
	if ev.run.X0 != placeholderFieldLeft+4 {
		t.Errorf("run starts at x=%d, want the field left %d inset by the 2px border and the 2px padding", ev.run.X0, placeholderFieldLeft)
	}
	if over := ev.overflowsField(green); over != nil {
		t.Errorf("the pseudo background grew outside the field box %v: %+v", ev.field, over)
	}
}

// qc:BR-325
func TestPlaceholderSubPixelBorderKeepsOneDeviceRow(t *testing.T) {
	blue := frame.RGB(0, 0, 255)
	ev := paintPlaceholderRun(t, `<input placeholder="HELLOWORLD">`,
		[]string{`input::placeholder { border: 0.5px solid rgb(0, 0, 255); }`})
	ev.report(t)
	want := []frame.Rect{
		frame.Rect4(placeholderFieldLeft, placeholderFieldTop, placeholderFieldRight, placeholderFieldTop+1),
		frame.Rect4(placeholderFieldLeft, placeholderFieldBottom-1, placeholderFieldRight, placeholderFieldBottom),
		frame.Rect4(placeholderFieldLeft, placeholderFieldTop, placeholderFieldLeft+1, placeholderFieldBottom),
		frame.Rect4(placeholderFieldRight-1, placeholderFieldTop, placeholderFieldRight, placeholderFieldBottom),
	}
	for _, r := range want {
		if !ev.hasFill(blue, r) {
			t.Errorf("missing the 1px blue edge %v that the measured reference keeps for a 0.5px border", r)
		}
	}
	if ev.run.X0 != placeholderFieldLeft+1 {
		t.Errorf("run starts at x=%d, want the field left %d inset by the one device pixel the 0.5px border snaps to", ev.run.X0, placeholderFieldLeft)
	}
	if over := ev.overflowsField(blue); over != nil {
		t.Errorf("the pseudo border grew outside the field box %v: %+v", ev.field, over)
	}
}

// qc:BR-325
func TestPlaceholderOpacityDimsEveryPseudoLayer(t *testing.T) {
	green := frame.RGB(0, 255, 0)
	blue := frame.RGB(0, 0, 255)
	red := frame.RGB(255, 0, 0)
	ev := paintPlaceholderRun(t, `<input placeholder="HELLOWORLD">`,
		[]string{`input::placeholder { opacity: 0.5; background-color: rgb(0, 255, 0); border: 2px solid rgb(0, 0, 255); text-decoration: underline solid rgb(255, 0, 0); }`})
	ev.report(t)
	if ev.runOp != 0.5 {
		t.Errorf("placeholder ink opacity = %v, want the declared 0.5", ev.runOp)
	}
	for _, col := range []frame.Color{green, blue, red} {
		var seen int
		for _, cmd := range ev.fills {
			if cmd.Color != col {
				continue
			}
			seen++
			if cmd.Opacity != 0.5 {
				t.Errorf("%v layer at %v has opacity %v, want the declared 0.5 on every pseudo layer", col, cmd.Rect, cmd.Opacity)
			}
		}
		if seen == 0 {
			t.Errorf("no %v layer reached the display list, so the opacity check above proves nothing", col)
		}
	}
}

// qc:BR-325
func TestPlaceholderThickDecorationStaysInsideFieldBox(t *testing.T) {
	red := frame.RGB(255, 0, 0)
	ev := paintPlaceholderRun(t, `<input placeholder="HELLOWORLD">`,
		[]string{`input::placeholder { text-decoration: underline solid rgb(255, 0, 0); text-decoration-thickness: 5px; }`})
	ev.report(t)
	hits := fillsIn(ev, red)
	if len(hits) == 0 {
		t.Fatal("no bar reached the display list, so the containment below proves nothing")
	}
	for _, bar := range hits {
		if bar.Y1 > placeholderFieldBottom {
			t.Errorf("bar spans rows to y=%d, want it clipped at the field content bottom %d instead of overpainting the control chrome", bar.Y1, placeholderFieldBottom)
		}
		if bar.X0 < placeholderFieldLeft || bar.X1 > placeholderFieldRight {
			t.Errorf("bar spans x%d..%d, want it inside the field content box %d..%d", bar.X0, bar.X1, placeholderFieldLeft, placeholderFieldRight)
		}
	}
}

// qc:BR-325
func TestPlaceholderBorderDrawsFourEdgesAndInsetsTheRun(t *testing.T) {
	blue := frame.RGB(0, 0, 255)
	ev := paintPlaceholderRun(t, `<input placeholder="HELLOWORLD">`,
		[]string{`input::placeholder { border: 2px solid rgb(0, 0, 255); }`})
	ev.report(t)
	want := []frame.Rect{
		frame.Rect4(placeholderFieldLeft, placeholderFieldTop, placeholderFieldRight, placeholderFieldTop+2),
		frame.Rect4(placeholderFieldLeft, placeholderFieldBottom-2, placeholderFieldRight, placeholderFieldBottom),
		frame.Rect4(placeholderFieldLeft, placeholderFieldTop, placeholderFieldLeft+2, placeholderFieldBottom),
		frame.Rect4(placeholderFieldRight-2, placeholderFieldTop, placeholderFieldRight, placeholderFieldBottom),
	}
	for _, r := range want {
		if !ev.hasFill(blue, r) {
			t.Errorf("missing blue edge %v", r)
		}
	}
	if got := len(fillsIn(ev, blue)); got != 4 {
		t.Errorf("blue fills = %d, want the 4 edges of one box", got)
	}
	if ev.run.X0 != placeholderFieldLeft+2 {
		t.Errorf("run starts at x=%d, want the field left %d inset by the 2px pseudo border", ev.run.X0, placeholderFieldLeft)
	}
	if over := ev.overflowsField(blue); over != nil {
		t.Errorf("the pseudo border grew outside the field box %v: %+v", ev.field, over)
	}
}

// qc:BR-325
func TestPlaceholderPaddingInsetsTheRunBeyondTheBorder(t *testing.T) {
	blue := frame.RGB(0, 0, 255)
	ev := paintPlaceholderRun(t, `<input placeholder="HELLOWORLD">`,
		[]string{`input::placeholder { border: 2px solid rgb(0, 0, 255); padding: 2px; }`})
	ev.report(t)
	left := frame.Rect4(placeholderFieldLeft, placeholderFieldTop, placeholderFieldLeft+2, placeholderFieldBottom)
	right := frame.Rect4(placeholderFieldRight-2, placeholderFieldTop, placeholderFieldRight, placeholderFieldBottom)
	if !ev.hasFill(blue, left) || !ev.hasFill(blue, right) {
		t.Errorf("the two side edges vanished with padding: left=%v right=%v", ev.hasFill(blue, left), ev.hasFill(blue, right))
	}
	if ev.run.X0 != placeholderFieldLeft+4 {
		t.Errorf("run starts at x=%d, want the field left %d inset by the 2px border and the 2px padding", ev.run.X0, placeholderFieldLeft)
	}
	if over := ev.overflowsField(blue); over != nil {
		t.Errorf("the pseudo border grew outside the field box %v: %+v", ev.field, over)
	}
}

// qc:BR-325
func TestPlaceholderPseudoBoxStaysInsideTheFieldWithHostDecoration(t *testing.T) {
	blue := frame.RGB(0, 0, 255)
	ev := paintPlaceholderRun(t, `<input placeholder="HELLOWORLD">`,
		[]string{`input { text-decoration: underline solid rgb(0, 0, 255); }`, `input::placeholder { background-color: rgb(0, 255, 0); border: 2px solid rgb(0, 0, 255); padding: 2px; }`})
	ev.report(t)
	if got := len(fillsIn(ev, blue)); got != 5 {
		t.Errorf("blue fills = %d, want the 4 edges of one box plus the propagated host bar", got)
	}
	if over := ev.overflowsField(blue); over != nil {
		t.Errorf("with a host decoration the pseudo border grew outside the field box %v: %+v", ev.field, over)
	}
	if over := ev.overflowsField(frame.RGB(0, 255, 0)); over != nil {
		t.Errorf("with a host decoration the pseudo background grew outside the field box %v: %+v", ev.field, over)
	}
	if ev.run.X0 != placeholderFieldLeft+4 {
		t.Errorf("run starts at x=%d, want the field left %d inset by the 2px border and the 2px padding", ev.run.X0, placeholderFieldLeft)
	}
}

// qc:BR-325
func TestHostThickDecorationStaysInsideFieldBox(t *testing.T) {
	red := frame.RGB(255, 0, 0)
	thin := paintPlaceholderRun(t, `<input placeholder="HELLOWORLD">`,
		[]string{`input { text-decoration: underline solid rgb(255, 0, 0); }`})
	thin.report(t)
	if hits := fillsIn(thin, red); len(hits) != 1 {
		t.Fatalf("host decoration fills with the auto thickness = %d, want the one propagated bar", len(hits))
	} else if hits[0].Y1 > placeholderFieldBottom {
		t.Fatalf("the propagated bar reaches y=%d, past the field content bottom %d", hits[0].Y1, placeholderFieldBottom)
	}
	thick := paintPlaceholderRun(t, `<input placeholder="HELLOWORLD">`,
		[]string{`input { text-decoration: underline solid rgb(255, 0, 0); text-decoration-thickness: 5px; }`})
	thick.report(t)
	hits := fillsIn(thick, red)
	if len(hits) == 0 {
		t.Fatal("no propagated bar reached the display list, so the containment below proves nothing")
	}
	for _, bar := range hits {
		if bar.Y1 > placeholderFieldBottom {
			t.Errorf("a 5px propagated bar spans rows to y=%d, want it clipped at the field content bottom %d instead of overpainting the control chrome", bar.Y1, placeholderFieldBottom)
		}
	}
}

// qc:BR-325
func TestPlaceholderBoxNotPaintedWhenNotShown(t *testing.T) {
	authorCSS := []string{`input::placeholder { background-color: rgb(0, 255, 0); border: 2px solid rgb(0, 0, 255); text-decoration: underline solid rgb(255, 0, 0); }`}
	cases := []struct {
		body    string
		painted string
		reason  string
	}{
		{`<input placeholder="PLACEHOLDERTEXT" value="TYPEDTEXT">`, "TYPEDTEXT", "a field carrying a value does not show its placeholder"},
		{`<input>`, "", "a field with no placeholder attribute has no placeholder box"},
	}
	for _, c := range cases {
		t.Run(c.body, func(t *testing.T) {
			cmds := placeholderCommands(t, c.body, authorCSS)
			for _, cmd := range cmds {
				if cmd.Kind == paint.CmdFill {
					switch cmd.Color {
					case frame.RGB(0, 255, 0), frame.RGB(0, 0, 255), frame.RGB(255, 0, 0):
						t.Errorf("%s, but %v was painted over %v", c.reason, cmd.Color, cmd.Rect)
					}
				}
				if cmd.Kind == paint.CmdText {
					var sb strings.Builder
					for _, g := range cmd.Text.Glyphs {
						sb.WriteRune(g.Rune)
					}
					if sb.String() == "PLACEHOLDERTEXT" {
						t.Errorf("%s, but the placeholder ink was painted at %v", c.reason, cmd.Rect)
					}
				}
			}
			if c.painted != "" && !paintsTextRun(cmds, c.painted) {
				t.Errorf("no %q run reached the display list, so the absence above proves nothing", c.painted)
			}
		})
	}
}

func placeholderCommands(t *testing.T, body string, authorCSS []string) []paint.DisplayCmd {
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
	return dl.Build(1).All()
}

func paintsTextRun(cmds []paint.DisplayCmd, text string) bool {
	for _, cmd := range cmds {
		if cmd.Kind != paint.CmdText {
			continue
		}
		var sb strings.Builder
		for _, g := range cmd.Text.Glyphs {
			sb.WriteRune(g.Rune)
		}
		if sb.String() == text {
			return true
		}
	}
	return false
}

// qc:BR-325
func TestHostDecorationStillPaintsThroughPseudoNone(t *testing.T) {
	ev := paintPlaceholderRun(t, `<input placeholder="HELLOWORLD">`,
		[]string{`input { text-decoration: underline solid rgb(0, 0, 255); }`, `input::placeholder { text-decoration: none; }`})
	ev.report(t)
	hits := fillsIn(ev, frame.RGB(0, 0, 255))
	if len(hits) != 1 {
		t.Fatalf("host blue decoration fills = %d, want the single propagated bar", len(hits))
	}
	bar := hits[0]
	if bar.X0 != ev.run.X0 || bar.X1 <= ev.lastGlyph {
		t.Errorf("bar spans x%d..%d, want the placeholder text advance starting at %d", bar.X0, bar.X1, ev.run.X0)
	}
	if bar.Y0 != ev.baseline+1 {
		t.Errorf("bar top row y=%d, want the auto offset one device pixel below the baseline at %d", bar.Y0, ev.baseline)
	}
}
