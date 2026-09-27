package frame_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/frame"
)

// frameTime and frameDuration keep the recorder tests on a fake epoch. They take
// and return milliseconds as integers because the assertions are about ranks, and
// a wall-clock reading here would only make the test flaky.
func frameTime(ms int) time.Time { return time.Unix(0, int64(ms)*int64(time.Millisecond)) }

func frameDuration(ms int) time.Duration { return time.Duration(ms) * time.Millisecond }

// newStrideBitmap returns a 4x4 bitmap whose stride is 8 pixels wide, so every
// access path that assumes stride == W*4 breaks loudly rather than shifting the
// image diagonally.
func newStrideBitmap() *frame.Bitmap {
	w, h, stridePx := 4, 4, 8
	b := &frame.Bitmap{RGBA: make([]byte, stridePx*4*h), W: w, H: h, Stride: stridePx * 4}
	return b
}

func TestBitmapHonorsStride(t *testing.T) {
	b := newStrideBitmap()
	b.Set(3, 3, frame.RGB(1, 2, 3))
	if got := b.At(3, 3); got != frame.RGB(1, 2, 3) {
		t.Fatalf("At after Set = %v", got)
	}
	// Padding past the row must be untouched; a stride bug writes there and the
	// next row's pixels shift by one pixel per row.
	if got := b.At(0, 1); got != frame.TransparentBlack {
		t.Fatalf("untouched pixel = %v, want transparent (rows are aliasing)", got)
	}
	if got := b.Bounds(); got != frame.Rect4(0, 0, 4, 4) {
		t.Fatalf("local bounds = %v", got)
	}
	if got := b.Size(); got != (frame.Size{W: 4, H: 4}) {
		t.Fatalf("Size = %v", got)
	}
	if got := b.Bytes(); got != int64(len(b.RGBA)) {
		t.Fatalf("Bytes = %d, want %d (the budget tracks the allocation, not the useful area)", got, len(b.RGBA))
	}
}

func TestBitmapAtOutOfBoundsIsTransparent(t *testing.T) {
	b := frame.NewBitmap(4, 4)
	for _, p := range []frame.Point{{X: -1, Y: 0}, {X: 0, Y: -1}, {X: 4, Y: 0}, {X: 0, Y: 4}} {
		if got := b.At(int(p.X), int(p.Y)); got != frame.TransparentBlack {
			t.Fatalf("At(%v) = %#x, want transparent", p, uint32(got))
		}
	}
	b.Set(9, 9, frame.RGB(255, 0, 0)) // must be ignored, not a panic
	if b.At(0, 0) != frame.TransparentBlack {
		t.Fatal("out-of-range Set wrote into the buffer")
	}
}

func TestFillRectClipsAllFourEdges(t *testing.T) {
	b := frame.NewBitmap(8, 8)
	want := frame.RGB(10, 20, 30)
	b.FillRect(frame.Rect4(-4, -4, 12, 12), want, nil)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if got := b.At(x, y); got != want {
				t.Fatalf("over-wide fill left (%d,%d) = %v", x, y, got)
			}
		}
	}

	b.Reset()
	if b.At(0, 0) != frame.TransparentBlack {
		t.Fatal("Reset did not clear the buffer; a pooled buffer would paint the previous page")
	}
	// Each edge in turn.
	b.FillRect(frame.Rect4(2, 6, 5, 20), want, nil)
	if b.At(3, 7) != want || b.At(3, 5) != frame.TransparentBlack {
		t.Fatal("fill clipped the wrong side of the bottom edge")
	}
	b.Reset()
	b.FillRect(frame.Rect4(100, 100, 200, 200), want, nil)
	if b.At(0, 0) != frame.TransparentBlack {
		t.Fatal("fully disjoint fill wrote pixels")
	}
}

func TestFillRectCountsWrites(t *testing.T) {
	b := frame.NewBitmap(8, 8)
	var writes int64
	b.FillRect(frame.Rect4(-10, -10, 10, 10), frame.RGB(1, 1, 1), &writes)
	if writes != 64 {
		t.Fatalf("writes = %d, want 64 (clipped to the buffer, not the requested 20x20)", writes)
	}
}

