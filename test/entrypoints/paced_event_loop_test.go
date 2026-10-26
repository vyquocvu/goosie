package entrypoints

// GOOSIE-PUMP-001: a paced run - a screenshot or a measured gate burst - used
// to load the page, draw it, and never run its event loop, and the script-image
// fetch context died as soon as the load call returned. A page whose timer,
// RAF callback or `new Image()` completion was supposed to change what it looks
// like was therefore captured before any of that could happen.
//
// These oracles drive the real binary over a local server and read the written
// PNG, so they measure the same path an operator runs: BR-253 (zero-delay
// timer), BR-254 (script-assigned image source reaching the network and its
// onload being dispatched) and BR-255 (timer that mutates the DOM).

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	pacedWidth  = 200
	pacedHeight = 150
)

// runPacedScreenshot runs `goosie -screenshot` against url through the headless
// backend and returns the decoded PNG it wrote.
func runPacedScreenshot(t *testing.T, bin, url string) image.Image {
	t.Helper()
	out := filepath.Join(t.TempDir(), "shot.png")
	var stderr bytes.Buffer
	cmd := exec.Command(bin,
		"-backend", "headless",
		"-width", strconv.Itoa(pacedWidth),
		"-height", strconv.Itoa(pacedHeight),
		"-dpr", "1",
		"-screenshot",
		"-out", out,
		"-url", url)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("paced screenshot of %s failed: %v\nstderr: %s", url, err, stderr.String())
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatalf("no screenshot written for %s: %v", url, err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode the screenshot of %s: %v", url, err)
	}
	return img
}

// newPacedPageServer serves the named HTML pages and 404s everything else.
func newPacedPageServer(pages map[string]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := pages[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(body))
	}))
}

