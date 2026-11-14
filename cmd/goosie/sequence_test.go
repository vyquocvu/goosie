package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/surface"
)

const seqStaticPage = `<!doctype html><html><head><meta charset="utf-8"></head><body style="margin:0">` +
	`<div style="width:400px;height:300px;margin:20px;background:rgb(200,0,0)"></div>` +
	`<div style="width:400px;height:60px;margin:20px;background:rgb(0,0,200)"></div>` +
	`</body></html>`

const seqAnimPage = `<!doctype html><html><head><meta charset="utf-8"></head><body style="margin:0">` +
	`<div style="width:400px;height:300px;margin:20px;background:rgb(200,0,0)"></div>` +
	`<div id="b" style="width:400px;height:60px;margin:20px;background:rgb(0,0,200)"></div>` +
	`<script>var n=0;setInterval(function(){n++;document.getElementById('b').style.background=` +
	`'rgb(0,'+(n*40%256)+',0)'},250);</script>` +
	`</body></html>`

var (
	seqStaticBox  = image.Rect(30, 30, 410, 310)
	seqAnimBox    = image.Rect(30, 350, 410, 400)
	seqViewport   = image.Rect(0, 0, 600, 400)
	seqIntervalMS = int64(250)
)

func writeSeqPage(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write page: %v", err)
	}
	return "file://" + path
}

type seqRecord struct {
	URL           string  `json:"url"`
	OS            string  `json:"os"`
	Arch          string  `json:"arch"`
	Backend       string  `json:"backend"`
	ViewportW     int     `json:"viewport_width"`
	ViewportH     int     `json:"viewport_height"`
	DPR           float64 `json:"dpr"`
	Zoom          float64 `json:"zoom"`
	IntervalMS    int64   `json:"interval_ms"`
	ScrollScreens int     `json:"scroll_screens"`
	ScrollPresses int     `json:"scroll_presses"`
	ScrollLanded  *bool   `json:"scroll_landed"`
	Frames        []struct {
		PNG           string `json:"png"`
		MS            int64  `json:"ms"`
		Idle          bool   `json:"idle"`
		TilesNeeded   int    `json:"tiles_needed"`
		PaintingBytes int64  `json:"painting_bytes"`
		Presents      int64  `json:"presents"`
		Frames        int64  `json:"frames"`
		ScrollY       int32  `json:"scroll_y"`
	} `json:"frames"`
}

func captureSeq(t *testing.T, url string, count int, interval time.Duration) (string, seqRecord, []image.Image) {
	t.Helper()
	return captureSeqArgs(t, url, count, interval)
}

