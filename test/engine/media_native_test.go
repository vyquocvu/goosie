package engine_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/media"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/platform/darwin"
	"github.com/vyquocvu/goosie/internal/platform/headless"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/surface"
)

// This file is the only place the whole stack meets: a real HTTP server, the
// engine's real download pipeline and controller, the real AVFoundation decoder,
// and the real paint output. Everything else in the media tests drives one of
// those against a fake for the others, which is how it should be — but a fake on
// every side of a boundary can agree with itself and still be wrong about the
// thing in the middle.
//
// It needs no display. AVPlayer owns the audio and the clock and hands pixels
// over instead of drawing them, so the only thing the process must supply is a
// main-thread run loop, which TestMain below does the way the browser's own
// GoosieAppRun does.
//
// There is no build tag here on purpose. A tag would take this file out of a
// CGO_ENABLED=0 build entirely, and a suite that never runs is a suite that
// reports green; instead the tests compile everywhere and skip where there is no
// decoder, which is the one report a reader can trust. cgo stays confined to
// internal/platform/darwin, which is where the archtest requires it.

// nativeBackend returns the real decoder, or skips the test on a build that has
// none.
func nativeBackend(t *testing.T) media.Backend {
	t.Helper()
	b := darwin.NewMediaBackend()
	if b == nil {
		t.Skip("this build has no native media backend, so nothing here can be decoded")
	}
	return b
}

// TestMain pins the main goroutine to the process main thread and services that
// thread's run loop while the tests run. AVPlayer will not prepare its item on any
// other thread: a worker with its own CFRunLoop turns the loop faithfully and the
// item's status still never leaves Unknown, with no error to explain it.
//
// A build with no backend gets the plain runner. Pumping a run loop that has no
// shim behind it would spin the default case at full speed for the whole suite.
func TestMain(m *testing.M) {
	if darwin.NewMediaBackend() == nil {
		os.Exit(m.Run())
	}
	runtime.LockOSThread()
	code := 1
	done := make(chan struct{})
	go func() {
		defer close(done)
		code = m.Run()
	}()
	for {
		select {
		case <-done:
			os.Exit(code)
		default:
		}
		darwin.PumpMainRunLoopForTest(20 * time.Millisecond)
	}
}

