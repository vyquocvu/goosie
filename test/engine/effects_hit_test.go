package engine_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/vyquocvu/goosie/internal/archtest"
	"github.com/vyquocvu/goosie/internal/engine"
)

// qc:BR-073
func TestHitTestingIgnoresPaintOrderOfEffects(t *testing.T) {
	sess := newEffectsSession(t)
	defer sess.Close()
	for _, p := range effectsPoints {
		t.Run(p.label, func(t *testing.T) {
			if got := sess.HitTestLink(p.x, p.y); got != p.want {
				t.Errorf("HitTestLink(%v,%v) = %q, want %q (Chromium)", p.x, p.y, got, p.want)
			}
		})
	}
}

// qc:BR-073
func TestClickOnARoundedCornerDoesNotActivateThatControl(t *testing.T) {
	html := `<!doctype html><html><head><style>` +
		`html,body{margin:0}` +
		`#under{position:absolute;left:0;top:0;width:400px;height:200px;background:#eee}` +
		`#over{position:absolute;left:20px;top:20px;width:160px;height:80px;` +
		`border-radius:40px;background:#0a0}` +
		`</style></head><body><div id="under"></div><button id="over" type="submit"></button></body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()
	if sess.ClickAt(21, 21) {
		t.Error("ClickAt on the cut-away corner activated the rounded button")
	}
	if !sess.ClickAt(100, 60) {
		t.Error("ClickAt on the button interior did not activate it")
	}
}

type effectsProbe struct {
	x, y  float32
	want  string
	label string
}

var effectsPoints = []effectsProbe{
	{260, 50, "#radius", "radius centre"},
	{202, 22, "#under", "corner outside the curve"},
	{230, 22, "#radius", "corner inside the curve"},
	{192, 170, "#under", "outline ring"},
	{196, 170, "#under", "gap between box and outline"},
	{330, 300, "#under", "shadow band right"},
	{250, 330, "#under", "shadow band below"},
	{260, 290, "#shadow", "shadow box interior"},
	{10, 10, "#under", "plain underlay"},
}

func newEffectsSession(t *testing.T) *engine.Session {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate this test file")
	}
	root, err := archtest.RepoRoot(file)
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	html, err := os.ReadFile(filepath.Join(root, "testdata", "layout-parity", "effects-001.html"))
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	sess, err := engine.NewSession(string(html), nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	return sess
}
