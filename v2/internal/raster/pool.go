package raster

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/vyquocvu/goosie/v2/internal/frame"
	"github.com/vyquocvu/goosie/v2/internal/paint"
)

// ErrPoolClosed and ErrQueueFull are the two ways a submit can be refused. Both
// are ordinary outcomes rather than failures: a full queue means the rasterizers
// are behind, and the correct response is to present the frame with the tiles
// that are ready and re-ask next vsync. Blocking here would put tile raster on
// the UI thread, which is invariant 2.
var (
	ErrPoolClosed = errors.New("raster: pool is closed")
	ErrQueueFull  = errors.New("raster: job queue is full")
)

// ErrTilePanic replaces a panic raised inside one job. The distinction survives
// because it has to: a tile whose rasterizer panicked is retried once and then
// rendered blank with a counter, while a tile that returned an error may simply
// be waiting on something that has not arrived yet.
var ErrTilePanic = errors.New("raster: tile rasterizer panicked")

// ErrNoRasterizer is what a job submitted to a pool built without a rasterizer
// reports. Returning an error rather than panicking keeps a construction mistake
// observable through the same channel as every other tile failure.
var ErrNoRasterizer = errors.New("raster: pool has no rasterizer")

// Job is one tile to paint. Out is a buffer the caller already acquired from the
// grid's pool, so a worker never allocates a tile and never has to know where
// buffers come from.
type Job struct {
	Layer  *frame.Layer
	Coord  frame.TileCoord
	DL     *paint.LayerDL
	Bounds frame.Rect
	Out    *frame.Bitmap
}

// Done is one job's result. Out is the same buffer the job carried, whether the
// raster succeeded or not: it is the caller's, and on failure the caller decides
// whether to release it or keep the stale pixels.
//
// Version is the content version those pixels depict, read off the job's frozen
// display list rather than taken from the layer the caller still holds. The
// difference is the whole point: content can bump while a job runs, and a caller
// that labelled the older rendering current would mark a tile valid that it still
// owes a raster for, permanently.
type Done struct {
	Coord   frame.TileCoord
	LayerID frame.LayerID
	Out     *frame.Bitmap
	Err     error
	// Version is 0 for a job that carried no display list, which no caller may mark
	// valid at any real version.
	Version uint64
}

// RasterFunc paints one tile. The pool takes a function rather than calling
// RasterizeTile directly so that it stays independent of the fonts, the atlas,
// and the display-list format, which is also what lets a test hand it a
// rasterizer that panics on purpose.
type RasterFunc func(j Job) error

// DefaultRaster returns the RasterFunc the frame path uses.
func DefaultRaster(f *Fonts, g *GlyphAtlas) RasterFunc {
	return func(j Job) error {
		return RasterizeTile(j.DL, j.Bounds, j.Out, f, g)
	}
}

// PoolStats is a snapshot of what the pool did. Rasterized and Failed together
// are the jobs a worker ran; Refused is the submissions that never reached one;
// Panics is the subset of Failed that recovered a panic. Rasterized therefore
// measures work completed rather than traffic offered, which is what the frame
// gate asserts on.
//
// DroppedResults is kept apart from Refused on purpose. A dropped result is a job
// that did run, so folding the two together would let the frame path lose a tile
// buffer into a worker that will never report and still read a clean zero for
// refused work.
type PoolStats struct {
	Workers    int
	Queued     int
	Rasterized int64
	Failed     int64
	Panics     int64
	Refused    int64
	// DroppedResults counts finished jobs the completion queue could not hold, and is
	// zero by construction: Submit admits a job only while a result slot is free, so
	// the queue can never be asked for more than it was sized to hold. A non-zero
	// value means that admission control was broken, and the affected tile would stay
	// claimed by a silent worker forever.
	DroppedResults int64
	// Outstanding is how many admitted jobs have not been collected yet - queued,
	// running, or finished and waiting in the completion queue. It is capped at
	// queueDepth+workers by Submit, which is the same thing as saying the pool will
	// rather refuse work than lose a result.
	Outstanding int64
}

