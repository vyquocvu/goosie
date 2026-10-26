package engine_test

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

// Engine half of TASK-052: what internal/js deliberately cannot prove.
//
// internal/js owns the HTMLImageElement load state, dispatches onload/onerror
// and settles decode(), but it may not import internal/image or the engine
// (internal/archtest rules.go), so every byte it reports comes back through the
// injected js.ImageLoader. These tests drive the real session loader and pin
// three things a stub loader cannot:
//
//   - a class a script writes reaches the cascade (the binding's className is a
//     real attribute reflection, not an own JS property),
//   - an unbounded `new Image()` loop is stopped by the per-document attempt
//     ceiling, and
//   - script-created images charge the same decoded-pixel reservation a parsed
//     <img> charges, so two detached 4096x4096 images exhaust the document
//     budget and a third is refused before it decodes.
//
// The data: sources decode inline on the script's own goroutine, so the whole
// load sequence is deterministic; no fetcher is installed, and a network URL
// would simply fail as unreachable.

// palettedPNG encodes a solid w x h PNG with a one-colour palette. The pixel
// count (what Probe and the reservation charge) is w*h, while both the encoded
// payload and the decoded buffer stay near one byte per pixel instead of the
// four a same-sized RGBA image would cost - a 4096x4096 fixture is only
// ~17 MiB decoded, which keeps the budget test inside a unit test's means.
func palettedPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewPaletted(image.Rect(0, 0, w, h), color.Palette{color.Black})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode %dx%d png: %v", w, h, err)
	}
	return buf.Bytes()
}

func pngDataURL(b []byte) string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b)
}

// scriptSession builds a document whose only script is body, runs it during
// NewSession, and returns the session plus the runtime the globals live in.
func scriptSession(t *testing.T, body string) (*engine.Session, *js.Runtime) {
	t.Helper()
	rt, err := js.New(js.Options{URL: "https://example.test/page"})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	html := `<!DOCTYPE html><html><body><script>` + body + `</script></body></html>`
	sess, err := engine.NewSession(html, nil, 800,
		engine.WithJS(rt), engine.WithMetrics(&fixedMetrics{advance: 7}))
	if err != nil {
		rt.Close()
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(sess.Close)
	return sess, rt
}

// qc:BR-247
func TestScriptClassNameReachesTheCascade(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/page"})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	html := `<!DOCTYPE html><html><head>
<style>.on { font-size: 18px; }</style>
</head><body>
<div id="t">t</div>
<script>document.getElementById('t').className = 'on';</script>
</body></html>`
	sess, err := engine.NewSession(html, nil, 800,
		engine.WithJS(rt), engine.WithMetrics(&fixedMetrics{advance: 7}))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()

	// The script runs before the load-time cascade, so the class it wrote must
	// be in the selector match. Without the className reflection the assignment
	// would be an own JS property, the div would keep inheriting 16px, and the
	// rule would never apply.
	if got := engineComputed(t, rt, `getComputedStyle(document.getElementById("t")).fontSize`); got != "18px" {
		t.Errorf("computed font-size after className='on' = %q, want 18px (the .on rule)", got)
	}
	if got := engineComputed(t, rt, `document.getElementById("t").getAttribute("class")`); got != "on" {
		t.Errorf("class attribute after className='on' = %q, want on", got)
	}
	if got := engineComputed(t, rt, `document.getElementById("t").classList.contains("on")`); got != "true" {
		t.Errorf("classList after className='on' does not contain on")
	}
}

// qc:BR-248
func TestScriptImageLoopStopsAtTheAttemptCap(t *testing.T) {
	_, rt := scriptSession(t, `
var loaded = 0, failed = 0, capped = 0, other = 0;
var SRC = '`+pngDataURL(palettedPNG(t, 1, 1))+`';
for (var i = 0; i < 1000; i++) {
  var im = new Image();
  im.onload = function(){ loaded += 1; };
  im.onerror = function(){ failed += 1; };
  im.src = SRC;
  im.decode().then(function(){}, function(e){
    if (e && e.message && e.message.indexOf('attempt limit') >= 0) { capped += 1; }
    else { other += 1; }
  });
}
`)

	// maxLoadedImages is 256 (internal/engine/engine.go): the ceiling counts
	// attempts, so the first 256 sources decode and report load, the remaining
	// 744 are refused before any fetch or decode (an error event plus a rejected
	// decode() naming the attempt limit). 1000 - 256 = 744.
	if got := engineComputed(t, rt, `[loaded, failed, capped, other].join(',')`); got != "256,744,744,0" {
		t.Errorf("1000-image loop = %q, want 256,744,744,0 (maxLoadedImages stops it)", got)
	}
}

// qc:BR-249
func TestScriptImagesChargeTheDocumentPixelBudget(t *testing.T) {
	// 4096 * 4096 = 16,777,216 pixels, exactly MaxDecodedImagePixels.
	// MaxDocumentDecodedPixels is 33,554,432, so two images fit exactly and the
	// third must be refused by the reservation rather than by Probe.
	const side = 4096
	if got := int64(side) * int64(side); got != engine.MaxDocumentDecodedPixels/2 {
		t.Fatalf("fixture is %d pixels, want half of MaxDocumentDecodedPixels (%d)", got, engine.MaxDocumentDecodedPixels/2)
	}
	_, rt := scriptSession(t, `var out = '', ims = [];
var SRC = '`+pngDataURL(palettedPNG(t, side, side))+`';
for (var i = 0; i < 3; i++) {
  var im = new Image();
  ims.push(im);
  im.src = SRC;
  im.decode().then(function(){ out += 'ok;'; }, function(e){ out += e.message + ';'; });
}
`)

	if got := engineComputed(t, rt, `out`); !strings.HasPrefix(got, "ok;ok;engine: script image exceeds") {
		t.Errorf("three 4096x4096 script images = %q, want ok;ok; and the pixel-budget error (M04)", got)
	}
	// The two admitted images decoded for real: the load reports their intrinsic
	// size, while the refused third never set one.
	if got := engineComputed(t, rt, `ims[0].naturalWidth + 'x' + ims[0].naturalHeight + ',' +
		ims[1].naturalWidth + 'x' + ims[1].naturalHeight + ',' +
		ims[2].naturalWidth + 'x' + ims[2].naturalHeight`); got != "4096x4096,4096x4096,0x0" {
		t.Errorf("natural sizes after the budget split = %q, want 4096x4096,4096x4096,0x0", got)
	}
}