func TestBlitOverOpaqueAndTranslucent(t *testing.T) {
	dst := frame.NewBitmap(4, 4)
	dst.FillRect(dst.Bounds(), frame.RGB(200, 200, 200), nil)
	src := frame.NewBitmap(2, 2)
	src.FillRect(src.Bounds(), frame.RGB(0, 0, 0), nil)

	dst.BlitOver(src, frame.Point{X: 1, Y: 1}, src.Bounds(), dst.Bounds(), nil)
	if got := dst.At(1, 1); got != frame.RGB(0, 0, 0) {
		t.Fatalf("opaque blit left %v at the destination origin", got)
	}
	if got := dst.At(0, 0); got != frame.RGB(200, 200, 200) {
		t.Fatalf("opaque blit wrote outside the source rect: %v", got)
	}

	trans := frame.NewBitmap(2, 2)
	trans.FillRect(trans.Bounds(), frame.RGBA(0, 0, 0, 128), nil)
	dst.BlitOver(trans, frame.Point{X: 3, Y: 3}, trans.Bounds(), dst.Bounds(), nil)
	got := dst.At(3, 3)
	if d := int(got.R()) - 100; d > 2 && d < -2 {
		t.Fatalf("half-black over 200 = %d, want ~100", got.R())
	}
}

func TestBlitOverClipsPartialTilesWithoutOOB(t *testing.T) {
	// A tile hanging off the right and bottom of the backing store is the normal
	// case at a viewport edge, and the case that indexes past a slice if the
	// source bounds are not clipped.
	src := frame.NewBitmap(8, 8)
	src.FillRect(src.Bounds(), frame.RGB(5, 5, 5), nil)
	dst := frame.NewBitmap(10, 10)

	var writes int64
	dst.BlitOver(src, frame.Point{X: 6, Y: 6}, src.Bounds(), dst.Bounds(), &writes)
	if writes != 16 {
		t.Fatalf("writes = %d, want 16 (only the 4x4 inside dst)", writes)
	}
	for _, p := range []frame.Point{{X: 9, Y: 9}, {X: 6, Y: 9}, {X: 9, Y: 6}} {
		if dst.At(int(p.X), int(p.Y)) != frame.RGB(5, 5, 5) {
			t.Fatalf("clipped blit missed (%d,%d)", p.X, p.Y)
		}
	}
	if dst.At(5, 5) != frame.TransparentBlack {
		t.Fatal("clipped blit wrote outside the destination rect")
	}

	// A srcRect larger than the source must clip to what exists rather than read
	// past the buffer.
	dst2 := frame.NewBitmap(10, 10)
	dst2.BlitOver(src, frame.Point{X: 0, Y: 0}, frame.Rect4(0, 0, 64, 64), dst2.Bounds(), nil)
	if dst2.At(7, 7) != frame.RGB(5, 5, 5) {
		t.Fatal("oversized srcRect missed the region the source can actually reach")
	}
	if dst2.At(8, 8) != frame.TransparentBlack {
		t.Fatalf("srcRect beyond the source's own 8x8 wrote at (8,8): %v", dst2.At(8, 8))
	}
}

func TestPoolReturnsSameBacking(t *testing.T) {
	size := frame.Size{W: 16, H: 16}
	p := frame.NewBitmapPool(size, 4)
	b := p.Acquire()
	before := p.Stats()
	p.Release(b)
	got := p.Acquire()
	if got != b {
		t.Fatal("Acquire after Release returned a different buffer; the pool is not recycling")
	}
	if &got.RGBA[0] != &b.RGBA[0] {
		t.Fatal("recycled buffer has a new backing array")
	}
	if len(got.RGBA) != cap(got.RGBA) {
		t.Fatalf("len %d != cap %d; a re-append would grow the buffer out of the budget", len(got.RGBA), cap(got.RGBA))
	}
	if after := p.Stats(); after.Created != before.Created {
		t.Fatalf("pool allocated %d more buffers after warm-up", after.Created-before.Created)
	}
}