func captureSeqArgs(t *testing.T, url string, count int, interval time.Duration, extra ...string) (string, seqRecord, []image.Image) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "seq")
	args := []string{"-screenshot-seq", dir, "-url", url, "-backend", "headless",
		"-width", "600", "-height", "400", "-dpr", "1",
		"-seq-frames", strconv.Itoa(count), "-seq-interval", interval.String()}
	if err := run(append(args, extra...)); err != nil {
		t.Fatalf("run: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "frames.json"))
	if err != nil {
		t.Fatalf("read frames.json: %v", err)
	}
	var rec seqRecord
	if err := json.Unmarshal(body, &rec); err != nil {
		t.Fatalf("decode frames.json %s: %v", body, err)
	}
	var shots []image.Image
	for _, f := range rec.Frames {
		file, err := os.Open(filepath.Join(dir, f.PNG))
		if err != nil {
			t.Fatalf("open %s: %v", f.PNG, err)
		}
		img, err := png.Decode(file)
		file.Close()
		if err != nil {
			t.Fatalf("decode %s: %v", f.PNG, err)
		}
		shots = append(shots, img)
	}
	return dir, rec, shots
}

func changed(a, b image.Image, region image.Rectangle) int {
	n := 0
	for y := region.Min.Y; y < region.Max.Y; y++ {
		for x := region.Min.X; x < region.Max.X; x++ {
			if a.At(x, y) != b.At(x, y) {
				n++
			}
		}
	}
	return n
}

func fileHash(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sum := md5.Sum(body)
	return hex.EncodeToString(sum[:])
}

func hashOf(t *testing.T, dir string, rec seqRecord) []string {
	t.Helper()
	var out []string
	for _, f := range rec.Frames {
		out = append(out, fileHash(t, filepath.Join(dir, f.PNG)))
	}
	return out
}

// qc:BR-085
func TestSequenceOfAStaticPageIsPixelIdentical(t *testing.T) {
	url := writeSeqPage(t, "static.html", seqStaticPage)
	_, rec, shots := captureSeq(t, url, 5, time.Duration(seqIntervalMS)*time.Millisecond)
	if len(shots) != 5 {
		t.Fatalf("captured %d frames, want 5", len(shots))
	}
	for i, shot := range shots {
		if got := changed(shots[0], shot, seqViewport); got != 0 {
			t.Errorf("frame %d differs from frame 0 in %d px of the viewport, want 0", i, got)
		}
		if !rec.Frames[i].Idle || rec.Frames[i].TilesNeeded != 0 || rec.Frames[i].PaintingBytes != 0 {
			t.Errorf("frame %d was not a complete frame: %+v", i, rec.Frames[i])
		}
	}
}

// qc:BR-085
func TestSequenceSpacesItsCapturesByTheInterval(t *testing.T) {
	url := writeSeqPage(t, "static.html", seqStaticPage)
	_, rec, _ := captureSeq(t, url, 4, time.Duration(seqIntervalMS)*time.Millisecond)
	if len(rec.Frames) != 4 {
		t.Fatalf("captured %d frames, want 4", len(rec.Frames))
	}
	for i := 1; i < len(rec.Frames); i++ {
		gap := rec.Frames[i].MS - rec.Frames[i-1].MS
		if gap < seqIntervalMS {
			t.Errorf("capture %d came %dms after capture %d, want at least %dms", i, gap, i-1, seqIntervalMS)
		}
		if prev := rec.Frames[i-1].Frames; rec.Frames[i].Frames <= prev {
			t.Errorf("capture %d drew on frame %d, not after %d: the loop stopped pacing", i, rec.Frames[i].Frames, prev)
		}
	}
}

// qc:BR-085
func TestSequenceSeparatesARepaintingBoxFromAStaticOne(t *testing.T) {
	url := writeSeqPage(t, "anim.html", seqAnimPage)
	dir, rec, shots := captureSeq(t, url, 4, time.Duration(seqIntervalMS)*time.Millisecond)
	if len(shots) != 4 {
		t.Fatalf("captured %d frames, want 4", len(shots))
	}
	for i := 1; i < len(shots); i++ {
		if got := changed(shots[i-1], shots[i], seqAnimBox); got == 0 {
			t.Errorf("the repainting box did not change between captures %d and %d", i-1, i)
		}
		if got := changed(shots[i-1], shots[i], seqStaticBox); got != 0 {
			t.Errorf("the static box changed by %d px between captures %d and %d, want 0", got, i-1, i)
		}
	}
	hashes := hashOf(t, dir, rec)
	if hashes[0] == hashes[1] {
		t.Fatal("the first two captures of a repainting page are the same file: the sequence is not re-reading the window")
	}
}

// qc:BR-085
func TestSequenceRecordNamesTheConditionsItCapturedUnder(t *testing.T) {
	url := writeSeqPage(t, "static.html", seqStaticPage)
	_, rec, _ := captureSeq(t, url, 2, time.Duration(seqIntervalMS)*time.Millisecond)
	if rec.URL != url {
		t.Errorf("url = %q, want %q", rec.URL, url)
	}
	if rec.OS == "" || rec.Arch == "" || rec.Backend != "headless" {
		t.Errorf("environment = %q/%q/%q, want os, arch and backend named", rec.OS, rec.Arch, rec.Backend)
	}
	if rec.ViewportW != 600 || rec.ViewportH != 400 || rec.DPR != 1 || rec.Zoom != 1 {
		t.Errorf("viewport = %dx%d dpr %v zoom %v, want 600x400 dpr 1 zoom 1", rec.ViewportW, rec.ViewportH, rec.DPR, rec.Zoom)
	}
	if rec.IntervalMS != seqIntervalMS {
		t.Errorf("interval = %dms, want %dms", rec.IntervalMS, seqIntervalMS)
	}
	for i, f := range rec.Frames {
		if f.PNG != fmt.Sprintf("frame-%03d.png", i) {
			t.Errorf("capture %d is %q, want the numbered PNG in capture order", i, f.PNG)
		}
	}
}

// qc:BR-085
func TestSequenceHoldsOnlyAfterAFrameWorthWriting(t *testing.T) {
	f := &framePath{window: &stubWindow{events: make(chan surface.Event)}}
	d := newSteadyDriver(f, 8)
	d.watchIdle(func() bool { return true })
	d.watchProgress(func() (int64, int64) { return 10, 0 })
	started := time.Now()
	for i := 0; i < 8; i++ {
		if tickReady(d, started) {
			t.Fatalf("the run settled at frame %d, before two idle frames", d.sent)
		}
	}
	if !tickReady(d, started) {
		t.Fatal("an unwatched run did not settle on the second idle frame")
	}

	d2 := newSteadyDriver(f, 8)
	d2.watchIdle(func() bool { return true })
	d2.watchProgress(func() (int64, int64) { return 10, 0 })
	asked, owed := 0, 3
	d2.watchSequence(func(time.Time) bool {
		asked++
		owed--
		return owed > 0
	}, 3*time.Second)
	deadline := d2.deadline
	started2 := time.Now()
	for i := 0; i < 8; i++ {
		if tickReady(d2, started2) {
			t.Fatalf("the sequence run settled at frame %d, before two idle frames", d2.sent)
		}
	}
	if asked != 0 {
		t.Fatalf("the sequence was asked %d times before the settle law had a frame to end on, want 0", asked)
	}
	for i := 0; i < 2; i++ {
		if tickReady(d2, started2) {
			t.Fatalf("the sequence run ended after %d captures, want it held for three", i+1)
		}
	}
	if !tickReady(d2, started2) {
		t.Fatal("the run did not end on the frame that wrote its last capture")
	}
	if asked != 3 {
		t.Fatalf("the sequence was asked %d times, want 3", asked)
	}
	if plain := newSteadyDriver(f, 8).deadline; deadline <= plain {
		t.Fatalf("deadline %v did not grow past the plain screenshot deadline %v to cover the sequence", deadline, plain)
	}
}

// qc:BR-085
func TestSequenceFlagsAreRefusedWhenTheyCannotMeanAnything(t *testing.T) {
	for _, args := range [][]string{
		{"-screenshot-seq", "/tmp/seq"},
		{"-screenshot-seq", "/tmp/seq", "-url", "file:///tmp/x.html", "-seq-frames", "0"},
		{"-screenshot-seq", "/tmp/seq", "-url", "file:///tmp/x.html", "-seq-interval", "0s"},
	} {
		if _, err := parse(args); err == nil {
			t.Errorf("parse(%q) accepted an unusable sequence", args)
		}
	}
	if _, err := parse([]string{"-screenshot-seq", "/tmp/seq", "-url", "file:///tmp/x.html"}); err != nil {
		t.Errorf("parse of a usable sequence: %v", err)
	}
}

const seqTallPage = `<!doctype html><html><head><meta charset="utf-8"></head><body style="margin:0">` +
	`<div style="width:400px;height:300px;margin:20px;background:rgb(200,0,0)"></div>` +
	`<div style="width:400px;height:300px;margin:0"></div>` +
	`<div style="width:400px;height:60px;margin:20px 0 0 20px;background:rgb(0,0,200)"></div>` +
	`<div style="width:400px;height:600px;margin:0;background:rgb(250,250,250)"></div>` +
	`</body></html>`

var seqBlue = color.RGBA{R: 0, G: 0, B: 200, A: 255}

func inked(img image.Image, want color.RGBA) int {
	n := 0
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if got := (color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}); got == want {
				n++
			}
		}
	}
	return n
}

