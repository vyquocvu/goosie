package paint_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

func placeholderRunColor(t *testing.T, body string, authorCSS []string) (frame.Color, bool, float32) {
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
	for _, cmd := range dl.Build(1).All() {
		if cmd.Kind != paint.CmdText {
			continue
		}
		var sb strings.Builder
		for _, g := range cmd.Text.Glyphs {
			sb.WriteRune(g.Rune)
		}
		if sb.String() == "HELLOWORLD" {
			return cmd.Text.Color, true, cmd.Opacity
		}
	}
	return frame.Color(0), false, 0
}

// qc:BR-321
func TestPlaceholderPaintsAuthorColor(t *testing.T) {
	_, found, _ := placeholderRunColor(t, `<input placeholder="HELLOWORLD">`, nil)
	if !found {
		t.Fatal("placeholder text run was not painted")
	}

	col, found, _ := placeholderRunColor(t, `<input placeholder="HELLOWORLD">`, []string{`input::placeholder { color: rgb(200, 0, 0); }`})
	if !found {
		t.Fatal("placeholder text run was not painted")
	}
	if col != frame.RGB(200, 0, 0) {
		t.Errorf("placeholder color = %v, want rgb(200,0,0) from ::placeholder rule", col)
	}
}

// qc:BR-321
func TestPlaceholderDefaultColorUnchanged(t *testing.T) {
	col, found, _ := placeholderRunColor(t, `<input placeholder="HELLOWORLD">`, []string{`input::placeholder { opacity: 0.5; }`})
	if !found {
		t.Fatal("placeholder text run was not painted")
	}
	if col != frame.RGB(117, 117, 117) {
		t.Errorf("placeholder color = %v, want UA default rgb(117,117,117) when only opacity is set", col)
	}
}
