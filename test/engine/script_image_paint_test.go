package engine_test

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
	"runtime"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/paint"
)

type blockingFetcher struct {
	entered chan string
	release chan fetchResult
}

type fetchResult struct {
	data []byte
	err  error
}

func newBlockingFetcher() *blockingFetcher {
	return &blockingFetcher{
		entered: make(chan string, 4),
		release: make(chan fetchResult),
	}
}

func (f *blockingFetcher) fetch(base, url string) ([]byte, error) {
	f.entered <- url
	r := <-f.release
	return r.data, r.err
}

func (f *blockingFetcher) waitForURL(t *testing.T, sess *engine.Session, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		select {
		case got := <-f.entered:
			if got != want {
				t.Fatalf("fetcher was asked for %q, want %q (which source the script assigned)", got, want)
			}
			return
		default:
		}
		sess.PumpJS(800)
		if time.Now().After(deadline) {
			t.Fatalf("the fetcher was never asked for %q", want)
		}
		runtime.Gosched()
	}
}

func pumpUntilSet(t *testing.T, sess *engine.Session, rt *js.Runtime, flag string) (before, after engine.WorkCounts) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		before = sess.WorkCounts()
		sess.PumpJS(800)
		after = sess.WorkCounts()
		if engineComputed(t, rt, flag) == "yes" {
			return before, after
		}
		if time.Now().After(deadline) {
			t.Fatalf("the page never set %s", flag)
		}
		runtime.Gosched()
	}
}

func solidPNG(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode %dx%d png: %v", w, h, err)
	}
	return buf.Bytes()
}

func dataURL(b []byte) string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b)
}

