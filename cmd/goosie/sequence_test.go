package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
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
	URL        string  `json:"url"`
	OS         string  `json:"os"`
	Arch       string  `json:"arch"`
	Backend    string  `json:"backend"`
	ViewportW  int     `json:"viewport_width"`
	ViewportH  int     `json:"viewport_height"`
	DPR        float64 `json:"dpr"`
	Zoom       float64 `json:"zoom"`
	IntervalMS int64   `json:"interval_ms"`
	Frames     []struct {
		PNG           string `json:"png"`
		MS            int64  `json:"ms"`
		Idle          bool   `json:"idle"`
		TilesNeeded   int    `json:"tiles_needed"`
		PaintingBytes int64  `json:"painting_bytes"`
		Presents      int64  `json:"presents"`
		Frames        int64  `json:"frames"`
	} `json:"frames"`
}

func captureSeq(t *testing.T, url string, count int, interval time.Duration) (string, seqRecord, []image.Image) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "seq")
	if err := run([]string{"-screenshot-seq", dir, "-url", url, "-backend", "headless",
		"-width", "600", "-height", "400", "-dpr", "1",
		"-seq-frames", strconv.Itoa(count), "-seq-interval", interval.String()}); err != nil {
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