// Pool is the raster side of the frame path: a fixed set of workers fed by a
// queue the UI thread can always write to without waiting.
//
// The counters are atomic rather than mutex-guarded, and the reason is the same
// invariant the channels are there for. A mutex held by a worker that is finishing
// a tile can put the UI thread to sleep for the length of a raster - and when it
// does, the runtime allocates a semaphore ticket for the waiter, which the frame
// gate in v2/test/gate counts against the frame path it cannot tell apart from a
// buffer. Neither is acceptable at 60fps, and neither is what a counter is worth.
type Pool struct {
	raster  RasterFunc
	workers int

	jobs chan Job
	done chan Done

	ctx       context.Context
	cancel    context.CancelFunc
	start     sync.Once
	wg        sync.WaitGroup
	closeOnce sync.Once

	slots int

	// closed and outstanding are the pool's two decisions. The rest are tallies.
	//
	// outstanding is exact for the pool as the frame path uses it, which is one
	// submitting goroutine: a check-then-add from several submitters could overshoot
	// slots by their count, and the channel's own capacity is the hard bound either
	// way. A second submitter would need a CAS loop here, not a mutex.
	closed      atomic.Bool
	outstanding atomic.Int64

	rasterized atomic.Int64
	failed     atomic.Int64
	panics     atomic.Int64
	refused    atomic.Int64
	dropped    atomic.Int64
}

// DefaultWorkers is how many raster threads a pool gets when nobody says: all
// but the one core the UI thread is spinning on, capped low enough that a
// 128-core CI box does not build a queue nobody can drain in one vsync.
func DefaultWorkers() int {
	n := runtime.GOMAXPROCS(0) - 1
	if n < 1 {
		n = 1
	}
	if n > 4 {
		n = 4
	}
	return n
}

// New returns an unstarted pool. queueDepth is how many tiles may wait to be
// painted; a frame's worth of newly visible tiles is the sensible value, because
// anything beyond that is work for a viewport the user may have already scrolled
// past.
func New(workers int, queueDepth int, raster RasterFunc) *Pool {
	if workers <= 0 {
		workers = DefaultWorkers()
	}
	if queueDepth <= 0 {
		queueDepth = workers * 32
	}
	if raster == nil {
		raster = func(Job) error { return ErrNoRasterizer }
	}
	ctx, cancel := context.WithCancel(context.Background())
	// The completion queue holds one slot per job that can exist: at most queueDepth
	// are queued and at most workers are running, and Submit refuses rather than
	// waiting once that many results are uncollected. Sizing it that way is
	// deliberate rather than generous - a dropped result is unrecoverable, since the
	// tile stays claimed by a worker that will never report and its buffer never
	// returns to the pool. Making the drop impossible costs one admission check on a
	// call that is already refusing on queue depth, and turns "the UI thread stopped
	// polling" into refused submissions, which the frame path already handles.
	slots := queueDepth + workers
	p := &Pool{
		raster:  raster,
		workers: workers,
		jobs:    make(chan Job, queueDepth),
		done:    make(chan Done, slots),
		slots:   slots,
		ctx:     ctx,
		cancel:  cancel,
	}
	return p
}

// Start launches the workers. It is idempotent, and the first call's context is
// the one that governs them: when it is cancelled the pool closes itself, so a
// caller that owns a document lifetime needs no separate teardown call.
func (p *Pool) Start(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	p.start.Do(func() {
		p.wg.Add(p.workers)
		for i := 0; i < p.workers; i++ {
			go p.work(ctx)
		}
		if done := ctx.Done(); done != nil {
			go func() {
				select {
				case <-done:
					p.Close()
				case <-p.ctx.Done():
				}
			}()
		}
	})
}

// Workers returns the configured worker count.
func (p *Pool) Workers() int { return p.workers }

func (p *Pool) work(ctx context.Context) {
	defer p.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.ctx.Done():
			return
		case j, ok := <-p.jobs:
			if !ok {
				return
			}
			p.run(j)
		}
	}
}

