package raster

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/v2/internal/frame"
	"github.com/vyquocvu/goosie/v2/internal/paint"
	"github.com/vyquocvu/goosie/v2/internal/raster"
)

// testDeadline is the hang budget for every blocking step below. The pool's
// contract is that neither direction blocks the caller, so a lost result or a
// leaked panic has to fail as "did not return within 2s" rather than as a test
// suite that CI's own timeout eventually reports as nothing at all.
const testDeadline = 2 * time.Second

// within runs fn on a helper goroutine and fails if it has not returned by the
// deadline. The goroutine is abandoned rather than killed, which is fine in a
// test and is the only way to observe a hang at all.
func within(t *testing.T, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(testDeadline):
		t.Fatalf("%s did not return within %v", what, testDeadline)
	}
}

// mustNotPanic fails instead of taking the test binary down. It is applied to the
// caller's own calls: a panic raised inside a worker is supposed to be converted
// by the pool long before it could reach here.
func mustNotPanic(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if v := recover(); v != nil {
			t.Fatalf("%s panicked in the caller: %v", what, v)
		}
	}()
	fn()
}

// collect waits for n results, failing on timeout rather than blocking forever.
func collect(t *testing.T, p *raster.Pool, n int) []raster.Done {
	t.Helper()
	ch := make(chan []raster.Done, 1)
	go func() { ch <- p.Wait(n) }()
	select {
	case ds := <-ch:
		if len(ds) != n {
			t.Fatalf("Wait(%d) returned %d results", n, len(ds))
		}
		return ds
	case <-time.After(testDeadline):
		t.Fatalf("Wait(%d) did not collect %d results within %v", n, n, testDeadline)
		return nil
	}
}

// pollUntil waits for cond to hold, reporting whether it ever did. Every test
// that needs an asynchronous side effect to land uses this instead of a sleep,
// so the suite stays fast when the code works and still fails when it does not.
func pollUntil(cond func() bool) bool {
	deadline := time.Now().Add(testDeadline)
	for {
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(time.Millisecond)
	}
}

func TestDefaultWorkersLeavesOneCoreAndCapsAtFour(t *testing.T) {
	want := runtime.GOMAXPROCS(0) - 1
	if want < 1 {
		want = 1
	}
	if want > 4 {
		want = 4
	}
	if got := raster.DefaultWorkers(); got != want {
		t.Fatalf("DefaultWorkers() = %d, want min(max(GOMAXPROCS-1, 1), 4) = %d", got, want)
	}
	// Zero means "decide for me" rather than a pool with no workers, which would
	// silently never finish a tile.
	if p := raster.New(0, 0, nil); p.Workers() != want {
		t.Fatalf("New(0, 0).Workers() = %d, want %d", p.Workers(), want)
	}
	if p := raster.New(16, 0, nil); p.Workers() != 16 {
		t.Fatalf("an explicit worker count was overridden: %d", p.Workers())
	}
}

func TestAllJobsCompleteWithNoLostResults(t *testing.T) {
	const cols, rows = 16, 16
	n := cols * rows
	// The buffers only have to be identifiable here, so they are small: this test
	// is about the pool carrying one result per job, not about pixels.
	outs := make([]*frame.Bitmap, n)
	for i := range outs {
		outs[i] = frame.NewBitmap(4, 4)
	}
	p := raster.New(raster.DefaultWorkers(), 2*n, func(raster.Job) error { return nil })
	p.Start(context.Background())
	defer p.Close()

	seen := make(map[frame.TileCoord]int, n)
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			i := row*cols + col
			c := frame.TileCoord{Col: int32(col), Row: int32(row)}
			if err := p.Submit(raster.Job{Coord: c, Out: outs[i]}); err != nil {
				t.Fatalf("Submit: %v", err)
			}
		}
	}
	for _, d := range collect(t, p, n) {
		if d.Err != nil {
			t.Fatalf("job %v failed: %v", d.Coord, d.Err)
		}
		seen[d.Coord]++
		want := outs[int(d.Coord.Row)*cols+int(d.Coord.Col)]
		if d.Out != want {
			t.Fatalf("job %v came back with the wrong buffer", d.Coord)
		}
	}
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			c := frame.TileCoord{Col: int32(col), Row: int32(row)}
			switch seen[c] {
			case 1:
			case 0:
				t.Fatalf("job %v was never reported", c)
			default:
				t.Fatalf("job %v was reported %d times", c, seen[c])
			}
		}
	}
	if got := p.Stats().Rasterized; got != int64(n) {
		t.Fatalf("Rasterized = %d, want %d", got, n)
	}
}

