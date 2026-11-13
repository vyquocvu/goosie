package main

import (
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/surface"
)

// GOOSIE-FADESETTLE-001: the screenshot run wrote the page as it was before
// the page had shown itself. Measured before the fix on /tmp/fade/reveal.html
// (a 400x300 display:none box revealed by `setTimeout(..., 200)`, 600x400 at
// DPR 1): goosie -screenshot produced 1 distinct colour and 0 ink in 0.258s,
// while Chromium 131.0.6778.33 at load+1s produces 2 colours and 120,000 ink.
// Nothing the driver consulted could see the debt - every tile was drawn,
// nothing was on order, and no callback had repainted because no callback had
// run - so the run decided it was idle 10 vsyncs before the reveal.
//
// The fix asks the document when it next owes itself a timer it queued during
// its own load, and will not call a frame settled while one is due inside
// steadyRevealHorizon. The oracles below pin both halves: the reveal is
// captured, and the wait stays bounded, because a screenshot sweep that waits
// for every pending timer would wait for gov.vn's 24-hour one.

func revealDriver(reveal func() (time.Duration, bool)) *steadyDriver {
	f := &framePath{window: &stubWindow{events: make(chan surface.Event)}}
	d := newSteadyDriver(f, 8)
	d.watchIdle(func() bool { return true })
	d.watchProgress(func() (int64, int64) { return 10, 0 })
	d.watchReveal(reveal)
	return d
}

func tickReady(d *steadyDriver, started time.Time) bool {
	d.mu.Lock()
	d.sent++
	d.mu.Unlock()
	return d.ready(started)
}

// Startup work due inside the horizon holds the run, and the frames
// it spends waiting are the frames the reveal is drawn in.
//
// qc:BR-283
func TestSteadyWaitsForStartupTimer(t *testing.T) {
	d := revealDriver(func() (time.Duration, bool) { return 200 * time.Millisecond, true })
	started := time.Now()
	for i := 0; i < 40; i++ {
		if tickReady(d, started) {
			t.Fatalf("the run settled at frame %d while a 200ms startup timer was pending", d.sent)
		}
	}
	// The timer fires and the pump repaints; from there the ordinary two idle
	// frames rule decides, so the first quiet frame is not enough.
	d.watchReveal(func() (time.Duration, bool) { return 0, false })
	if tickReady(d, started) {
		t.Fatal("the run settled on the first idle frame after the reveal")
	}
	if !tickReady(d, started) {
		t.Fatalf("the run did not settle on the second idle frame, settled=%d", d.settled)
	}
}

// The other half of the rule, and the reason a sweep does not slow
// down for pages that boot polling timers. 600ms is past the horizon, so the
// run settles on the floor and two idle frames, exactly as it did before this
// existed.
//
// qc:BR-283
func TestSteadyDoesNotWaitForLateStartupTimer(t *testing.T) {
	d := revealDriver(func() (time.Duration, bool) { return 600 * time.Millisecond, true })
	started := time.Now()
	for i := 0; i < 7; i++ {
		if tickReady(d, started) {
			t.Fatalf("the run ended before the floor, at frame %d", d.sent)
		}
	}
	if tickReady(d, started) {
		t.Fatalf("the run settled at frame %d on one idle frame", d.sent)
	}
	if !tickReady(d, started) {
		t.Fatalf("a timer past the horizon held the run, settled=%d sent=%d", d.settled, d.sent)
	}
}

// The wait itself is capped at one horizon, so a page that keeps
// queueing zero-delay startup tasks costs the sweep half a second rather than
// running to the deadline.
//
// qc:BR-283
func TestSteadyRevealWaitSpendsOnlyOneHorizon(t *testing.T) {
	d := revealDriver(func() (time.Duration, bool) { return time.Millisecond, true })
	started := time.Now()
	for i := 0; i < 10; i++ {
		if tickReady(d, started) {
			t.Fatalf("the run settled before it had spent the reveal budget, at frame %d", d.sent)
		}
	}
	d.mu.Lock()
	d.revealStart = time.Now().Add(-steadyRevealHorizon - time.Millisecond)
	d.mu.Unlock()
	if tickReady(d, started) {
		t.Fatal("the run settled on one idle frame once the budget was spent")
	}
	if !tickReady(d, started) {
		t.Fatal("the run refused to settle after the reveal budget was spent")
	}
}