// qc:BR-287
func TestSequenceScrollsBeforeItCaptures(t *testing.T) {
	url := writeSeqPage(t, "tall.html", seqTallPage)

	topDir, top, topShots := captureSeqArgs(t, url, 2, time.Duration(seqIntervalMS)*time.Millisecond)
	if got := inked(topShots[0], seqBlue); got != 0 {
		t.Fatalf("unscrolled run already inks the marker (%d px): the fixture is wrong, dir %s", got, topDir)
	}
	if top.ScrollScreens != 0 || top.ScrollPresses != 0 {
		t.Errorf("unscrolled record = screens %d presses %d, want 0 0", top.ScrollScreens, top.ScrollPresses)
	}
	for i, f := range top.Frames {
		if f.ScrollY != 0 {
			t.Errorf("unscrolled frame %d scroll_y = %d, want 0", i, f.ScrollY)
		}
	}

	_, rec, shots := captureSeqArgs(t, url, 2, time.Duration(seqIntervalMS)*time.Millisecond, "-seq-scroll-screens", "1")
	if rec.ScrollScreens != 1 {
		t.Errorf("report scroll_screens = %d, want 1", rec.ScrollScreens)
	}
	if rec.ScrollLanded == nil || !*rec.ScrollLanded {
		t.Fatalf("report scroll_landed = %s, want true on a 1300px page in a 400px viewport", landedText(rec.ScrollLanded))
	}
	if rec.ScrollPresses != 1 {
		t.Errorf("report scroll_presses = %d, want 1: one screen asked for, one PageDown sent", rec.ScrollPresses)
	}
	for i, f := range shots {
		if got := inked(f, seqBlue); got == 0 {
			t.Errorf("capture %d inks no marker: the scroll had not landed when the frame was written", i)
		}
		if b := f.Bounds(); b.Dx() != 600 || b.Dy() != 400 {
			t.Errorf("capture %d bounds = %v, want 600x400", i, b)
		}
	}
	for i, f := range rec.Frames {
		if f.ScrollY != 360 {
			t.Errorf("frame %d scroll_y = %d, want 360: every capture must sit at one PageDown of a 400px viewport", i, f.ScrollY)
		}
	}
	if n := changed(shots[0], shots[1], seqViewport); n != 0 {
		t.Errorf("the two scrolled captures differ by %d px: captures must all sit at one scroll position", n)
	}
}

