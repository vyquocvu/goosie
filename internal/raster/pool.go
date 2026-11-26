package raster

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

var (
	ErrPoolClosed = errors.New("raster: pool is closed")
	ErrQueueFull  = errors.New("raster: job queue is full")
)

var ErrTilePanic = errors.New("raster: tile rasterizer panicked")

var ErrNoRasterizer = errors.New("raster: pool has no rasterizer")

type Job struct {
	Layer             *frame.Layer
	Coord             frame.TileCoord
	DL                *paint.LayerDL
	Bounds            frame.Rect
	Out               *frame.Bitmap
	Background        frame.Color
	BackgroundVersion uint64
	Scratch           *frame.BitmapPool
}

type Done struct {
	Coord             frame.TileCoord
	LayerID           frame.LayerID
	Layer             *frame.Layer
	Out               *frame.Bitmap
	Err               error
	Version           uint64
	BackgroundVersion uint64
}

type RasterFunc func(j Job) error

func DefaultRaster(f *Fonts, g *GlyphAtlas) RasterFunc {
	return func(j Job) error {
		return rasterizeTile(j.DL, j.Bounds, j.Out, f, g, j.Background, j.Scratch)
	}
}

type PoolStats struct {
	Workers        int
	Queued         int
	Rasterized     int64
	Failed         int64
	Panics         int64
	Refused        int64
	DroppedResults int64
	Outstanding    int64
}

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

	closed      atomic.Bool
	outstanding atomic.Int64

	rasterized atomic.Int64
	failed     atomic.Int64
	panics     atomic.Int64
	refused    atomic.Int64
	dropped    atomic.Int64
}

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

func (p *Pool) run(j Job) {
	d := Done{Coord: j.Coord, Out: j.Out, Layer: j.Layer, BackgroundVersion: j.BackgroundVersion}
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
	select {
	case p.done <- d:
	default:
		p.dropped.Add(1)
	}
}

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

func (p *Pool) collected(n int) {
	if n <= 0 {
		return
	}
	p.outstanding.Add(-int64(n))
}

func (p *Pool) Poll() (Done, bool) {
	select {
	case d := <-p.done:
		p.collected(1)
		return d, true
	default:
		return Done{}, false
	}
}

func (p *Pool) Wait(n int) []Done {
	if n <= 0 {
		return nil
	}
	out := make([]Done, 0, min(n, 64))
	defer func() { p.collected(len(out)) }()
	for len(out) < n {
		select {
		case d := <-p.done:
			out = append(out, d)
		case <-p.ctx.Done():
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

func (p *Pool) Close() error {
	p.closeOnce.Do(func() {
		p.closed.Store(true)
		p.cancel()
		for {
			select {
			case j := <-p.jobs:
				d := Done{
					Coord:             j.Coord,
					Layer:             j.Layer,
					Out:               j.Out,
					Err:               context.Canceled,
					BackgroundVersion: j.BackgroundVersion,
				}
				if j.Layer != nil {
					d.LayerID = j.Layer.ID
				}
				if j.DL != nil {
					d.Version = j.DL.Version()
				}
				select {
				case p.done <- d:
				default:
				}
			default:
				p.wg.Wait()
				return
			}
		}
	})
	return nil
}
