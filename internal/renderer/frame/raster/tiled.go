// Package raster provides tiled parallel rasterization.
//
// M2.1: Divides the document into horizontal tiles and rasterizes them
// in parallel using goroutines (up to runtime.GOMAXPROCS). Each tile is
// rasterized into a buffer from BufferPool, then composited into the
// final frame. This achieves near-linear speedup on multi-core machines.
package raster

import (
	"image"
	"image/draw"
	"runtime"
	"sync"

	"github.com/vyquocvu/goosie/internal/renderer/frame"
)

// DefaultTileHeight is the default height of each horizontal tile in
// device pixels. 1024px balances parallelism against per-tile overhead.
const DefaultTileHeight = 1024

// TiledRasterizer splits a frame into horizontal tiles and rasterizes
// them in parallel across N goroutines.
type TiledRasterizer struct {
	// backend is a prototype backend used to determine command processing.
	// Each worker creates its own CPUBackend for its tile.
	pool *BufferPool

	// tileHeight is the height of each horizontal tile strip.
	tileHeight int

	// maxWorkers caps the number of parallel goroutines. Defaults to
	// runtime.GOMAXPROCS(0).
	maxWorkers int
}

// TiledOption configures a TiledRasterizer.
type TiledOption interface {
	apply(*tiledConfig)
}

type tiledFuncOption func(*tiledConfig)

func (f tiledFuncOption) apply(cfg *tiledConfig) { f(cfg) }

type tiledConfig struct {
	tileHeight int
	maxWorkers int
	pool       *BufferPool
}

// WithTileHeight sets the height of each tile strip.
func WithTileHeight(h int) TiledOption {
	return tiledFuncOption(func(cfg *tiledConfig) {
		if h > 0 {
			cfg.tileHeight = h
		}
	})
}

// WithMaxWorkers caps the number of parallel rasterization goroutines.
func WithMaxWorkers(n int) TiledOption {
	return tiledFuncOption(func(cfg *tiledConfig) {
		if n > 0 {
			cfg.maxWorkers = n
		}
	})
}

// WithPool sets the buffer pool to use for tile buffers.
func WithPool(p *BufferPool) TiledOption {
	return tiledFuncOption(func(cfg *tiledConfig) {
		cfg.pool = p
	})
}

// NewTiledRasterizer creates a tiled parallel rasterizer.
func NewTiledRasterizer(opts ...TiledOption) *TiledRasterizer {
	cfg := tiledConfig{
		tileHeight: DefaultTileHeight,
		maxWorkers: runtime.GOMAXPROCS(0),
		pool:       GlobalBufferPool(),
	}
	for _, o := range opts {
		o.apply(&cfg)
	}
	return &TiledRasterizer{
		pool:       cfg.pool,
		tileHeight: cfg.tileHeight,
		maxWorkers: cfg.maxWorkers,
	}
}

// tileJob describes a single tile to rasterize.
type tileJob struct {
	index  int        // tile index (0-based, top to bottom)
	yStart int        // top edge in device pixels
	yEnd   int        // bottom edge in device pixels (exclusive)
	width  int        // frame width in device pixels
	height int        // full frame height in device pixels
	cmds   []DisplayCmd
	dirty  []frame.Rect
}

// tileResult holds the output of a single tile rasterization.
type tileResult struct {
	index int
	img   *image.RGBA
	err   error
}

// RasterizeParallel rasterizes the given commands into a full-frame
// *image.RGBA by splitting the frame into horizontal tiles and
// processing them in parallel. The returned image covers the full
// viewport (0,0)-(width,height).
//
// If the frame is small enough for a single tile, or maxWorkers <= 1,
// it falls back to single-threaded rasterization via CPUBackend.
func (tr *TiledRasterizer) RasterizeParallel(
	width, height int,
	cmds []DisplayCmd,
	dirty []frame.Rect,
	vp frame.Viewport,
) (*image.RGBA, error) {
	if width <= 0 {
		width = 1
	}
	if height <= 0 {
		height = 1
	}

	// Calculate number of tiles
	numTiles := (height + tr.tileHeight - 1) / tr.tileHeight
	if numTiles <= 1 || tr.maxWorkers <= 1 {
		return tr.rasterizeSingle(width, height, cmds, dirty, vp)
	}

	// Build tile jobs
	jobs := make([]tileJob, 0, numTiles)
	for i := 0; i < numTiles; i++ {
		yStart := i * tr.tileHeight
		yEnd := yStart + tr.tileHeight
		if yEnd > height {
			yEnd = height
		}
		jobs = append(jobs, tileJob{
			index:  i,
			yStart: yStart,
			yEnd:   yEnd,
			width:  width,
			height: height,
			cmds:   cmds,
			dirty:  dirty,
		})
	}

	// Determine worker count: min(numTiles, maxWorkers)
	workers := tr.maxWorkers
	if workers > numTiles {
		workers = numTiles
	}

	// Dispatch workers
	jobCh := make(chan tileJob, len(jobs))
	resultCh := make(chan tileResult, len(jobs))

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tr.tileWorker(jobCh, resultCh, vp)
		}()
	}

	// Feed jobs
	for _, j := range jobs {
		jobCh <- j
	}
	close(jobCh)

	// Wait for workers to finish, then close results
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collect results and composite into final frame
	result := tr.pool.Get(width, height)
	for res := range resultCh {
		if res.err != nil {
			// On error, return what we have so far. In production this
			// should log and fall back to full single-threaded raster.
			tr.pool.Put(result)
			return nil, res.err
		}
		if res.img != nil {
			// Blit tile into the final frame at the correct Y offset
			tileY := res.index * tr.tileHeight
			srcBounds := res.img.Bounds()
			dstRect := image.Rect(0, tileY, width, tileY+srcBounds.Dy())
			draw.Draw(result, dstRect, res.img, srcBounds.Min, draw.Src)
			tr.pool.Put(res.img)
		}
	}

	return result, nil
}