func TestPoolPreallocRemovesWarmupAllocations(t *testing.T) {
	p := frame.NewBitmapPool(frame.Size{W: 8, H: 8}, 32)
	p.Prealloc(8)
	if got := p.Stats().Created; got != 8 {
		t.Fatalf("Prealloc created %d buffers, want 8", got)
	}
	held := make([]*frame.Bitmap, 0, 8)
	var allocs float64
	allocs += float64(testing.AllocsPerRun(8, func() { held = append(held, p.Acquire()) }))
	for _, b := range held {
		p.Release(b)
	}
	if allocs != 0 {
		t.Fatalf("Acquire from a warmed pool allocated %v times per call", allocs)
	}
}

func TestPoolRejectsWrongSize(t *testing.T) {
	p := frame.NewBitmapPool(frame.Size{W: 8, H: 8}, 4)
	p.Release(frame.NewBitmap(16, 16))
	if got := p.Stats().Idle; got != 0 {
		t.Fatalf("pool kept a wrong-size buffer (idle = %d); Acquire would hand out a buffer that aliases the wrong rows", got)
	}
	p.Release(nil)
	p.Release(&frame.Bitmap{})
}

func TestRecorderPercentilesAreNearestRank(t *testing.T) {
	rec := frame.NewFrameRecorder(1024)
	// 100 frames with work times 1ms..100ms, so nearest-rank p50 = 50ms and
	// p99 = 99ms. A mean of 50.5ms. Anything interpolated would differ.
	for i := 1; i <= 100; i++ {
		start := frameTime(i)
		rec.Record(frame.FrameMark{
			Serial:      uint64(i),
			VsyncAt:     start,
			PresentedAt: start.Add(frameDuration(i)),
		})
	}
	rep := rec.Report()
	if rep.Frames != 100 {
		t.Fatalf("Frames = %d, want 100", rep.Frames)
	}
	if rep.P50MS < 49.9 || rep.P50MS > 50.1 {
		t.Fatalf("P50 = %v ms, want 50", rep.P50MS)
	}
	if rep.P99MS < 98.9 || rep.P99MS > 99.1 {
		t.Fatalf("P99 = %v ms, want 99", rep.P99MS)
	}
	if rep.MaxMS < 99.9 || rep.MaxMS > 100.1 {
		t.Fatalf("Max = %v ms, want 100", rep.MaxMS)
	}
	if rep.MeanMS < 50.4 || rep.MeanMS > 50.6 {
		t.Fatalf("Mean = %v ms, want 50.5", rep.MeanMS)
	}
}

func TestRecorderWindowIsRingAndDoesNotAllocate(t *testing.T) {
	allocRec := frame.NewFrameRecorder(16)
	m := frame.FrameMark{Serial: 1}
	if n := testing.AllocsPerRun(100, func() { allocRec.Record(m) }); n != 0 {
		t.Fatalf("Record allocated %v times per call; it runs inside the frame path", n)
	}

	rec := frame.NewFrameRecorder(16)
	for i := 1; i <= 40; i++ {
		rec.Record(frame.FrameMark{Serial: uint64(i)})
	}
	if rec.Len() != 16 {
		t.Fatalf("Len = %d, want the window capped at 16", rec.Len())
	}
	if rec.Total() != 40 {
		t.Fatalf("Total = %d, want 40 recorded since construction", rec.Total())
	}
	marks := rec.Marks()
	if marks[0].Serial != 25 || marks[15].Serial != 40 {
		t.Fatalf("window = serials %d..%d, want 25..40 in chronological order", marks[0].Serial, marks[15].Serial)
	}
}