// A startup timer enters the horizon as the run approaches it, and
// by then the run may already have counted an idle frame. Waiting has to
// restart the settle count, because the frame the run would have written is
// not the frame the reveal paints.
//
// qc:BR-283
func TestSteadyRevealEnteringTheHorizonRestartsTheSettleCount(t *testing.T) {
	due, pending := 900*time.Millisecond, true
	d := revealDriver(func() (time.Duration, bool) { return due, pending })
	started := time.Now()
	for i := 0; i < 8; i++ {
		if tickReady(d, started) {
			t.Fatalf("the run settled at frame %d on a page still 900ms from its reveal", d.sent)
		}
	}
	if d.settled != 1 {
		t.Fatalf("settled = %d after the floor and one quiet frame, want 1", d.settled)
	}
	due = 200 * time.Millisecond
	if tickReady(d, started) {
		t.Fatal("the run wrote its frame the moment the reveal entered the horizon")
	}
	if d.settled != 0 {
		t.Fatalf("settled = %d during the wait, want the count restarted", d.settled)
	}
	pending = false // the callback ran, so the document reports no startup debt
	if tickReady(d, started) {
		t.Fatal("the run settled on the first idle frame after the reveal")
	}
	if !tickReady(d, started) {
		t.Fatalf("the run did not settle on the second idle frame after the reveal, settled=%d", d.settled)
	}
}

func writeRevealPage(t *testing.T, delayMS int) string {
	t.Helper()
	page := `<!doctype html><html><head><meta charset="utf-8"></head><body style="margin:0">` +
		`<div id="d" style="display:none"><div style="width:400px;height:300px;margin:20px;background:rgb(200,0,0)"></div></div>` +
		`<script>setTimeout(function(){document.getElementById('d').style.display='block'}, ` + strconv.Itoa(delayMS) + `);</script>` +
		`</body></html>`
	path := filepath.Join(t.TempDir(), "reveal.html")
	if err := os.WriteFile(path, []byte(page), 0o600); err != nil {
		t.Fatalf("write page: %v", err)
	}
	return "file://" + path
}

// ink counts the non-white pixels of the PNG the screenshot run wrote, which
// is the whole question this defect is about: was the page captured as it
// looks once its own queued work has run?
func ink(t *testing.T, path string) int {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open screenshot: %v", err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode screenshot: %v", err)
	}
	n := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bb, a := img.At(x, y).RGBA()
			if a != 0 && (r != 0xFFFF || g != 0xFFFF || bb != 0xFFFF) {
				n++
			}
		}
	}
	return n
}

// The shipped flag path, on the measured fixture. A box the page
// reveals 200ms after load has to be in the PNG, as it is in Chromium's at
// load+1s. 200ms rather than 100ms because the delay sweep says so: the
// pre-fix run ended 150 to 190ms after it started, so a 100ms reveal already
// landed by luck (measured: HEAD r0/r10/r50/r100 ink 120,000,
// r200/r400/r800 ink 0). Only past that window does the missing signal show.
//
// qc:BR-283
func TestScreenshotCapturesStartupReveal(t *testing.T) {
	out := filepath.Join(t.TempDir(), "reveal.png")
	if err := run([]string{"-screenshot", "-url", writeRevealPage(t, 200), "-out", out,
		"-backend", "headless", "-width", "600", "-height", "400", "-dpr", "1"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := ink(t, out); got < 100_000 {
		t.Fatalf("ink = %d px, want the revealed 400x300 box (120,000 px measured in Chromium)", got)
	}
}

// And the bound is real. A reveal 3s out is past the horizon, so
// the run ends without it rather than holding the sweep, exactly as it did
// before this rule existed.
//
// qc:BR-283
func TestScreenshotDoesNotWaitForLateReveal(t *testing.T) {
	out := filepath.Join(t.TempDir(), "late.png")
	if err := run([]string{"-screenshot", "-url", writeRevealPage(t, 3000), "-out", out,
		"-backend", "headless", "-width", "600", "-height", "400", "-dpr", "1"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := ink(t, out); got != 0 {
		t.Fatalf("ink = %d px, want a blank page captured before a 3s reveal", got)
	}
}

// A page with no startup timers at all must not pay for the rule.
//
// qc:BR-283
func TestScreenshotOfStaticPageIsUnaffected(t *testing.T) {
	out := filepath.Join(t.TempDir(), "static.png")
	page := `<!doctype html><html><body style="margin:0"><div style="width:400px;height:300px;margin:20px;background:rgb(200,0,0)"></div></body></html>`
	path := filepath.Join(t.TempDir(), "static.html")
	if err := os.WriteFile(path, []byte(page), 0o600); err != nil {
		t.Fatalf("write page: %v", err)
	}
	if err := run([]string{"-screenshot", "-url", "file://" + path, "-out", out,
		"-backend", "headless", "-width", "600", "-height", "400", "-dpr", "1"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := ink(t, out); got < 100_000 {
		t.Fatalf("ink = %d px, want the 120,000 px box the page paints without any JS", got)
	}
}