// nativeMediaSession serves a real MP4 over HTTP and builds a session that plays
// it through the real backend. It returns the session, the element's controller
// and the server, with everything cleaned up on test end.
func nativeMediaSession(t *testing.T, extra ...engine.Option) (*engine.Session, media.Controller, *httptest.Server) {
	t.Helper()
	backend := nativeBackend(t)
	clip, err := os.ReadFile(filepath.Join("..", "..", "testdata", "media", "clip.mp4"))
	if err != nil {
		t.Skipf("the generated fixture is missing (%v); run the ffmpeg step in testdata/media", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><video id="v" src="clip.mp4" controls width="320" height="180"></video></body></html>`))
	})
	mux.HandleFunc("/clip.mp4", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write(clip)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := net.DefaultClient()
	t.Cleanup(func() { _ = client.Close() })
	fetch := func(ctx context.Context, url string) (engine.MediaResponse, error) {
		resp, err := client.Get(ctx, url)
		if err != nil {
			return engine.MediaResponse{}, err
		}
		return engine.MediaResponse{StatusCode: resp.StatusCode, Body: resp.Body, URL: resp.URL}, nil
	}

	opts := []engine.Option{
		engine.WithMetrics(&fixedMetrics{advance: 7}),
		engine.WithMedia(backend, fetch, srv.URL+"/"),
	}
	sess, err := engine.NewSession(
		`<html><body><video id="v" src="clip.mp4" controls width="320" height="180"></video></body></html>`,
		nil, 800, append(opts, extra...)...)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(sess.Close)

	c := sess.MediaController(videoNode(t, sess.Doc))
	if c == nil {
		t.Fatal("the document produced no media controller")
	}
	return sess, c, srv
}

// pumpNative polls the session until want holds. It allows longer than the fake
// backend's tests because a real decoder has to read a file and initialise a
// hardware or software pipeline before the first frame exists.
func pumpNative(t *testing.T, sess *engine.Session, what string, want func() bool) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		sess.PumpMedia()
		if want() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%s never happened", what)
}

// TestNativeEndToEndPlaysARealVideo is the acceptance test for the whole feature,
// short of a human looking at a screen: a page loads a real MP4 over HTTP, the
// browser downloads it, AVFoundation decodes it, and a real decoded frame reaches
// the display list that the compositor presents.
func TestNativeEndToEndPlaysARealVideo(t *testing.T) {
	sess, c, _ := nativeMediaSession(t)

	// The src attribute was recorded at construction and nothing was fetched for
	// it: this engine ignores preload hints.
	if got := c.Snapshot().State; got != media.StateEmpty {
		t.Fatalf("state before any request = %v, want StateEmpty", got)
	}
	if got := c.CurrentSrc(); got != "" {
		t.Fatalf("currentSrc before loading = %q, want empty", got)
	}

	// A gesture, then play — which is what a press on the painted control does.
	sess.SetMediaActivation(true)
	op, err := c.Play()
	sess.SetMediaActivation(false)
	if err != nil {
		t.Fatalf("Play: %v", err)
	}

	pumpNative(t, sess, "metadata from the real decoder", func() bool {
		s := c.Snapshot()
		return !media.UnknownDuration(s.Duration) && s.Width > 0
	})
	snap := c.Snapshot()
	if snap.Width != 320 || snap.Height != 180 {
		t.Fatalf("decoded size = %dx%d, want 320x180", snap.Width, snap.Height)
	}
	if snap.Duration < 1.5 || snap.Duration > 2.5 {
		t.Fatalf("decoded duration = %v, want about 2s", snap.Duration)
	}
	if got := c.CurrentSrc(); got == "" {
		t.Fatal("currentSrc is empty once a source is loading")
	}

	pumpNative(t, sess, "playback", func() bool {
		return c.Snapshot().State == media.StatePlaying
	})
	// The play request settles only when playback has actually begun, which is
	// the whole reason play() is not a resolved promise handed back on success.
	var resolved bool
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && !resolved {
		sess.PumpMedia()
		for _, e := range c.Events() {
			if e.Kind == media.EvPlayResolved && e.Op == op {
				resolved = true
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !resolved {
		t.Fatal("the play request never settled after playback started")
	}

	// A real decoded frame, through the real paint path, into a real display list.
	var view paint.VideoView
	pumpNative(t, sess, "a decoded frame", func() bool {
		v, ok := sess.MediaView(videoNode(t, sess.Doc))
		if ok {
			view = v
		}
		return ok && v.HasFrame()
	})
	img, ok := view.Pixels.(*image.RGBA)
	if !ok {
		t.Fatalf("the painted frame is a %T, want *image.RGBA", view.Pixels)
	}
	if img.Bounds().Dx() != 320 || img.Bounds().Dy() != 180 {
		t.Fatalf("the painted frame is %v, want 320x180", img.Bounds())
	}
	if view.State != paint.VideoPlaying {
		t.Fatalf("the painted view reports %v, want VideoPlaying", view.State)
	}
	if !opaqueEnough(img) {
		t.Fatal("the decoded frame is transparent: the premultiplied RGBA conversion lost the alpha")
	}

	dl, err := sess.PaintChecked(1.0)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	var found bool
	for _, cmd := range dl.Build(1).All() {
		if cmd.Kind == paint.CmdImage && cmd.Image.Src == view.Pixels {
			found = true
		}
	}
	if !found {
		t.Fatal("the decoded frame never reached the display list")
	}

	// The position advances on its own, which is what distinguishes a video that
	// is playing from one that decoded a single frame and stopped.
	first := c.Snapshot().Position
	pumpNative(t, sess, "the position to advance", func() bool {
		return c.Snapshot().Position > first+0.05
	})

	// Pause, then seek: the two things a user does next.
	c.Pause()
	pumpNative(t, sess, "the pause", func() bool { return c.Snapshot().State == media.StatePaused })
	before := c.Snapshot().Seq
	c.Seek(1.5)
	pumpNative(t, sess, "the seek to settle", func() bool {
		s := c.Snapshot()
		return s.Position > 1.3 && s.Position < 1.7 && s.Seq > before
	})

	// Teardown stops the player and removes the file it was handed.
	sess.Close()
	if got := c.Snapshot().State; got != media.StateClosed {
		t.Fatalf("state after Close = %v, want StateClosed", got)
	}
}

// TestNativeEndToEndLeavesNothingBehind pins the cleanup half of the same journey:
// a document that goes away takes its decoder, its temporary directory and its
// audio with it.
func TestNativeEndToEndLeavesNothingBehind(t *testing.T) {
	sess, c, _ := nativeMediaSession(t)
	sess.SetMediaActivation(true)
	if _, err := c.Play(); err != nil {
		t.Fatalf("Play: %v", err)
	}
	sess.SetMediaActivation(false)
	pumpNative(t, sess, "playback", func() bool { return c.Snapshot().State == media.StatePlaying })

	dirs, err := filepath.Glob(filepath.Join(os.TempDir(), "goosie-media-*"))
	if err != nil {
		t.Fatalf("glob the media directories: %v", err)
	}
	if len(dirs) == 0 {
		t.Fatal("no media directory exists while a video is playing")
	}

	sess.Close()
	for _, d := range dirs {
		if _, err := os.Stat(d); err == nil {
			entries, _ := os.ReadDir(d)
			t.Fatalf("Session.Close left %s behind with %d entries", d, len(entries))
		}
	}
}

// opaqueEnough reports whether a decoded frame has visible content. The test
// pattern is bright and busy, so a frame that came through the BGRA conversion
// with its alpha zeroed fails here — which is the mistake worth catching, because
// a fully transparent frame composites to nothing and looks exactly like a video
// that never started.
func opaqueEnough(img *image.RGBA) bool {
	opaque := 0
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] > 0 {
			opaque++
		}
	}
	return opaque*4 >= len(img.Pix)/4
}

// refFrames is testdata/media/ref-frames.json: what the clip's pixels are,
// measured by a decoder that is not this one.
type refFrames struct {
	Grid      [2]int                  `json:"grid"`
	Tolerance float64                 `json:"tolerancePerChannel"`
	Frames    map[string][][3]float64 `json:"frames"`
}

// blockMeans reduces a frame to the mean R, G and B of each cell of a cols x rows
// grid, row-major. A block mean rather than a per-pixel diff because the question
// is whether AVFoundation decoded the right picture, and a mean survives the
// one-or-two-unit rounding a different YUV matrix produces while a per-pixel
// comparison would not. The frames are premultiplied, which is the same as
// straight RGB here: the clip is fully opaque, and opaqueEnough is what proves it.
func blockMeans(img *image.RGBA, cols, rows int) [][3]float64 {
	b := img.Bounds()
	bw, bh := b.Dx()/cols, b.Dy()/rows
	out := make([][3]float64, 0, cols*rows)
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			var sum [3]float64
			n := 0
			for y := b.Min.Y + r*bh; y < b.Min.Y+(r+1)*bh; y++ {
				for x := b.Min.X + c*bw; x < b.Min.X+(c+1)*bw; x++ {
					i := img.PixOffset(x, y)
					sum[0] += float64(img.Pix[i])
					sum[1] += float64(img.Pix[i+1])
					sum[2] += float64(img.Pix[i+2])
					n++
				}
			}
			out = append(out, [3]float64{sum[0] / float64(n), sum[1] / float64(n), sum[2] / float64(n)})
		}
	}
	return out
}

// settleFrames pumps until the decoder has stopped handing samples over, and
// returns the frame sequence it settled on.
//
// The quiet period is the point, not the return value. A sample that was already
// in flight when the pause landed increments Seq *after* the seek is issued, so a
// wait for "Seq changed" is satisfied by the pre-seek picture and the test reads
// the frame the player was showing a moment ago rather than the one it seeked to.
// Nothing can be in flight once Seq has held still, and after that a change means
// the seek's own frame arrived.
func settleFrames(t *testing.T, sess *engine.Session, c media.Controller) uint64 {
	t.Helper()
	const quietPolls = 10
	stable, last := 0, c.Snapshot().Seq
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && stable < quietPolls {
		sess.PumpMedia()
		if s := c.Snapshot().Seq; s == last {
			stable++
		} else {
			last, stable = s, 0
		}
		time.Sleep(10 * time.Millisecond)
	}
	if stable < quietPolls {
		t.Fatalf("frames were still arriving while the player was paused (seq %d)", last)
	}
	return last
}

// TestNativeFramesMatchTheSourceDecode is the visual-comparison half of the
// acceptance evidence, and it is the one test here that can be wrong about
// colour rather than about plumbing. It seeks a paused player to two timestamps
// and compares the frame AVFoundation handed over against a decode of the same
// bitstream taken with ffmpeg, cell by cell.
//
// The seek is frame-accurate on both sides — the native player passes
// kCMTimeZero for both tolerances — so there is no timing slack to hide behind
// and the only allowance is the colour one the fixture declares.
func TestNativeFramesMatchTheSourceDecode(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "media", "ref-frames.json"))
	if err != nil {
		t.Skipf("no reference fixture (%v)", err)
	}
	var ref refFrames
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatalf("read the reference fixture: %v", err)
	}
	if ref.Grid[0] <= 0 || ref.Grid[1] <= 0 || ref.Tolerance <= 0 {
		t.Fatalf("the reference fixture is not usable as written: %+v", ref)
	}

	sess, c, _ := nativeMediaSession(t)
	sess.SetMediaActivation(true)
	if _, err := c.Play(); err != nil {
		t.Fatalf("Play: %v", err)
	}
	sess.SetMediaActivation(false)
	pumpNative(t, sess, "playback", func() bool { return c.Snapshot().State == media.StatePlaying })
	c.Pause()
	pumpNative(t, sess, "the pause", func() bool { return c.Snapshot().State == media.StatePaused })

	timestamps := make([]string, 0, len(ref.Frames))
	for ts := range ref.Frames {
		timestamps = append(timestamps, ts)
	}
	sort.Strings(timestamps)
	if len(timestamps) == 0 {
		t.Fatal("the reference fixture names no frames to compare")
	}
	for _, ts := range timestamps {
		sec, err := strconv.ParseFloat(ts, 64)
		if err != nil {
			t.Fatalf("the fixture names %q, which is not a timestamp: %v", ts, err)
		}
		want := ref.Frames[ts]

		before := settleFrames(t, sess, c)
		c.Seek(sec)
		pumpNative(t, sess, "the seek to "+ts, func() bool {
			s := c.Snapshot()
			return s.Seq > before && math.Abs(s.Position-sec) < 0.05
		})

		v, ok := sess.MediaView(videoNode(t, sess.Doc))
		if !ok || !v.HasFrame() {
			t.Fatalf("seeking to %s produced no frame to compare", ts)
		}
		img, ok := v.Pixels.(*image.RGBA)
		if !ok {
			t.Fatalf("the frame at %s is a %T, want *image.RGBA", ts, v.Pixels)
		}
		if img.Bounds().Dx()/ref.Grid[0] < 1 || img.Bounds().Dy()/ref.Grid[1] < 1 {
			t.Fatalf("the frame at %s is %v, too small for a %dx%d grid", ts, img.Bounds(), ref.Grid[0], ref.Grid[1])
		}
		got := blockMeans(img, ref.Grid[0], ref.Grid[1])
		if len(got) != len(want) {
			t.Fatalf("the frame at %s split into %d cells, the fixture has %d", ts, len(got), len(want))
		}
		if dir := os.Getenv("GOOSIE_MEDIA_DUMP"); dir != "" {
			p := filepath.Join(dir, "goosie-"+ts+".png")
			f, err := os.Create(p)
			if err != nil {
				t.Fatalf("create %s: %v", p, err)
			}
			if err := png.Encode(f, img); err != nil {
				f.Close()
				t.Fatalf("encode %s: %v", p, err)
			}
			if err := f.Close(); err != nil {
				t.Fatalf("close %s: %v", p, err)
			}
			t.Logf("t=%s: wrote %s", ts, p)
		}

		var worst float64
		var where string
		for i := range want {
			for k := 0; k < 3; k++ {
				if d := math.Abs(got[i][k] - want[i][k]); d > worst {
					worst = d
					where = fmt.Sprintf("cell %d channel %d: decoded %.1f, reference %.1f", i, k, got[i][k], want[i][k])
				}
			}
		}
		t.Logf("t=%s: worst channel error %.2f of 255, tolerance %.1f", ts, worst, ref.Tolerance)
		if worst > ref.Tolerance {
			t.Errorf("the frame at t=%s is not the frame the clip contains: %s (worst error %.2f, tolerance %.1f)",
				ts, where, worst, ref.Tolerance)
		}
	}
}

// TestNativeDumpCompositedPage writes the whole page — a real decoded frame in
// the video, the control bar over it — to $GOOSIE_MEDIA_DUMP/goosie-page.png.
// It is an inspection tool rather than an assertion: the assertions about the
// same pixels live in TestNativeFramesMatchTheSourceDecode, and this is the one
// place the frame can be seen in its element instead of only measured. Skipped
// unless the variable is set, so a plain test run never spends a compositor on a
// picture nobody asked for.
//
// The wiring is cmd/goosie-headless's — the real tile rasterizer, composer and
// present path against a headless window — which is as close to the pixels a
// display would show as a process without one can get.
func TestNativeDumpCompositedPage(t *testing.T) {
	dir := os.Getenv("GOOSIE_MEDIA_DUMP")
	if dir == "" {
		t.Skip("set GOOSIE_MEDIA_DUMP to a directory to write the composited page")
	}
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("init fonts: %v", err)
	}

	sess, c, _ := nativeMediaSession(t, engine.WithMetrics(fonts))
	sess.SetMediaActivation(true)
	if _, err := c.Play(); err != nil {
		t.Fatalf("Play: %v", err)
	}
	sess.SetMediaActivation(false)
	pumpNative(t, sess, "playback", func() bool { return c.Snapshot().State == media.StatePlaying })
	c.Pause()
	pumpNative(t, sess, "the pause", func() bool { return c.Snapshot().State == media.StatePaused })
	before := settleFrames(t, sess, c)
	c.Seek(0.5)
	pumpNative(t, sess, "the seek to 0.5", func() bool {
		s := c.Snapshot()
		return s.Seq > before && math.Abs(s.Position-0.5) < 0.05
	})

	// The frame reaches the layout arena on a reflow, which is where the
	// browser's own frame picks it up between paints.
	if err := sess.Reflow(800); err != nil {
		t.Fatalf("Reflow: %v", err)
	}
	dumpCompositedPage(t, sess, fonts, filepath.Join(dir, "goosie-page.png"))

	// The muted mark is drawn geometry too, and it is only ever visible on a
	// muted player, so the dump takes one of each.
	c.SetMuted(true)
	pumpNative(t, sess, "the mute", func() bool { return c.Snapshot().Muted })
	if err := sess.Reflow(800); err != nil {
		t.Fatalf("Reflow: %v", err)
	}
	dumpCompositedPage(t, sess, fonts, filepath.Join(dir, "goosie-page-muted.png"))
}

// dumpCompositedPage renders one presented frame of the session to path.
func dumpCompositedPage(t *testing.T, sess *engine.Session, fonts *raster.Fonts, path string) {
	t.Helper()
	const dpr = 2
	list, err := sess.PaintChecked(dpr)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	dl := list.Build(1)
	extent := dl.Extent()
	budgetTiles, budgetBytes, err := engine.TileCacheBudget(extent)
	if err != nil {
		t.Fatalf("size the tile cache: %v", err)
	}
	pool := frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, budgetTiles)
	layer := frame.NewLayer(1, frame.Rect{X1: extent.X1, Y1: extent.Y1}, budgetBytes, pool)
	layer.SetContent(dl)

	dev := frame.Size{W: 800 * dpr, H: extent.Y1}
	wp := raster.New(raster.DefaultWorkers(), 4096, raster.DefaultRaster(fonts, raster.NewGlyphAtlas(4<<20, fonts)))
	wp.Start(context.Background())
	defer wp.Close()
	sched := raster.NewScheduler(layer, wp, frame.Viewport{Size: dev}, dpr, raster.Pref{})
	sched.SetPlan(frame.FramePlan{Serial: 1, Layers: []*frame.Layer{layer}, Background: sess.BackgroundColor()})

	win := headless.New(headless.Config{Size: dev, Scale: dpr})
	defer win.Close()
	loop := surface.NewLoop(win, sched, surface.NewComposer(dev, pool), frame.NewFrameRecorder(2))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- loop.Run(ctx) }()
	win.Inject(surface.Event{Kind: surface.EvResize, Size: dev, Scale: dpr})

	deadline := time.Now().Add(10 * time.Second)
	for win.Stats().Presents < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("the loop: %v", err)
	}
	if win.Stats().Presents < 2 {
		t.Fatal("the compositor never presented the page")
	}
	if err := win.WritePNG(path); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	t.Logf("wrote %s", path)
}
