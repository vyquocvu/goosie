package engine

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
)

// png7x9DataURL builds the fixture pixel source the retention tests assign.
func png7x9DataURL(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 7, 9))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i+3] = 255
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode 7x9 png: %v", err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

// The memory-only half of BR-252 (GOOSIE-LAZYIMG-001): a detached Image still
// decodes, reports its intrinsic size and is charged to the document budget,
// but is never retained for paint. A node with no box has no pixel oracle, so
// this internal test inspects the retained/pending tables directly after one
// pump, and checks that dropping the result refunded nothing.
func TestDetachedScriptImageIsNeverRetained(t *testing.T) {
	src := png7x9DataURL(t)

	rt, err := js.New(js.Options{URL: "https://example.test/page"})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	html := `<html><body><script>
var detached = new Image();
detached.src = '` + src + `';
</script></body></html>`
	s, err := NewSession(html, nil, 800, WithJS(rt))
	if err != nil {
		rt.Close()
		t.Fatalf("NewSession: %v", err)
	}
	defer s.Close()

	// The detached element decoded and reports its intrinsic size.
	if got, err := rt.Evaluate(`detached.naturalWidth + 'x' + detached.naturalHeight`); err != nil || got != "7x9" {
		t.Fatalf("detached natural size = %q (err %v), want 7x9 (a detached Image still decodes and reports)", got, err)
	}

	// One pump reconciles the recorded result. The node is not connected, so
	// the pixels are dropped instead of retained.
	s.PumpJS(800)

	s.imgMu.Lock()
	retained := len(s.scriptImgs)
	pending := len(s.pendingScriptImgs)
	painted := len(s.images)
	s.imgMu.Unlock()
	if retained != 0 || pending != 0 || painted != 0 {
		t.Errorf("detached Image: retained=%d pending=%d painted=%d, want 0/0/0 (never retained for paint)", retained, pending, painted)
	}

	// The drop refunds nothing: the decode stays charged to the document, so
	// a src-cycling page cannot decode unbounded bytes.
	s.imgReservation.mu.Lock()
	reserved := s.imgReservation.reserved
	s.imgReservation.mu.Unlock()
	if reserved != 7*9 {
		t.Errorf("detached Image: decoded-pixel reservation = %d, want 63 (charged, never refunded on drop)", reserved)
	}
}

// The release rule of TASK-057 §3.3: a retained entry that is dropped (here
// the node is removed from the document) keeps its decoded-pixel charge. A
// refund would let a src-cycling page decode unbounded bytes under a fixed
// per-document ceiling, so this is a memory-only invariant with no pixel
// oracle and is checked directly against the session tables.
func TestScriptImageDropKeepsPixelReservation(t *testing.T) {
	src := png7x9DataURL(t)

	rt, err := js.New(js.Options{URL: "https://example.test/page"})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	s, err := NewSession(`<html><body><img id="pic"></body></html>`, nil, 800, WithJS(rt))
	if err != nil {
		rt.Close()
		t.Fatalf("NewSession: %v", err)
	}
	defer s.Close()

	if _, err := rt.Evaluate(`document.getElementById('pic').src = '` + src + `';`); err != nil {
		t.Fatalf("assign img src: %v", err)
	}
	s.PumpJS(800)

	s.imgMu.Lock()
	retained := len(s.scriptImgs)
	painted := len(s.images)
	s.imgMu.Unlock()
	if retained != 1 || painted != 1 {
		t.Fatalf("after attach: retained=%d painted=%d, want 1/1 (the connected <img> keeps its pixels)", retained, painted)
	}
	s.imgReservation.mu.Lock()
	afterAttach := s.imgReservation.reserved
	s.imgReservation.mu.Unlock()
	if afterAttach != 7*9 {
		t.Fatalf("after attach: decoded-pixel reservation = %d, want 63", afterAttach)
	}

	if _, err := rt.Evaluate(`document.body.removeChild(document.getElementById('pic'));`); err != nil {
		t.Fatalf("remove img: %v", err)
	}
	s.PumpJS(800)

	s.imgMu.Lock()
	retained = len(s.scriptImgs)
	pending := len(s.pendingScriptImgs)
	painted = len(s.images)
	s.imgMu.Unlock()
	if retained != 0 || pending != 0 || painted != 0 {
		t.Errorf("after removal: retained=%d pending=%d painted=%d, want 0/0/0 (the disconnected node's pixels are dropped)", retained, pending, painted)
	}
	s.imgReservation.mu.Lock()
	afterDrop := s.imgReservation.reserved
	s.imgReservation.mu.Unlock()
	if afterDrop != 63 {
		t.Errorf("after removal: decoded-pixel reservation = %d, want 63 (a drop must not refund the charge)", afterDrop)
	}
}