// tileWorker processes tile jobs from jobCh and sends results to resultCh.
func (tr *TiledRasterizer) tileWorker(jobCh <-chan tileJob, resultCh chan<- tileResult, vp frame.Viewport) {
	for job := range jobCh {
		img, err := tr.rasterizeTile(job, vp)
		resultCh <- tileResult{
			index: job.index,
			img:   img,
			err:   err,
		}
	}
}

// rasterizeTile rasterizes a single tile into a buffer from the pool.
func (tr *TiledRasterizer) rasterizeTile(job tileJob, vp frame.Viewport) (*image.RGBA, error) {
	tileH := job.yEnd - job.yStart
	if tileH <= 0 {
		return nil, nil
	}

	// Create a CPU backend for this tile
	backend := NewCPUBackend(job.width, tileH)
	defer backend.Close()

	// Set up a viewport for this tile
	tileVP := frame.NewViewport(
		float32(job.width),
		float32(tileH),
		vp.PixelScale,
	)

	if err := backend.BeginFrame(tileVP); err != nil {
		return nil, err
	}

	// Filter commands to those that intersect this tile's Y range,
	// adjusting their Y coordinates to be relative to the tile.
	tileCmds := filterAndShiftCommands(job.cmds, job.yStart, job.yEnd)

	// Compute dirty regions relative to this tile
	var tileDirty []frame.Rect
	if len(job.dirty) > 0 {
		for _, d := range job.dirty {
			tileRect := frame.Rect{
				X: d.X,
				Y: d.Y - float32(job.yStart),
				W: d.W,
				H: d.H,
			}
			// Clip to tile bounds
			if tileRect.Y < 0 {
				tileRect.H += tileRect.Y
				tileRect.Y = 0
			}
			if tileRect.Y+tileRect.H > float32(tileH) {
				tileRect.H = float32(tileH) - tileRect.Y
			}
			if tileRect.H > 0 && tileRect.W > 0 {
				tileDirty = append(tileDirty, tileRect)
			}
		}
	}

	rasterImg, err := backend.Rasterize(tileCmds, tileDirty)
	if err != nil {
		return nil, err
	}
	if err := backend.EndFrame(); err != nil {
		return nil, err
	}

	// Copy the result into a pooled buffer
	tileBuf := tr.pool.Get(job.width, tileH)
	if src, ok := rasterImg.(*image.RGBA); ok && src != nil {
		draw.Draw(tileBuf, tileBuf.Bounds(), src, src.Bounds().Min, draw.Src)
	}

	return tileBuf, nil
}

// filterAndShiftCommands filters display commands to those intersecting
// the tile's Y range [yStart, yEnd) and shifts their Y coordinates to
// be relative to yStart.
func filterAndShiftCommands(cmds []DisplayCmd, yStart, yEnd int) []DisplayCmd {
	var out []DisplayCmd
	yStartF := float32(yStart)
	yEndF := float32(yEnd)

	for _, cmd := range cmds {
		cmdTop := cmd.Rect.Y
		cmdBottom := cmd.Rect.Y + cmd.Rect.H

		// Skip commands entirely outside the tile
		if cmdBottom <= yStartF || cmdTop >= yEndF {
			continue
		}

		// Clone the command and shift Y
		shifted := cmd
		shifted.Rect.Y -= yStartF

		// Adjust other Y-bearing fields
		switch shifted.Kind {
		case CmdBorder:
			// Border rect also needs shifting (already done via Rect)
		case CmdText:
			shifted.TextRun.Color = shifted.TextRun.Color // no-op, text run Y is in glyphs
		case CmdImage:
			// Image rect already shifted
		}

		out = append(out, shifted)
	}
	return out
}

// rasterizeSingle is the single-threaded fallback for small frames.
func (tr *TiledRasterizer) rasterizeSingle(
	width, height int,
	cmds []DisplayCmd,
	dirty []frame.Rect,
	vp frame.Viewport,
) (*image.RGBA, error) {
	backend := NewCPUBackend(width, height)
	defer backend.Close()

	if err := backend.BeginFrame(vp); err != nil {
		return nil, err
	}
	img, err := backend.Rasterize(cmds, dirty)
	if err != nil {
		return nil, err
	}
	if err := backend.EndFrame(); err != nil {
		return nil, err
	}

	if rgba, ok := img.(*image.RGBA); ok {
		return rgba, nil
	}

	// Copy into a new RGBA buffer
	result := tr.pool.Get(width, height)
	draw.Draw(result, result.Bounds(), img, img.Bounds().Min, draw.Src)
	return result, nil
}