// TestJobsRunConcurrentlyAcrossWorkers proves the pool really has the worker
// count it claims. Every job blocks inside the rasterizer until all of them are
// in there at once, so a serialized pool hits the deadline instead of passing by
// accident.
func TestJobsRunConcurrentlyAcrossWorkers(t *testing.T) {
	const w = 4
	gate := make(chan struct{})
	var inside int32
	rf := func(raster.Job) error {
		if atomic.AddInt32(&inside, 1) == w {
			close(gate)
		}
		select {
		case <-gate:
			return nil
		case <-time.After(testDeadline):
			return errors.New("workers never overlapped")
		}
	}
	p := raster.New(w, w, rf)
	p.Start(context.Background())
	defer p.Close()

	for i := 0; i < w; i++ {
		if err := p.Submit(raster.Job{Coord: frame.TileCoord{Col: int32(i)}}); err != nil {
			t.Fatalf("Submit: %v", err)
		}
	}
	for _, d := range collect(t, p, w) {
		if d.Err != nil {
			t.Fatalf("tile %v: %v", d.Coord, d.Err)
		}
	}
}

func TestPanickingJobIsRecoveredAndPoolKeepsServing(t *testing.T) {
	boom := frame.TileCoord{Col: 3, Row: 4}
	var calls int64
	rf := func(j raster.Job) error {
		atomic.AddInt64(&calls, 1)
		if j.Coord == boom {
			panic("synthetic rasterizer explosion")
		}
		return nil
	}
	p := raster.New(raster.DefaultWorkers(), 128, rf)
	p.Start(context.Background())
	defer p.Close()

	const clean0 = 8
	mustNotPanic(t, "the submit path", func() {
		for i := 0; i < clean0; i++ {
			if err := p.Submit(raster.Job{Coord: frame.TileCoord{Col: int32(i)}}); err != nil {
				t.Errorf("Submit: %v", err)
			}
		}
		// The same exploding tile twice: recovery has to be per job, not a
		// one-shot that leaves the pool disabled afterwards.
		for i := 0; i < 2; i++ {
			if err := p.Submit(raster.Job{Coord: boom}); err != nil {
				t.Errorf("Submit(boom): %v", err)
			}
		}
	})

	var panicked int
	for _, d := range collect(t, p, clean0+2) {
		if d.Coord == boom {
			if !errors.Is(d.Err, raster.ErrTilePanic) {
				t.Fatalf("exploding tile reported %v, want ErrTilePanic", d.Err)
			}
			panicked++
			continue
		}
		if d.Err != nil {
			t.Fatalf("innocent tile %v reported %v", d.Coord, d.Err)
		}
	}
	if panicked != 2 {
		t.Fatalf("%d results carried ErrTilePanic, want 2", panicked)
	}
	if st := p.Stats(); st.Panics != 2 || st.Failed != 2 || st.Rasterized != clean0 {
		t.Fatalf("stats = %+v, want Panics=2 Failed=2 Rasterized=%d", st, clean0)
	}

	// Still serving afterwards: a panic must not retire the worker that ran it.
	const more = 64
	mustNotPanic(t, "the submit path after a panic", func() {
		for i := 0; i < more; i++ {
			if err := p.Submit(raster.Job{Coord: frame.TileCoord{Row: int32(i)}}); err != nil {
				t.Fatalf("Submit after a panic: %v", err)
			}
		}
	})
	within(t, "collecting results after a panic", func() {
		for _, d := range collect(t, p, more) {
			if d.Err != nil {
				t.Errorf("post-panic job %v: %v", d.Coord, d.Err)
			}
		}
	})
	if got := p.Stats().Rasterized; got != clean0+more {
		t.Fatalf("Rasterized = %d, want %d", got, clean0+more)
	}
	if got := atomic.LoadInt64(&calls); got != clean0+2+more {
		t.Fatalf("the rasterizer ran %d times, want one call per job that executed", got)
	}
}

