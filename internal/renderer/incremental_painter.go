package renderer

import (
	"image"

	"github.com/vyquocvu/goosie/internal/renderer/frame"
	"github.com/vyquocvu/goosie/internal/renderer/frame/raster"
)

// IncrementalPainter converts dirty PaintChunks into raster commands and lets
// the raster backend redraw only the affected rectangles instead of the full
// frame. The returned frame buffer is reused across paints and is owned by
// the raster backend.
type IncrementalPainter struct {
	backend       raster.Backend
	viewport      frame.Viewport
	width, height int
}

func NewIncrementalPainter(width, height int) (*IncrementalPainter, error) {
	backend := raster.NewCPUBackend(width, height)
	return &IncrementalPainter{
		backend:  backend,
		viewport: frame.NewViewport(float32(width), float32(height), frame.PixelScaleDefault),
		width:    width,
		height:   height,
	}, nil
}

func (p *IncrementalPainter) Close() error {
	if p == nil || p.backend == nil {
		return nil
	}
	return p.backend.Close()
}

func (p *IncrementalPainter) PaintDirty(chunks []PaintChunk, cmds []raster.DisplayCmd) (image.Image, error) {
	if p == nil || p.backend == nil {
		return nil, nil
	}
	dirty := DirtyRect(chunks)
	if err := p.backend.BeginFrame(p.viewport); err != nil {
		return nil, err
	}
	if !dirty.IsEmpty() {
		if err := p.paintBackground([]frame.Rect{dirty}); err != nil {
			return nil, err
		}
	}
	img, err := p.backend.Rasterize(cmds, []frame.Rect{dirty})
	if err != nil {
		return nil, err
	}
	if err := p.backend.EndFrame(); err != nil {
		return nil, err
	}
	return img, nil
}

func (p *IncrementalPainter) PaintFull(cmds []raster.DisplayCmd) (image.Image, error) {
	if p == nil || p.backend == nil {
		return nil, nil
	}
	if err := p.backend.BeginFrame(p.viewport); err != nil {
		return nil, err
	}
	img, err := p.backend.Rasterize(cmds, nil)
	if err != nil {
		return nil, err
	}
	if err := p.backend.EndFrame(); err != nil {
		return nil, err
	}
	return img, nil
}

func (p *IncrementalPainter) paintBackground(dirty []frame.Rect) error {
	if len(dirty) == 0 {
		return nil
	}
	for _, d := range dirty {
		cmd := raster.DisplayCmd{
			Kind:  raster.CmdFill,
			Rect:  d,
			Color: frame.White,
		}
		if _, err := p.backend.Rasterize([]raster.DisplayCmd{cmd}, []frame.Rect{d}); err != nil {
			return err
		}
	}
	return nil
}

// Present pushes the most recent frame buffer to the supplied Fyne adapter
// when one is configured. The painter owns the raster output; callers should
// invoke Present after PaintFull or PaintDirty.
func (p *IncrementalPainter) Present(adapter *FyneAdapter) {
	if p == nil || adapter == nil || p.backend == nil {
		return
	}
	if cpu, ok := p.backend.(*raster.CPUBackend); ok {
		adapter.PresentFrame(cpu.FrameBuffer())
	}
}

// DirtyRect unions chunk bounds into a single dirty rect for raster clipping.
func DirtyRect(chunks []PaintChunk) frame.Rect {
	var dirty frame.Rect
	initialized := false
	for _, chunk := range chunks {
		if !chunk.dirty {
			continue
		}
		bounds := frame.Rect{X: chunk.Bounds.X, Y: chunk.Bounds.Y, W: chunk.Bounds.W, H: chunk.Bounds.H}
		if !initialized {
			dirty = bounds
			initialized = true
			continue
		}
		dirty = dirty.Union(bounds)
	}
	return dirty
}