// run executes one job with the recovery that keeps a panic inside a single
// tile. Without it, one bad display list would end every worker goroutine and
// the page would stop painting with no diagnostic at all.
func (p *Pool) run(j Job) {
	d := Done{Coord: j.Coord, Out: j.Out}
	if j.Layer != nil {
		d.LayerID = j.Layer.ID
	}
	if j.DL != nil {
		d.Version = j.DL.Version()
	}
	err := func() (err error) {
		defer func() {
			if v := recover(); v != nil {
				p.panics.Add(1)
				err = ErrTilePanic
			}
		}()
		return p.raster(j)
	}()
	d.Err = err
	if err == nil {
		p.rasterized.Add(1)
	} else {
		p.failed.Add(1)
	}
	// A result the completion queue cannot hold is dropped rather than waited
	// on: the buffer belongs to the caller, who will find the tile still stale and
	// ask again, and a worker blocked on a UI thread that stopped polling is
	// exactly the coupling this pool exists to avoid.
	select {
	case p.done <- d:
	default:
		p.dropped.Add(1)
	}
}

// Submit hands a tile to the pool without ever blocking. A full queue is
// reported, not waited on: the tile stays stale and the next frame asks again,
// which is the difference between a hitch and a stall. So is a full result
// window - a caller that cannot collect any faster than this has no use for a
// fifth tile, and would have had it handed back as a dropped result instead.
func (p *Pool) Submit(j Job) error {
	if p.closed.Load() {
		p.refused.Add(1)
		return ErrPoolClosed
	}
	if p.outstanding.Load() >= int64(p.slots) {
		p.refused.Add(1)
		return ErrQueueFull
	}
	select {
	case p.jobs <- j:
		p.outstanding.Add(1)
		return nil
	default:
		p.refused.Add(1)
		return ErrQueueFull
	}
}

// collected accounts for n results taken out of the completion queue, freeing the
// result slots their submissions reserved.
func (p *Pool) collected(n int) {
	if n <= 0 {
		return
	}
	p.outstanding.Add(-int64(n))
}

// Poll takes one finished job if there is one, and never waits. This is the
// shape invariant 2 requires of the UI thread: it can collect results, but there
// is no call it can make that puts a raster behind a present.
func (p *Pool) Poll() (Done, bool) {
	select {
	case d := <-p.done:
		p.collected(1)
		return d, true
	default:
		return Done{}, false
	}
}

// Wait collects up to n results, blocking until either n have arrived or the
// pool is closing. It exists for tools and tests that want a deterministic end
// state; the frame path uses Poll, because invariant 2 forbids waiting on raster.
//
// Each call returns its own slice. A reused buffer was tried first, on the theory
// that Wait is off the hot path anyway, and it is quietly wrong: two waiters that
// serialise cleanly still each hand back the same array, so the second refill
// rewrites the tiles the first is about to read, and the results are lost without
// a race to point at.
func (p *Pool) Wait(n int) []Done {
	if n <= 0 {
		return nil
	}
	// Capped growth rather than make([]Done, n): n is a caller's guess at how many
	// results exist, and a pool must not be talked into a huge allocation by one.
	out := make([]Done, 0, min(n, 64))
	defer func() { p.collected(len(out)) }()
	for len(out) < n {
		select {
		case d := <-p.done:
			out = append(out, d)
		case <-p.ctx.Done():
			// Drain whatever already arrived so a caller that is tearing down can
			// release every buffer it handed out.
			for {
				select {
				case d := <-p.done:
					out = append(out, d)
				default:
					return out
				}
			}
		}
	}
	return out
}

// Stats reads the counters. Each is loaded atomically and the set is not a
// transaction: a worker can finish a tile between two of the loads, so Rasterized
// and Outstanding can disagree by one job. Nothing can fix that without stopping the
// workers to look, which is what this pool exists to avoid.
func (p *Pool) Stats() PoolStats {
	return PoolStats{
		Workers:        p.workers,
		Queued:         len(p.jobs),
		Rasterized:     p.rasterized.Load(),
		Failed:         p.failed.Load(),
		Panics:         p.panics.Load(),
		Refused:        p.refused.Load(),
		DroppedResults: p.dropped.Load(),
		Outstanding:    p.outstanding.Load(),
	}
}

// Close stops the workers and waits for them to leave. In-flight jobs finish;
// queued ones are dropped, which is correct because a dropped tile is simply a
// tile nobody asked for any more.
func (p *Pool) Close() error {
	p.closeOnce.Do(func() {
		p.closed.Store(true)
		p.cancel()
		p.wg.Wait()
	})
	return nil
}