func TestRecorderReportAggregatesInvariantCounters(t *testing.T) {
	rec := frame.NewFrameRecorder(8)
	start := frameTime(0)
	rec.Record(frame.FrameMark{
		Serial: 1, VsyncAt: start, PresentedAt: start.Add(frameDuration(5)),
		TilesRasterized: 3, TilesReused: 4, StylePasses: 1, LayoutPasses: 0,
	})
	rec.Record(frame.FrameMark{
		Serial: 2, VsyncAt: start, PresentedAt: start.Add(frameDuration(5)),
		TilesReused: 96,
	})
	rep := rec.Report()
	if rep.TilesRasterized != 3 || rep.TilesReused != 100 || rep.StylePasses != 1 {
		t.Fatalf("aggregated counters = %+v", rep)
	}
	// Only the second frame did no style, no layout, and no rasterization, so it
	// is the only one that can count as a warm scroll frame.
	if rep.ZeroWorkFrames != 1 {
		t.Fatalf("ZeroWorkFrames = %d, want 1", rep.ZeroWorkFrames)
	}
	if rep.Frames != 2 {
		t.Fatalf("Frames = %d, want 2", rep.Frames)
	}
}

// TestRecorderReportsPresentPathLatency covers M1's criterion 8 instrument: how long a
// frame took to leave the composer and be accepted by the window. It is reported apart
// from the frame mean because the two move independently - a shim that copies or refuses
// changes this figure and nobody else's - and because the baseline file has to record
// one number that came from the window rather than from the pipeline.
func TestRecorderReportsPresentPathLatency(t *testing.T) {
	rec := frame.NewFrameRecorder(1024)
	for i := 1; i <= 100; i++ {
		start := frameTime(i)
		rec.Record(frame.FrameMark{
			Serial:      uint64(i),
			VsyncAt:     start,
			ComposedAt:  start,
			PresentedAt: start.Add(frameDuration(i)),
		})
	}
	rep := rec.Report()
	// composed→presented is i ms for i in 1..100, so the ranks are the frame ones above.
	if rep.PresentMeanMS < 50.4 || rep.PresentMeanMS > 50.6 {
		t.Errorf("PresentMeanMS = %v, want 50.5", rep.PresentMeanMS)
	}
	if rep.PresentP99MS < 98.9 || rep.PresentP99MS > 99.1 {
		t.Errorf("PresentP99MS = %v, want 99", rep.PresentP99MS)
	}

	// A frame that never reached a present contributes nothing, rather than a zero that
	// drags the mean down and a percentile read off a list one shorter than the frames.
	noPresent := frame.NewFrameRecorder(4)
	noPresent.Record(frame.FrameMark{Serial: 1, VsyncAt: frameTime(0)})
	noPresent.Record(frame.FrameMark{Serial: 2, ComposedAt: frameTime(0), PresentedAt: frameTime(4)})
	got := noPresent.Report()
	if got.PresentMeanMS < 3.9 || got.PresentMeanMS > 4.1 {
		t.Errorf("PresentMeanMS = %v with one unstamped frame in the window; unstamped frames must be left out of the denominator, not averaged in as zero", got.PresentMeanMS)
	}

	// The instrument is worth nothing if it cannot see a slow present.
	late := frame.NewFrameRecorder(4)
	late.Record(frame.FrameMark{Serial: 1, ComposedAt: frameTime(0), PresentedAt: frameTime(2)})
	late.Record(frame.FrameMark{Serial: 2, ComposedAt: frameTime(0), PresentedAt: frameTime(900)})
	if v := late.Report().PresentP99MS; v < 899 {
		t.Errorf("PresentP99MS = %v for a window holding a 900ms present; the instrument cannot see the case it exists for", v)
	}
}