// TestFailedTileIsRetriedOnceThenMarkedFailed walks the caller's retry policy end
// to end against a real grid: the first panic leaves the tile worth asking about
// again, the second exhausts MaxTileAttempts, and only then does the tile become
// a blank one with a counter. The pool supplies the distinction and the grid owns
// the bookkeeping, which is why neither can retire a tile alone.
func TestFailedTileIsRetriedOnceThenMarkedFailed(t *testing.T) {
	c := frame.TileCoord{Col: 1, Row: 1}
	l, dl := newLayer(t, 2, 2)
	rf := func(j raster.Job) error {
		if j.Coord == c {
			panic("deterministic failure")
		}
		return nil
	}
	p := raster.New(raster.DefaultWorkers(), 16, rf)
	p.Start(context.Background())
	defer p.Close()

	out, ok := l.Grid.Acquire(c)
	if !ok {
		t.Fatal("Acquire refused a tile inside the layer")
	}
	if l.Grid.Attempts(c) != 1 {
		t.Fatalf("Attempts after Acquire = %d, want 1", l.Grid.Attempts(c))
	}
	reports := 0
	for {
		if err := p.Submit(raster.Job{Layer: l, Coord: c, DL: dl, Bounds: c.Rect(frame.TileSize), Out: out}); err != nil {
			t.Fatalf("Submit: %v", err)
		}
		d := collect(t, p, 1)[0]
		reports++
		if !errors.Is(d.Err, raster.ErrTilePanic) {
			t.Fatalf("attempt %d reported %v, want ErrTilePanic", reports, d.Err)
		}
		if l.Grid.Attempts(c) >= frame.MaxTileAttempts {
			break
		}
		// The retry re-acquires rather than reusing the job: that is what counts
		// the attempt and what keeps the buffer's ownership unambiguous.
		out, _ = l.Grid.Acquire(c)
	}
	if reports != frame.MaxTileAttempts {
		t.Fatalf("the tile was rasterized %d times, want %d", reports, frame.MaxTileAttempts)
	}
	l.Grid.MarkFailed(c)
	if !l.Grid.Failed(c) {
		t.Fatal("the exhausted tile is not marked failed")
	}
	if t0 := l.Grid.Peek(c); t0.Blittable() {
		t.Fatal("a failed tile is still blittable; it must render as the layer background")
	}
	if got := l.Stats().Failed; got != 1 {
		t.Fatalf("grid Failed count = %d, want 1", got)
	}

	// The neighbour is untouched by any of this and rasterizes normally.
	near := frame.TileCoord{Col: 0, Row: 0}
	nout, ok := l.Grid.Acquire(near)
	if !ok {
		t.Fatal("Acquire refused the neighbour")
	}
	if err := p.Submit(raster.Job{Layer: l, Coord: near, DL: dl, Bounds: near.Rect(frame.TileSize), Out: nout}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	d := collect(t, p, 1)[0]
	if d.Err != nil {
		t.Fatalf("the neighbour tile failed too: %v", d.Err)
	}
	if !l.Grid.MarkValid(near, l.ContentVersion, d.Out) {
		t.Fatal("MarkValid rejected a current result")
	}
	if got := l.Stats().Valid; got != 1 {
		t.Fatalf("grid Valid count = %d, want 1", got)
	}
}

func TestQueuedSubmissionsAreNotCountedAsWork(t *testing.T) {
	// Unstarted, so nothing can have run: the queue holds all four jobs.
	p := raster.New(2, 4, func(raster.Job) error { return nil })
	for i := 0; i < 4; i++ {
		if err := p.Submit(raster.Job{Coord: frame.TileCoord{Col: int32(i)}}); err != nil {
			t.Fatalf("Submit: %v", err)
		}
	}
	if st := p.Stats(); st.Queued != 4 || st.Rasterized != 0 || st.Failed != 0 || st.Refused != 0 {
		t.Fatalf("stats before Start = %+v, want Queued=4 and every other counter zero", st)
	}
	p.Start(context.Background())
	defer p.Close()
	collect(t, p, 4)
	if got := p.Stats().Rasterized; got != 4 {
		t.Fatalf("Rasterized = %d, want 4 once the jobs actually ran", got)
	}
}

func TestSubmitRefusesInsteadOfBlocking(t *testing.T) {
	p := raster.New(2, 1, func(raster.Job) error { return nil })
	if err := p.Submit(raster.Job{}); err != nil {
		t.Fatalf("first Submit: %v", err)
	}
	// The queue is one deep and no worker exists, so the next two are refused.
	// Refusing is the whole point: waiting here would put tile raster on the UI
	// thread, which is invariant 2.
	for i := 0; i < 2; i++ {
		if err := p.Submit(raster.Job{}); !errors.Is(err, raster.ErrQueueFull) {
			t.Fatalf("Submit to a full queue = %v, want ErrQueueFull", err)
		}
	}
	if got := p.Stats().Refused; got != 2 {
		t.Fatalf("Refused = %d, want 2", got)
	}
	within(t, "Submit onto a full queue", func() {
		if err := p.Submit(raster.Job{}); !errors.Is(err, raster.ErrQueueFull) {
			t.Errorf("Submit = %v, want ErrQueueFull", err)
		}
	})
}

func TestDroppedResultsAreCountedNotBlocked(t *testing.T) {
	// A completion queue one deep with nobody polling: workers must keep drawing
	// jobs from the job queue instead of stalling on a UI thread that stopped
	// listening, and the overflow has to be visible as a counter.
	const n = 32
	p := raster.New(2, 1, func(raster.Job) error { return nil })
	p.Start(context.Background())
	defer p.Close()
	for i := 0; i < n; i++ {
		submitted := false
		within(t, "submitting into a full pool", func() {
			for !submitted {
				if p.Submit(raster.Job{Coord: frame.TileCoord{Col: int32(i)}}) == nil {
					submitted = true
					continue
				}
				if _, ok := p.Poll(); !ok {
					time.Sleep(time.Millisecond)
				}
			}
		})
	}
	within(t, "draining the job queue", func() {
		for p.Stats().Queued > 0 {
			time.Sleep(time.Millisecond)
		}
	})
	st := p.Stats()
	if st.Rasterized+st.Failed != n {
		t.Fatalf("only %d of %d jobs ran (Rasterized=%d Failed=%d)", st.Rasterized+st.Failed, n, st.Rasterized, st.Failed)
	}
	if st.Refused == 0 {
		t.Fatal("nobody polled and yet no result was dropped; the completion queue cannot have bound")
	}
	if got := len(p.Wait(1)); got != 1 {
		t.Fatalf("the one slot in the completion queue held %d results", got)
	}
}

func TestPollNeverWaits(t *testing.T) {
	var once sync.Once
	entered := make(chan struct{})
	release := make(chan struct{})
	rf := func(raster.Job) error {
		once.Do(func() { close(entered) })
		<-release
		return nil
	}
	p := raster.New(1, 4, rf)
	p.Start(context.Background())
	defer p.Close()
	if err := p.Submit(raster.Job{}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	<-entered // the single worker is busy, so the completion queue is certainly empty
	within(t, "Poll on an empty completion queue", func() {
		for i := 0; i < 1000; i++ {
			if d, ok := p.Poll(); ok {
				t.Errorf("Poll reported a result that had not finished: %+v", d)
				return
			}
		}
	})
	close(release)
	if !pollUntil(func() bool { _, ok := p.Poll(); return ok }) {
		t.Fatal("the finished job was never polled")
	}
}

func TestSubmitPathIsAllocationFree(t *testing.T) {
	// Unstarted with a queue deeper than the measurement, so the steady-state
	// paths run without a worker racing them. Invariant 6 covers the UI thread,
	// and these are the calls the UI thread makes every frame.
	p := raster.New(1, 1024, func(raster.Job) error { return nil })
	j := raster.Job{Coord: frame.TileCoord{Col: 1, Row: 2}, Bounds: frame.Rect4(0, 0, 256, 256)}
	if err := p.Submit(j); err != nil {
		t.Fatalf("warm-up Submit: %v", err)
	}
	if n := testing.AllocsPerRun(200, func() {
		if err := p.Submit(j); err != nil {
			t.Errorf("Submit: %v", err)
		}
	}); n != 0 {
		t.Fatalf("Submit allocated %v times per call", n)
	}
	if n := testing.AllocsPerRun(200, func() { p.Poll() }); n != 0 {
		t.Fatalf("Poll allocated %v times per call", n)
	}
	if n := testing.AllocsPerRun(200, func() { p.Stats() }); n != 0 {
		t.Fatalf("Stats allocated %v times per call", n)
	}
}

func TestCloseIsIdempotentAndRefusesLaterSubmits(t *testing.T) {
	p := raster.New(2, 8, func(raster.Job) error { return nil })
	p.Start(context.Background())
	if err := p.Submit(raster.Job{}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	within(t, "Close", func() {
		for i := 0; i < 3; i++ {
			if err := p.Close(); err != nil {
				t.Errorf("Close: %v", err)
			}
		}
	})
	if err := p.Submit(raster.Job{}); !errors.Is(err, raster.ErrPoolClosed) {
		t.Fatalf("Submit after Close = %v, want ErrPoolClosed", err)
	}
	if got := p.Stats().Refused; got == 0 {
		t.Fatal("a closed pool refused a job without counting it")
	}
}

func TestCancelingTheStartContextClosesThePool(t *testing.T) {
	// A caller that owns a document lifetime must not need a separate teardown:
	// cancelling the context it started with has to shut the pool down by itself.
	ctx, cancel := context.WithCancel(context.Background())
	p := raster.New(2, 8, func(raster.Job) error { return nil })
	p.Start(ctx)
	defer p.Close()
	cancel()
	if !pollUntil(func() bool { return errors.Is(p.Submit(raster.Job{}), raster.ErrPoolClosed) }) {
		t.Fatal("cancelling the start context never closed the pool")
	}
}

func TestNilRasterizerReportsInsteadOfPanicking(t *testing.T) {
	p := raster.New(1, 4, nil)
	p.Start(context.Background())
	defer p.Close()
	if err := p.Submit(raster.Job{}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if d := collect(t, p, 1)[0]; !errors.Is(d.Err, raster.ErrNoRasterizer) {
		t.Fatalf("Err = %v, want ErrNoRasterizer", d.Err)
	}
}

func TestConcurrentWaitersGetDistinctResults(t *testing.T) {
	// Wait reuses one buffer, so two waiters have to be serialised: without it
	// both append into the same slot and each returns the other's tile. That is a
	// data race the race detector cannot see, because it is not unsynchronised
	// memory but a shared slice handed out twice.
	const n = 32
	p := raster.New(raster.DefaultWorkers(), n, func(raster.Job) error { return nil })
	p.Start(context.Background())
	defer p.Close()
	for i := 0; i < n; i++ {
		if err := p.Submit(raster.Job{Coord: frame.TileCoord{Col: int32(i)}}); err != nil {
			t.Fatalf("Submit: %v", err)
		}
	}
	ch := make(chan frame.TileCoord, n)
	for w := 0; w < 4; w++ {
		go func() {
			for _, d := range collect(t, p, n/4) {
				ch <- d.Coord
			}
		}()
	}
	seen := make(map[frame.TileCoord]int, n)
	deadline := time.After(testDeadline)
	for i := 0; i < n; i++ {
		select {
		case c := <-ch:
			seen[c]++
		case <-deadline:
			t.Fatalf("collected %d of %d results from four waiters", i, n)
		}
	}
	for i := 0; i < n; i++ {
		c := frame.TileCoord{Col: int32(i)}
		if seen[c] != 1 {
			t.Fatalf("tile %v reported %d times across concurrent waiters", c, seen[c])
		}
	}
}

func TestWaitZeroAndStatsAreUsableBeforeStart(t *testing.T) {
	// A tool that constructs a pool and never starts it must still be able to ask
	// for a summary, and Wait(0) is the non-blocking form that claims nothing.
	p := raster.New(4, 8, nil)
	if got := len(p.Wait(0)); got != 0 {
		t.Fatalf("Wait(0) returned %d results", got)
	}
	if st := p.Stats(); st.Workers != 4 || st.Queued != 0 {
		t.Fatalf("stats on a fresh pool = %+v", st)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close on an unstarted pool: %v", err)
	}
}

// TestPoolRasterizesRealTilesIntoGridBuffers is the one place before the
// scheduler where the whole raster side runs together: real buffers from a real
// grid, a real frozen display list, real glyph masks, and results the grid
// accepts back through MarkValid.
func TestPoolRasterizesRealTilesIntoGridBuffers(t *testing.T) {
	f, g := mustFonts(t)
	const cols, rows = 3, 3
	l, dl := newLayer(t, cols, rows)
	p := raster.New(raster.DefaultWorkers(), 32, raster.DefaultRaster(f, g))
	p.Start(context.Background())
	defer p.Close()

	total := 0
	for row := int32(0); row < rows; row++ {
		for col := int32(0); col < cols; col++ {
			c := frame.TileCoord{Col: col, Row: row}
			out, ok := l.Grid.Acquire(c)
			if !ok {
				t.Fatalf("Acquire(%v) refused", c)
			}
			if err := p.Submit(raster.Job{Layer: l, Coord: c, DL: dl, Bounds: c.Rect(frame.TileSize), Out: out}); err != nil {
				t.Fatalf("Submit: %v", err)
			}
			total++
		}
	}
	for _, d := range collect(t, p, total) {
		if d.Err != nil {
			t.Fatalf("tile %v: %v", d.Coord, d.Err)
		}
		if !l.Grid.MarkValid(d.Coord, l.ContentVersion, d.Out) {
			t.Fatalf("MarkValid rejected the current result for %v", d.Coord)
		}
	}
	if st := l.Stats(); st.Valid != total {
		t.Fatalf("Valid = %d, want %d", st.Valid, total)
	} else if st.Bytes > st.Budget {
		t.Fatalf("tile bytes %d over budget %d", st.Bytes, st.Budget)
	}

	// A worker that mixed up its buffers would still produce total pixels, so the
	// proof is per-tile agreement with a single-threaded raster of that same tile.
	for _, c := range []frame.TileCoord{{Col: 0, Row: 0}, {Col: 1, Row: 0}, {Col: 2, Row: 2}} {
		ref := newTile()
		if err := raster.RasterizeTile(dl, c.Rect(frame.TileSize), ref, f, g); err != nil {
			t.Fatalf("RasterizeTile: %v", err)
		}
		got := l.Grid.Pixels(c)
		if got == nil {
			t.Fatalf("tile %v holds no pixels after a successful raster", c)
		}
		if hashTile(got) != hashTile(ref) {
			t.Fatalf("tile %v disagrees with a direct raster: the pool handed it the wrong job", c)
		}
	}
	if got := p.Stats().Rasterized; got != int64(total) {
		t.Fatalf("Rasterized = %d, want %d", got, total)
	}
}

// newLayer returns a layer over a cols x rows tile grid whose single command
// fills the whole extent, with a budget that holds every tile at once.
func newLayer(t *testing.T, cols, rows int32) (*frame.Layer, *paint.LayerDL) {
	t.Helper()
	bounds := frame.Rect4(0, 0, cols*frame.TileSize, rows*frame.TileSize)
	bp := frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, int(cols*rows))
	l := frame.NewLayer(frame.LayerID(7), bounds, int64(cols*rows)*frame.TileSizeBytes(), bp)
	list := paint.NewList(3)
	list.Append(paint.DisplayCmd{Kind: paint.CmdFill, Rect: bounds, Color: frame.RGB(255, 255, 255)})
	list.Append(textCmd("Goosie raster pool", 20, 100, 16))
	list.Append(paint.DisplayCmd{Kind: paint.CmdBorder, Rect: frame.Rect4(8, 8, 700, 700),
		Border: paint.BorderSpec{Top: paint.SideSpec{Width: 2, Color: frame.RGB(0, 0, 0)}}})
	dl := list.Build(3).Publish()
	l.SetContent(dl)
	return l, dl
}
