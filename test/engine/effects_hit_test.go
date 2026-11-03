package engine_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/vyquocvu/goosie/internal/archtest"
	"github.com/vyquocvu/goosie/internal/engine"
)

// BR-073 asks that border-radius, outline and box-shadow effects do not break
// bounds or hit testing. The bounds half is scored by
// TestLayoutParityWithChromium through testdata/layout-parity/effects-001.html;
// these probes are the hit-testing half, measured against
// scripts/verify-br073-effects.js on this host:
//
//	HIT 260,50  -> radius   # centre of the rounded box
//	HIT 202,22  -> under    # inside the border box, outside the corner curve
//	HIT 230,22  -> radius   # inside the corner curve
//	HIT 192,170 -> under    # the outline ring is not part of the hit area
//	HIT 196,170 -> under    # gap between border box and outline
//	HIT 330,300 -> under    # the shadow band right of the box
//	HIT 250,330 -> under    # the shadow band below the box
//	HIT 260,290 -> shadow   # interior still hits the casting box
//	HIT 10,10   -> under    # plain underlay
//
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

// The click path resolves a control through activatableAt, so it has to cut the
// same corner paint cuts: a pointer landing on the tip of a rounded button
// belongs to what is underneath, and only the interior activates the control.
//
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
	// Box-relative (1,1) with a 40px corner: outside the quarter ellipse, and
	// Chromium reports the div there.
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