// TestRecorderJSONFieldNamesSayWhatTheyHold keeps the artifact a shell script reads
// honest about its own keys. The timestamps are time.Time values, so they encode as RFC
// 3339 strings; a name ending in "_ns" invites a consumer to subtract them as integers,
// which jq answers with a coerced null - the failure shape that looks like a fast frame.
func TestRecorderJSONFieldNamesSayWhatTheyHold(t *testing.T) {
	rec := frame.NewFrameRecorder(4)
	start := frameTime(0)
	rec.Record(frame.FrameMark{Serial: 1, VsyncAt: start, PresentedAt: start.Add(time.Millisecond)})

	var buf bytes.Buffer
	if err := rec.WriteJSON(&buf); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	var doc struct {
		Report map[string]any   `json:"report"`
		Frames []map[string]any `json:"frames"`
	}
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("the JSON this writes is not what it reads back: %v", err)
	}
	if len(doc.Frames) != 1 {
		t.Fatalf("%d frames in the artifact, want 1", len(doc.Frames))
	}
	for _, name := range []string{"vsync", "plan", "submit", "composed", "presented"} {
		key := name + "_at"
		if v, ok := doc.Frames[0][key]; !ok {
			t.Errorf("the mark has no %q field", key)
		} else if _, isString := v.(string); !isString {
			t.Errorf("%q holds %T, not a time string", key, v)
		}
		if _, ok := doc.Frames[0][key+"_ns"]; ok {
			t.Errorf("the mark still carries %q, which promises an integer it does not hold", key+"_ns")
		}
	}
	for _, key := range []string{"mean_ms", "p99_ms", "present_mean_ms", "present_p99_ms"} {
		if _, ok := doc.Report[key]; !ok {
			t.Errorf("the report has no %q field; the gate script reads it", key)
		}
	}
}

// TestRecorderReportsStartupLatency covers the brief's other headline number: how long
// the process took to put a frame on screen. The recorder is the only instrument that
// sees every frame, so the first presented mark is in its hands rather than the loop's,
// and the delta is computed here for the reason the marks' header gives - deltas are not
// a shell's job.
func TestRecorderReportsStartupLatency(t *testing.T) {
	rec := frame.NewFrameRecorder(8)
	rec.SetStartupReference(frameTime(0))
	// A vsync that arrived before the producer published a plan stamps no present,
	// and it is not the frame the user saw.
	rec.Record(frame.FrameMark{Serial: 1, VsyncAt: frameTime(0)})
	rec.Record(frame.FrameMark{Serial: 2, VsyncAt: frameTime(16), ComposedAt: frameTime(120), PresentedAt: frameTime(120)})
	rec.Record(frame.FrameMark{Serial: 3, VsyncAt: frameTime(32), ComposedAt: frameTime(200), PresentedAt: frameTime(200)})
	got := rec.Report().StartupMS
	if got == nil {
		t.Fatal("StartupMS is nil for a run that presented a frame")
	}
	if *got < 119.9 || *got > 120.1 {
		t.Errorf("StartupMS = %v, want the 120ms until the first presented frame, not the 200ms until the last", *got)
	}
}

// TestRecorderStartupSurvivesRingEviction is why the first present is held outside the
// window. A 600-frame gate run evicts its own first frame from a 1024-slot ring only
// rarely, but a run long enough to wrap the ring would otherwise report the startup of
// whichever frame happened to still be in it, and the number would read as plausible.
func TestRecorderStartupSurvivesRingEviction(t *testing.T) {
	rec := frame.NewFrameRecorder(4)
	rec.SetStartupReference(frameTime(0))
	rec.Record(frame.FrameMark{Serial: 1, PresentedAt: frameTime(250)})
	for i := 2; i <= 40; i++ {
		rec.Record(frame.FrameMark{Serial: uint64(i), PresentedAt: frameTime(1000 + i)})
	}
	got := rec.Report().StartupMS
	if got == nil {
		t.Fatal("StartupMS is nil after the window wrapped")
	}
	if *got != 250 {
		t.Errorf("StartupMS = %v, want the 250ms first present; the window has since evicted it", *got)
	}
}