func lazyImageSession(t *testing.T, rt *js.Runtime, fetch engine.ImageFetcher, html string) *engine.Session {
	t.Helper()
	sess, err := engine.NewSession(html, nil, 800,
		engine.WithJS(rt),
		engine.WithImages("https://example.test/page", fetch),
		engine.WithMetrics(&fixedMetrics{advance: 7}))
	if err != nil {
		rt.Close()
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(sess.Close)
	return sess
}

func laidOutImage(t *testing.T, sess *engine.Session, id string) *layout.Object {
	t.Helper()
	n := sess.Doc.ElementByID(id)
	if n == nil {
		t.Fatalf("<img id=%q> was not parsed", id)
	}
	oid, ok := sess.Arena.ForNode(n.ID)
	if !ok {
		t.Fatalf("<img id=%q> has no layout object", id)
	}
	return sess.Arena.Get(oid)
}

func checkFixturePixels(t *testing.T, obj *layout.Object, want color.RGBA, what string) {
	t.Helper()
	img, ok := obj.Image.(image.Image)
	if !ok || img == nil {
		t.Fatalf("%s: Object.Image = %T, want the decoded fixture pixels", what, obj.Image)
	}
	gotR, gotG, gotB, gotA := img.At(0, 0).RGBA()
	wantR, wantG, wantB, wantA := want.RGBA()
	if gotR != wantR || gotG != wantG || gotB != wantB || gotA != wantA {
		t.Errorf("%s: Object.Image pixel = rgba(%d,%d,%d,%d), want fixture rgba(%d,%d,%d,%d)",
			what, gotR, gotG, gotB, gotA, wantR, wantG, wantB, wantA)
	}
}

func imageCommands(sess *engine.Session) int {
	n := 0
	for _, cmd := range sess.Paint(1).All() {
		if cmd.Kind == paint.CmdImage {
			n++
		}
	}
	return n
}

// qc:BR-250
func TestLazyImagePaintAfterLoadAssignment(t *testing.T) {
	blue := color.RGBA{B: 255, A: 255}
	png7x9 := solidPNG(t, 7, 9, blue)

	t.Run("after-load assignment", func(t *testing.T) {
		rt, err := js.New(js.Options{URL: "https://example.test/page"})
		if err != nil {
			t.Fatalf("js.New: %v", err)
		}
		fetch := newBlockingFetcher()
		html := `<!DOCTYPE html><html><body><img id="lazy"><script>
setTimeout(function(){
  var i = document.getElementById('lazy');
  i.onload = function(){ window.done = 'yes'; };
  i.src = 'https://example.test/lazy.png';
}, 0);
</script></body></html>`
		sess := lazyImageSession(t, rt, fetch.fetch, html)

		fetch.waitForURL(t, sess, "https://example.test/lazy.png")
		fetch.release <- fetchResult{data: png7x9}
		pumpUntilSet(t, sess, rt, "window.done")

		obj := laidOutImage(t, sess, "lazy")
		checkFixturePixels(t, obj, blue, "after-load assignment")
		if obj.W != 7 || obj.H != 9 {
			t.Errorf("after-load assignment: box = %vx%v, want 7x9 (intrinsic size from the script-assigned source)", obj.W, obj.H)
		}
	})

	t.Run("during-parse control", func(t *testing.T) {
		rt, err := js.New(js.Options{URL: "https://example.test/page"})
		if err != nil {
			t.Fatalf("js.New: %v", err)
		}
		html := `<!DOCTYPE html><html><body><img id="eager"><script>
document.getElementById('eager').src = '` + dataURL(png7x9) + `';
</script></body></html>`
		sess := lazyImageSession(t, rt, nil, html)

		obj := laidOutImage(t, sess, "eager")
		checkFixturePixels(t, obj, blue, "during-parse control")
		if obj.W != 7 || obj.H != 9 {
			t.Errorf("during-parse control: box = %vx%v, want 7x9 (intrinsic size, no pump)", obj.W, obj.H)
		}
	})
}

// qc:BR-251
func TestLazyImageLayoutBuildsOncePerTurn(t *testing.T) {
	blue := color.RGBA{B: 255, A: 255}
	green := color.RGBA{G: 255, A: 255}
	png7x9 := solidPNG(t, 7, 9, blue)
	png10x4 := solidPNG(t, 10, 4, green)

	rt, err := js.New(js.Options{URL: "https://example.test/page"})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	fetch := newBlockingFetcher()
	html := `<!DOCTYPE html><html><body><img id="lazy"><script>
var i = document.getElementById('lazy');
setTimeout(function(){
  i.onload = function(){ window.done = 'yes'; };
  i.src = 'https://example.test/lazy.png';
}, 0);
</script></body></html>`
	sess := lazyImageSession(t, rt, fetch.fetch, html)

	fetch.waitForURL(t, sess, "https://example.test/lazy.png")
	fetch.release <- fetchResult{data: png7x9}
	pumpUntilSet(t, sess, rt, "window.done")
	if obj := laidOutImage(t, sess, "lazy"); obj.Image == nil {
		t.Fatalf("setup: the first assignment did not attach (Image=%T)", obj.Image)
	}

	if _, err := rt.Evaluate(`setTimeout(function(){
  i.onload = function(){ window.second = 'yes'; };
  i.src = 'https://example.test/second.png';
}, 0);`); err != nil {
		t.Fatalf("schedule second assignment: %v", err)
	}
	fetch.waitForURL(t, sess, "https://example.test/second.png")
	fetch.release <- fetchResult{data: png10x4}
	before, after := pumpUntilSet(t, sess, rt, "window.second")

	obj := laidOutImage(t, sess, "lazy")
	checkFixturePixels(t, obj, green, "second (10x4) assignment")
	if obj.W != 10 || obj.H != 4 {
		t.Errorf("second assignment: box in the onload turn = %vx%v, want 10x4 (the arena was not rebuilt)", obj.W, obj.H)
	}
	if d := after.LayoutBuilds - before.LayoutBuilds; d != 1 {
		t.Errorf("second assignment: LayoutBuilds delta in the onload turn = %d, want exactly 1 (no double layout, no extra tick)", d)
	}
}

// qc:BR-252 BR-171
func TestLazyImageStalePixelsStopPainting(t *testing.T) {
	blue := color.RGBA{B: 255, A: 255}
	green := color.RGBA{G: 255, A: 255}
	png7x9 := solidPNG(t, 7, 9, blue)
	png10x4 := solidPNG(t, 10, 4, green)

	rt, err := js.New(js.Options{URL: "https://example.test/page"})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	fetch := newBlockingFetcher()
	html := `<!DOCTYPE html><html><body><img id="lazy"><script>
var i = document.getElementById('lazy');
setTimeout(function(){
  i.onload = function(){ window.done = 'yes'; };
  i.src = 'https://example.test/lazy.png';
}, 0);
</script></body></html>`
	sess := lazyImageSession(t, rt, fetch.fetch, html)

	fetch.waitForURL(t, sess, "https://example.test/lazy.png")
	fetch.release <- fetchResult{data: png7x9}
	pumpUntilSet(t, sess, rt, "window.done")
	if obj := laidOutImage(t, sess, "lazy"); obj.Image == nil || obj.W != 7 || obj.H != 9 {
		t.Fatalf("setup: first assignment did not attach (Image=%T box=%vx%v)", obj.Image, obj.W, obj.H)
	}

	if _, err := rt.Evaluate(`setTimeout(function(){
  i.onerror = function(){ window.failed = 'yes'; };
  i.src = 'https://example.test/reject.png';
}, 0);`); err != nil {
		t.Fatalf("schedule failing assignment: %v", err)
	}
	fetch.waitForURL(t, sess, "https://example.test/reject.png")
	fetch.release <- fetchResult{err: errors.New("fixture rejects this source")}
	pumpUntilSet(t, sess, rt, "window.failed")
	obj := laidOutImage(t, sess, "lazy")
	if obj.Image != nil {
		t.Errorf("failed assignment: Object.Image = %T, want nil (stale pixels must be dropped)", obj.Image)
	}
	if obj.W != 0 || obj.H != 0 {
		t.Errorf("failed assignment: box = %vx%v, want 0x0 (intrinsic size cleared)", obj.W, obj.H)
	}
	if n := imageCommands(sess); n != 0 {
		t.Errorf("failed assignment: painted %d image commands, want 0 (document has no other image source)", n)
	}

	if _, err := rt.Evaluate(`setTimeout(function(){
  i.onload = function(){ window.again = 'yes'; };
  i.src = 'https://example.test/again.png';
}, 0);`); err != nil {
		t.Fatalf("schedule recovery assignment: %v", err)
	}
	fetch.waitForURL(t, sess, "https://example.test/again.png")
	fetch.release <- fetchResult{data: png10x4}
	pumpUntilSet(t, sess, rt, "window.again")
	obj = laidOutImage(t, sess, "lazy")
	checkFixturePixels(t, obj, green, "re-assignment after a failure")
	if obj.W != 10 || obj.H != 4 {
		t.Errorf("re-assignment after a failure: box = %vx%v, want 10x4", obj.W, obj.H)
	}

	if _, err := rt.Evaluate(`setTimeout(function(){
  i.onerror = null;
  i.src = '';
  window.emptied = 'yes';
}, 0);`); err != nil {
		t.Fatalf("schedule empty assignment: %v", err)
	}
	pumpUntilSet(t, sess, rt, "window.emptied")
	obj = laidOutImage(t, sess, "lazy")
	if obj.Image != nil {
		t.Errorf("src='': Object.Image = %T, want nil (empty source must drop the retained pixels)", obj.Image)
	}
	if obj.W != 0 || obj.H != 0 {
		t.Errorf("src='': box = %vx%v, want 0x0 (intrinsic size cleared)", obj.W, obj.H)
	}
	if n := imageCommands(sess); n != 0 {
		t.Errorf("src='': painted %d image commands, want 0 (document has no other image source)", n)
	}
}
