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

	if got := engineComputed(t, rt, `[loaded, failed, capped, other].join(',')`); got != "256,744,744,0" {
		t.Errorf("1000-image loop = %q, want 256,744,744,0 (maxLoadedImages stops it)", got)
	}
}

// qc:BR-249
func TestScriptImagesChargeTheDocumentPixelBudget(t *testing.T) {
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
	if got := engineComputed(t, rt, `ims[0].naturalWidth + 'x' + ims[0].naturalHeight + ',' +
		ims[1].naturalWidth + 'x' + ims[1].naturalHeight + ',' +
		ims[2].naturalWidth + 'x' + ims[2].naturalHeight`); got != "4096x4096,4096x4096,0x0" {
		t.Errorf("natural sizes after the budget split = %q, want 4096x4096,4096x4096,0x0", got)
	}
}