// TestRecorderStartupIsNilWhenNotMeasured keeps "no measurement" out of the artifact as a
// number. A zero here is the fastest possible startup, so a build that stopped stamping
// one, or a run whose window never presented, would gate as a sub-second cold start
// rather than as the missing metric it is.
func TestRecorderStartupIsNilWhenNotMeasured(t *testing.T) {
	// Nothing was ever told when the process began.
	unreferenced := frame.NewFrameRecorder(4)
	unreferenced.Record(frame.FrameMark{Serial: 1, PresentedAt: frameTime(80)})
	if got := unreferenced.Report().StartupMS; got != nil {
		t.Errorf("StartupMS = %v with no startup reference set; want nil", *got)
	}

	// The reference is known but no frame reached a window.
	noPresent := frame.NewFrameRecorder(4)
	noPresent.SetStartupReference(frameTime(0))
	noPresent.Record(frame.FrameMark{Serial: 1, VsyncAt: frameTime(0)})
	noPresent.Record(frame.FrameMark{Serial: 2, VsyncAt: frameTime(16)})
	if got := noPresent.Report().StartupMS; got != nil {
		t.Errorf("StartupMS = %v for a run that presented nothing; want nil", *got)
	}

	// Reset empties the window, so the frame it called first is gone with it. The
	// reference survives: it is a fact about the process, not about the window. The
	// assertion is the one after the reset rather than an empty-window nil, which the
	// report's empty-window early return would hand back whether or not Reset worked.
	rec := frame.NewFrameRecorder(4)
	rec.SetStartupReference(frameTime(0))
	rec.Record(frame.FrameMark{Serial: 1, PresentedAt: frameTime(250)})
	rec.Reset()
	rec.Record(frame.FrameMark{Serial: 2, PresentedAt: frameTime(400)})
	got := rec.Report().StartupMS
	if got == nil || *got != 400 {
		t.Errorf("StartupMS = %v after one post-reset present, want 400 rather than the 250 the window no longer holds", got)
	}
}

// TestRecorderStartupJSONIsNullRatherThanZero pins the shape the gate script reads. jq
// answers a missing key and a null the same way, so null is the encoding that lets the
// script's existing fail-closed check refuse an unmeasured startup.
func TestRecorderStartupJSONIsNullRatherThanZero(t *testing.T) {
	var buf bytes.Buffer
	unmeasured := frame.NewFrameRecorder(4)
	unmeasured.Record(frame.FrameMark{Serial: 1, PresentedAt: frameTime(80)})
	if err := unmeasured.WriteJSON(&buf); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	var doc struct {
		Report map[string]any `json:"report"`
	}
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	v, ok := doc.Report["startup_ms"]
	if !ok {
		t.Fatal("the report has no startup_ms key; the gate script reads it")
	}
	if v != nil {
		t.Errorf("startup_ms holds %v (%T) for an unmeasured run; want null", v, v)
	}

	buf.Reset()
	measured := frame.NewFrameRecorder(4)
	measured.SetStartupReference(frameTime(0))
	measured.Record(frame.FrameMark{Serial: 1, PresentedAt: frameTime(80)})
	if err := measured.WriteJSON(&buf); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if n, isNum := doc.Report["startup_ms"].(float64); !isNum || n != 80 {
		t.Errorf("startup_ms holds %v for a measured run; want the number 80", doc.Report["startup_ms"])
	}
}

// TestRecorderReportsTileLedger gives the artifact the number the frame timings cannot
// imply. TilesRasterized counts work, and a run that rasterized 3,000 tiles is the same
// report whether those buffers are held, were freed back to the pool, or escaped the
// accounting that authorised them - which is the difference between a cache that filled
// its configuration and a path that retains memory nobody asked for. The grid knows it
// and the recorder, which times frames, does not, so it arrives by setter exactly the
// way the startup reference does.
func TestRecorderReportsTileLedger(t *testing.T) {
	rec := frame.NewFrameRecorder(4)
	rec.SetStartupReference(frameTime(0))
	rec.Record(frame.FrameMark{Serial: 1, PresentedAt: frameTime(80)})
	rec.SetTileLedger(frame.GridStats{Bytes: 783286272, Budget: 785383424, Evictions: 7})

	got := rec.Report()
	for _, want := range []struct {
		name string
		held *int64
		n    int64
	}{
		{"TileBytes", got.TileBytes, 783286272},
		{"TileBudget", got.TileBudget, 785383424},
		{"TileEvictions", got.TileEvictions, 7},
	} {
		if want.held == nil {
			t.Errorf("%s is nil for a run that reported a ledger", want.name)
			continue
		}
		if *want.held != want.n {
			t.Errorf("%s = %d, want %d", want.name, *want.held, want.n)
		}
	}
}

