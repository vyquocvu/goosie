package paint_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

func paintedSelectText(t *testing.T, selectHTML string) string {
	t.Helper()
	html := `<!doctype html><html><body style="margin:0">` + selectHTML + `</body></html>`
	sess, err := engine.NewSession(html, nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer sess.Close()
	dl, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked failed: %v", err)
	}
	var glyphs []paint.GlyphRun
	for _, cmd := range dl.Build(1).All() {
		if cmd.Kind == paint.CmdText {
			glyphs = append(glyphs, cmd.Text.Glyphs...)
		}
	}
	sort.SliceStable(glyphs, func(i, j int) bool {
		if glyphs[i].Y != glyphs[j].Y {
			return glyphs[i].Y < glyphs[j].Y
		}
		return glyphs[i].X < glyphs[j].X
	})
	var b strings.Builder
	for _, g := range glyphs {
		b.WriteRune(g.Rune)
	}
	return b.String()
}

// qc:BR-101
func TestSelectPaintsNormalizedSelectedLabel(t *testing.T) {
	got := paintedSelectText(t, `<select><option selected>  Hi   there </option></select>`)
	if got != "Hi there" {
		t.Fatalf("painted select label = %q, want %q", got, "Hi there")
	}
}

// qc:BR-101
func TestSelectPaintsLastAuthoredSelectedOption(t *testing.T) {
	got := paintedSelectText(t, `<select>`+
		`<option selected value="a">A</option>`+
		`<option selected value="b">  B   B </option>`+
		`</select>`)
	if got != "B B" {
		t.Fatalf("painted select label = %q, want %q", got, "B B")
	}
}

// qc:BR-101
func TestSelectPaintsNoTextForSelectedOptionWithEmptyLabel(t *testing.T) {
	got := paintedSelectText(t, `<select>`+
		`<option>Placeholder</option>`+
		`<option selected value=""></option>`+
		`</select>`)
	if got != "" {
		t.Fatalf("painted select label = %q, want no text", got)
	}
}

// qc:BR-101
func TestSelectPaintsArrowWithoutSelectedLabel(t *testing.T) {
	html := `<!doctype html><html><body style="margin:0">` +
		`<select style="width:100px;height:20px"><option>Placeholder</option>` +
		`<option selected value=""></option></select></body></html>`
	sess, err := engine.NewSession(html, nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer sess.Close()
	dl, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked failed: %v", err)
	}
	var arrowRows, textGlyphs int
	for _, cmd := range dl.Build(1).All() {
		switch cmd.Kind {
		case paint.CmdFill:
			if cmd.Color == frame.RGB(80, 80, 80) {
				arrowRows++
			}
		case paint.CmdText:
			textGlyphs += len(cmd.Text.Glyphs)
		}
	}
	if arrowRows != 3 {
		t.Errorf("arrow rows = %d, want 3", arrowRows)
	}
	if textGlyphs != 0 {
		t.Errorf("painted glyphs = %d, want 0", textGlyphs)
	}
}