// qc:BR-287
func TestSequenceStillCapturesWhenThePageCannotScroll(t *testing.T) {
	url := writeSeqPage(t, "static.html", seqStaticPage)
	_, rec, shots := captureSeqArgs(t, url, 3, time.Duration(seqIntervalMS)*time.Millisecond, "-seq-scroll-screens", "3")
	if len(shots) != 3 {
		t.Fatalf("wrote %d captures, want 3: a page that cannot scroll must not starve the sequence", len(shots))
	}
	if rec.ScrollLanded == nil || *rec.ScrollLanded {
		t.Fatalf("report scroll_landed = %s, want false on a page with nothing below the fold", landedText(rec.ScrollLanded))
	}
	if rec.ScrollScreens != 3 {
		t.Errorf("report scroll_screens = %d, want the 3 asked for", rec.ScrollScreens)
	}
	for i, f := range rec.Frames {
		if f.ScrollY != 0 {
			t.Errorf("frame %d scroll_y = %d, want 0 on a page that cannot scroll", i, f.ScrollY)
		}
	}
	for i := 1; i < len(shots); i++ {
		if n := changed(shots[0], shots[i], seqViewport); n != 0 {
			t.Fatalf("capture %d differs from capture 0 by %d px on a static unscrollable page", i, n)
		}
	}
}

// qc:BR-287
func TestScrollScreensFlagIsRefusedWhenItCannotMeanAnything(t *testing.T) {
	base := []string{"-screenshot-seq", "/tmp/seq", "-url", "file:///tmp/x.html"}
	if _, err := parse(append(append([]string{}, base...), "-seq-scroll-screens", "0")); err != nil {
		t.Errorf("parse refused -seq-scroll-screens 0: %v", err)
	}
	for _, bad := range []string{"-1", "abc"} {
		if _, err := parse(append(append([]string{}, base...), "-seq-scroll-screens", bad)); err == nil {
			t.Errorf("parse(-seq-scroll-screens %s) accepted an unusable scroll request", bad)
		}
	}
	if _, err := parse([]string{"-url", "file:///tmp/x.html", "-seq-scroll-screens", "2"}); err == nil {
		t.Error("parse accepted -seq-scroll-screens without -screenshot-seq, where nothing can act on it")
	}
}

type seqClock struct {
	drawn int64
	off   int32
	moves bool
}

func landedText(p *bool) string {
	if p == nil {
		return "absent"
	}
	return strconv.FormatBool(*p)
}

func (c *seqClock) stats() (bool, int, int64, int64, int64) {
	return true, 0, 0, c.drawn, c.drawn
}

func (c *seqClock) scroll(screens int) seqScroll {
	return seqScroll{
		Screens: screens,
		Screen:  func() int32 { return 360 },
		Scroll: func(dy int32) {
			if c.moves {
				c.off += dy
			}
		},
		Offset: func() int32 { return c.off },
	}
}