// TestRecorderTileLedgerJSONIsMissingRatherThanZero is the fail-closed half of the
// ledger. An absent field and a zero read the same way to a shell that trusts them, and
// zero bytes held is a cache that never filled - the best possible result - so a build
// that stopped handing the grid in would gate as a pass rather than as the missing
// metric it is. null is what lets the script refuse.
func TestRecorderTileLedgerJSONIsMissingRatherThanZero(t *testing.T) {
	read := func(t *testing.T, rec *frame.FrameRecorder) map[string]any {
		t.Helper()
		var buf bytes.Buffer
		if err := rec.WriteJSON(&buf); err != nil {
			t.Fatalf("WriteJSON: %v", err)
		}
		var doc struct {
			Report map[string]any `json:"report"`
		}
		if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return doc.Report
	}

	unmeasured := frame.NewFrameRecorder(4)
	unmeasured.Record(frame.FrameMark{Serial: 1, PresentedAt: frameTime(80)})
	report := read(t, unmeasured)
	for _, key := range []string{"tile_bytes", "tile_budget", "tile_evictions"} {
		v, ok := report[key]
		if !ok {
			t.Errorf("the report has no %q key; the gate script reads it", key)
			continue
		}
		if v != nil {
			t.Errorf("%q holds %v (%T) for a run that reported no ledger; want null", key, v, v)
		}
	}

	// A measured zero stays a zero. Evictions in particular is a figure a warm
	// scroll run is meant to reach, and nulling it would make the passing case
	// indistinguishable from the uninstrumented one.
	buf := frame.NewFrameRecorder(4)
	buf.Record(frame.FrameMark{Serial: 1, PresentedAt: frameTime(80)})
	buf.SetTileLedger(frame.GridStats{Bytes: 262144, Budget: 785383424})
	report = read(t, buf)
	for _, want := range []struct {
		key string
		n   float64
	}{
		{"tile_bytes", 262144},
		{"tile_budget", 785383424},
		{"tile_evictions", 0},
	} {
		if n, isNum := report[want.key].(float64); !isNum || n != want.n {
			t.Errorf("%q holds %v (%T); want the number %v", want.key, report[want.key], report[want.key], want.n)
		}
	}
}

// TestRecorderReportsContent is the artifact's provenance. Every timing in a report was
// taken on something, and the recorder times frames without knowing what those frames
// depicted - so a caller that handed in the page's name and laid-out height has to be the
// one saying so, and an artifact with no frames in its window still describes the run.
func TestRecorderReportsContent(t *testing.T) {
	rec := frame.NewFrameRecorder(4)
	rec.SetContent("file:///tmp/tall.html", 16016)

	got := rec.Report()
	if got.Document != "file:///tmp/tall.html" {
		t.Errorf("Document = %q, want the page the run was handed", got.Document)
	}
	if got.DocHeight != 16016 {
		t.Errorf("DocHeight = %d, want the laid-out height in device pixels", got.DocHeight)
	}
	// The synthetic case is the empty name rather than a made-up one: a gate reading a
	// scene's artifact must be able to tell it apart from a page's.
	scene := frame.NewFrameRecorder(4)
	scene.SetContent("", 63600)
	if d := scene.Report().Document; d != "" {
		t.Errorf("Document = %q for a scene run, want empty", d)
	}

	var buf bytes.Buffer
	if err := rec.WriteJSON(&buf); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	var doc struct {
		Report map[string]any `json:"report"`
	}
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s, ok := doc.Report["document"].(string); !ok || s != "file:///tmp/tall.html" {
		t.Errorf(`"document" holds %v (%T), want the URL as a string`, doc.Report["document"], doc.Report["document"])
	}
	if n, ok := doc.Report["doc_height"].(float64); !ok || n != 16016 {
		t.Errorf(`"doc_height" holds %v (%T), want 16016`, doc.Report["doc_height"], doc.Report["doc_height"])
	}
}