// pngPixel reads one pixel of a decoded screenshot.
func pngPixel(t *testing.T, img image.Image, x, y int) color.RGBA {
	t.Helper()
	if !(image.Point{X: x, Y: y}).In(img.Bounds()) {
		t.Fatalf("pixel (%d,%d) is outside the %v screenshot", x, y, img.Bounds())
	}
	r, g, b, a := img.At(x, y).RGBA()
	return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

// assertPixel fails with both colours named. The tolerance absorbs nothing a
// solid fill should not produce, but keeps the assertion about the colour a
// page asked for rather than one encoder rounding.
func assertPixel(t *testing.T, img image.Image, x, y int, want color.RGBA, what string) {
	t.Helper()
	got := pngPixel(t, img, x, y)
	near := func(a, b uint8) bool {
		d := int(a) - int(b)
		return d >= -8 && d <= 8
	}
	if !near(got.R, want.R) || !near(got.G, want.G) || !near(got.B, want.B) {
		t.Fatalf("%s: pixel (%d,%d) = #%02x%02x%02x, want #%02x%02x%02x",
			what, x, y, got.R, got.G, got.B, want.R, want.G, want.B)
	}
}

// BR-253: a zero-delay timer runs before a paced screenshot settles.
//
// The page starts blue and a setTimeout(...,0) callback turns the body
// background red. The PNG the binary writes must hold the post-timer colour;
// the control page, identical without the script, must keep the initial one.
// The sample point is the middle of the viewport, which no box covers: it shows
// the plan's canvas colour, so this also measures that a scripted background
// change republished the plan rather than only re-rasterizing tiles.
//
// qc:BR-253
func TestPacedScreenshotRunsAZeroDelayTimer(t *testing.T) {
	bin := buildBinary(t, "goosie")

	const head = `<!DOCTYPE html><html><body style="margin:0;background:#0000ff"><p>paced timer</p>`
	server := newPacedPageServer(map[string]string{
		"/timer":   head + `<script>setTimeout(function(){document.body.style.background='#ff0000'},0);</script></body></html>`,
		"/control": head + `</body></html>`,
	})
	defer server.Close()

	x, y := pacedWidth/2, pacedHeight/2
	timer := runPacedScreenshot(t, bin, server.URL+"/timer")
	assertPixel(t, timer, x, y, color.RGBA{R: 0xff, A: 0xff},
		"BR-253: the settled paced screenshot after a zero-delay timer sets body background to #ff0000")

	control := runPacedScreenshot(t, bin, server.URL+"/control")
	assertPixel(t, control, x, y, color.RGBA{B: 0xff, A: 0xff},
		"BR-253 control: the settled paced screenshot of a page with no timer keeps its initial #0000ff")
}

// BR-254: a script-assigned image source reaches the network after the load
// call returned, and its onload runs inside the paced run.
//
// The server holds the image response for 50ms. That is deliberate: the old
// code canceled the image fetch context when loadURLCtx returned, so the hold
// guarantees the fetch is still open at that moment and the request is aborted
// unless the context outlives the call. After the hold the handler records
// r.Context().Err(), which is how the server sees the client cancel. The page
// paints green from onload and red from onerror, so a failed dispatch is not
// mistaken for a missing repaint.
//
// qc:BR-254
func TestPacedScreenshotScriptImageLoadOutlivesTheLoadCall(t *testing.T) {
	bin := buildBinary(t, "goosie")

	const hold = 50 * time.Millisecond
	page := `<!DOCTYPE html><html><body style="margin:0;background:#0000ff"><script>
var i = new Image();
i.onload = function(){ document.body.style.background = '#00ff00'; };
i.onerror = function(){ document.body.style.background = '#ff0000'; };
i.src = '/x.png';
</script></body></html>`

	var requests atomic.Int32
	var requestedOnce sync.Once
	requested := make(chan struct{})
	// held carries what the request context looked like when the hold ended.
	held := make(chan error, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(page))
		case "/x.png":
			requests.Add(1)
			requestedOnce.Do(func() { close(requested) })
			select {
			case <-time.After(hold):
			case <-r.Context().Done():
			}
			held <- r.Context().Err()
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(tinyPNG(t))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	img := runPacedScreenshot(t, bin, server.URL+"/")

	select {
	case <-requested:
	default:
		t.Fatalf("BR-254: the local server never recorded GET /x.png during the paced screenshot run")
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("BR-254: the local server recorded GET /x.png %d times, want 1", got)
	}
	select {
	case err := <-held:
		if err != nil {
			t.Fatalf("BR-254: the image request was canceled while the server held it for %v (%v): the script-image fetch did not outlive the load call", hold, err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("BR-254: the image handler never finished")
	}

	x, y := pacedWidth/2, pacedHeight/2
	if got := pngPixel(t, img, x, y); got.R > 0xc0 && got.G < 0x40 && got.B < 0x40 {
		t.Fatalf("BR-254: pixel (%d,%d) = #%02x%02x%02x: the image onerror callback ran, so the fetch failed after reaching the network", x, y, got.R, got.G, got.B)
	}
	assertPixel(t, img, x, y, color.RGBA{G: 0xff, A: 0xff},
		"BR-254: the settled paced screenshot after the script image's onload sets body background to #00ff00")
}

// BR-255: a timer that mutates the DOM in a paced run is repainted.
//
// The page starts white and a setTimeout(...,0) callback appends a 120x80 green
// block at the top-left. The settled PNG must show the block; the control page
// must not.
//
// qc:BR-255
func TestPacedScreenshotRepaintsATimerMutation(t *testing.T) {
	bin := buildBinary(t, "goosie")

	const head = `<!DOCTYPE html><html><body style="margin:0;background:#ffffff">`
	const script = `<script>setTimeout(function(){var d=document.createElement('div');d.style.cssText='width:120px;height:80px;background:#00c000';document.body.appendChild(d);},0);</script>`
	server := newPacedPageServer(map[string]string{
		"/mutate":  head + script + `</body></html>`,
		"/control": head + `</body></html>`,
	})
	defer server.Close()

	mutated := runPacedScreenshot(t, bin, server.URL+"/mutate")
	assertPixel(t, mutated, 60, 40, color.RGBA{G: 0xc0, A: 0xff},
		"BR-255: the settled paced screenshot after a zero-delay timer appended a 120x80 #00c000 block at (0,0)")

	control := runPacedScreenshot(t, bin, server.URL+"/control")
	assertPixel(t, control, 60, 40, color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff},
		"BR-255 control: the settled paced screenshot of a page with no timer stays white where the block would be")
}

// The gate driver is the other paced seam, and it shares the shape: a timer
// cannot prove it the way BR-253 does, because a zero-delay timer fires inside
// the load. A 30ms timer can only fire once the paced loop is running, so a
// fetch its callback issues reaching the local server is proof that the gate
// pumped the page. Without the hook the run draws its frames and exits with the
// server never seeing the request.
func TestGatePacedRunPumpsThePageEventLoop(t *testing.T) {
	bin := buildBinary(t, "goosie")

	pinged := make(chan struct{})
	var once sync.Once
	page := `<!DOCTYPE html><html><body><script>
setTimeout(function(){ fetch('/ping').catch(function(){}); }, 30);
</script></body></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(page))
		case "/ping":
			once.Do(func() { close(pinged) })
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("pong"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	report := filepath.Join(t.TempDir(), "report.json")
	var stderr bytes.Buffer
	cmd := exec.Command(bin,
		"-backend", "headless",
		"-width", strconv.Itoa(pacedWidth),
		"-height", strconv.Itoa(pacedHeight),
		"-dpr", "1",
		"-gate", "-frames", "20",
		"-out", report,
		"-url", server.URL+"/")
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("20-frame gate run failed: %v\nstderr: %s", err, stderr.String())
	}
	select {
	case <-pinged:
	default:
		t.Fatalf("the gate run drew its 20 frames without running the page's 30ms timer: the local server recorded no GET /ping from the timer's fetch callback")
	}
}

// tinyPNG encodes the image the BR-254 server answers with. Its size is
// deliberate but arbitrary: the oracle is about the request and the event, not
// about the decoded dimensions.
func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 7, 9))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 0x20, 0x20, 0xff, 0xff
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode fixture PNG: %v", err)
	}
	return buf.Bytes()
}