func scrollSeq(t *testing.T, clock *seqClock, screens int, count int) *frameSequence {
	t.Helper()
	s := newFrameSequence(config{width: 600, height: 400}, t.TempDir(), count, time.Second,
		func(string) error { return nil }, clock.stats, 1, "headless")
	s.setScroll(clock.scroll(screens))
	return s
}

// qc:BR-287
func TestSequenceScrollPhaseHoldsUntilTheOffsetIsDrawn(t *testing.T) {
	clock := &seqClock{moves: true}
	seq := scrollSeq(t, clock, 2, 1)
	if got, want := seq.horizon(), 3*time.Second; got != want {
		t.Errorf("horizon() = %v, want %v for 1 capture and 2 screens at 1s", got, want)
	}
	started := seq.started
	if !seq.capture(started) {
		t.Fatal("the sequence ended on the frame that only sent its first press")
	}
	if len(seq.report.Frames) != 0 {
		t.Fatalf("wrote %d captures before the scroll was drawn", len(seq.report.Frames))
	}
	if seq.report.ScrollPresses != 1 {
		t.Errorf("scroll_presses = %d after the first ask, want 1", seq.report.ScrollPresses)
	}
	clock.drawn = 1
	if !seq.capture(started.Add(time.Second)) {
		t.Fatal("the sequence ended after one frame past the press")
	}
	if len(seq.report.Frames) != 0 {
		t.Fatalf("wrote a capture one frame after the press: pixels at the new offset need two draws, got %+v", seq.report.Frames)
	}
	clock.drawn = 2
	if !seq.capture(started.Add(2 * time.Second)) {
		t.Fatal("the sequence ended on the frame that only sent its second press")
	}
	if seq.report.ScrollPresses != 2 {
		t.Errorf("scroll_presses = %d, want 2 for two screens", seq.report.ScrollPresses)
	}
	if len(seq.report.Frames) != 0 {
		t.Fatalf("captured before the second screen was asked for: %+v", seq.report.Frames)
	}
	clock.drawn = 3
	if !seq.capture(started.Add(3 * time.Second)) {
		t.Fatal("the sequence ended one frame past the second press")
	}
	clock.drawn = 4
	if seq.capture(started.Add(4 * time.Second)) {
		t.Fatal("the sequence kept running after writing its only capture")
	}
	if len(seq.report.Frames) != 1 {
		t.Fatalf("wrote %d captures, want 1", len(seq.report.Frames))
	}
	if got := seq.report.Frames[0].ScrollY; got != 720 {
		t.Errorf("capture scroll_y = %d, want 720: two screens of 360, and the pixels must be the drawn ones", got)
	}
	if seq.report.ScrollLanded == nil || !*seq.report.ScrollLanded {
		t.Errorf("scroll_landed = %s, want true", landedText(seq.report.ScrollLanded))
	}
}

// qc:BR-287
func TestSequenceScrollPhaseEndsWhenThePageDoesNotMove(t *testing.T) {
	clock := &seqClock{}
	seq := scrollSeq(t, clock, 3, 1)
	started := seq.started
	for _, drawn := range []int64{0, 1} {
		clock.drawn = drawn
		if !seq.capture(started.Add(time.Duration(drawn) * time.Second)) {
			t.Fatalf("the sequence ended on draw %d, before the page was known to be unscrollable", drawn)
		}
	}
	if len(seq.report.Frames) != 0 {
		t.Fatalf("captured while still waiting on a page that cannot move: %+v", seq.report.Frames)
	}
	clock.drawn = 2
	if seq.capture(started.Add(2 * time.Second)) {
		t.Fatal("the sequence never ends on a page that cannot scroll")
	}
	if seq.report.ScrollPresses != 1 {
		t.Errorf("scroll_presses = %d, want 1: an unmoving page must stop the phase, not burn all three screens", seq.report.ScrollPresses)
	}
	if seq.report.ScrollLanded == nil || *seq.report.ScrollLanded {
		t.Errorf("scroll_landed = %s, want false on a page that never moved", landedText(seq.report.ScrollLanded))
	}
	if len(seq.report.Frames) != 1 || seq.report.Frames[0].ScrollY != 0 {
		t.Errorf("captures = %+v, want one at scroll_y 0", seq.report.Frames)
	}
}
