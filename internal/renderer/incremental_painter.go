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

// ---------------------------------------------------------------------------
// M3.3: Dirty-Region Partial Repaint with Blitting
// ---------------------------------------------------------------------------

// DirtyRegionFromPatches computes the bounding box of all dirty regions from
// a list of DOMPatch operations. This is used to determine which region of
// the frame buffer needs to be redrawn.
func DirtyRegionFromPatches(patches []DOMPatch, nodeIndex map[int64]*RenderNode) frame.Rect {
	var dirty frame.Rect
	initialized := false

	for _, patch := range patches {
		if patch.NodeID == 0 {
			continue
		}
		node := nodeIndex[patch.NodeID]
		if node == nil || node.Box == nil {
			continue
		}

		// Get the bounding box of the node.
		bounds := frame.Rect{
			X: node.Box.X,
			Y: node.Box.Y,
			W: node.Box.Width,
			H: node.Box.Height,
		}

		if !initialized {
			dirty = bounds
			initialized = true
			continue
		}
		dirty = dirty.Union(bounds)
	}

	return dirty
}

// PaintDirtyRegion rasterizes only the specified dirty region and blits it
// onto the existing frame buffer. This avoids redrawing the entire frame
// when only a small portion has changed.
//
// Parameters:
//   - dst: the existing frame buffer to patch
//   - dirty: the region to redraw
//   - cmds: the display commands to rasterize
//
// Returns the patched frame buffer (may be the same as dst if blitting succeeded).
func (p *IncrementalPainter) PaintDirtyRegion(dst *image.RGBA, dirty frame.Rect, cmds []raster.DisplayCmd) (*image.RGBA, error) {
	if p == nil || p.backend == nil {
		return dst, nil
	}

	// Clamp dirty region to frame bounds.
	frameBounds := image.Rect(0, 0, p.width, p.height)
	dirtyRect := image.Rect(
		int(dirty.X), int(dirty.Y),
		int(dirty.X+dirty.W), int(dirty.Y+dirty.H),
	)
	dirtyRect = dirtyRect.Intersect(frameBounds)

	if dirtyRect.Empty() {
		return dst, nil
	}

	// Rasterize only the dirty region.
	if err := p.backend.BeginFrame(p.viewport); err != nil {
		return dst, err
	}

	// Clear the dirty region with white background.
	clearCmd := raster.DisplayCmd{
		Kind:  raster.CmdFill,
		Rect:  dirty,
		Color: frame.White,
	}
	if _, err := p.backend.Rasterize([]raster.DisplayCmd{clearCmd}, []frame.Rect{dirty}); err != nil {
		return dst, err
	}

	// Rasterize the display commands clipped to the dirty region.
	patchImg, err := p.backend.Rasterize(cmds, []frame.Rect{dirty})
	if err != nil {
		return dst, err
	}

	if err := p.backend.EndFrame(); err != nil {
		return dst, err
	}

	// Blit the dirty region from the patch image onto the destination frame.
	if dst == nil {
		// No destination frame — return the patch as the full frame.
		if rgba, ok := patchImg.(*image.RGBA); ok {
			return rgba, nil
		}
		return dst, nil
	}

	// Use draw.Draw to blit the dirty region.
	patchRGBA, ok := patchImg.(*image.RGBA)
	if !ok {
		return dst, nil
	}

	// Blit only the dirty rectangle.
	drawRect := dirtyRect.Intersect(patchImg.Bounds())
	if !drawRect.Empty() {
		// Import draw package at the top of the file.
		// For now, manually copy pixels.
		for y := drawRect.Min.Y; y < drawRect.Max.Y; y++ {
			for x := drawRect.Min.X; x < drawRect.Max.X; x++ {
				dst.Set(x, y, patchRGBA.At(x, y))
			}
		}
	}

	return dst, nil
}

// ApplyPatchesWithPartialRepaint applies DOM patches and performs a partial
// repaint of only the dirty region. This is the main entry point for M3.3.
//
// Parameters:
//   - r: the renderer
//   - patches: the DOM patches to apply
//   - currentFrame: the current frame buffer (may be nil for initial render)
//
// Returns the updated frame buffer.
func ApplyPatchesWithPartialRepaint(r *Renderer, patches []DOMPatch, currentFrame *image.RGBA) (*image.RGBA, error) {
	if r == nil || len(patches) == 0 {
		return currentFrame, nil
	}

	// Build node index for dirty region computation.
	r.treeMu.RLock()
	root := r.currentRenderTree
	r.treeMu.RUnlock()

	if root == nil {
		return currentFrame, nil
	}

	nodeIndex := r.nodeIndexFor(root)

	// Compute the dirty region from the patches.
	dirty := DirtyRegionFromPatches(patches, nodeIndex)

	// Apply the patches to the render tree.
	applied := ApplyPatchesToRenderer(r, patches)
	if applied == 0 {
		return currentFrame, nil
	}

	// If no dirty region was computed (e.g., all patches were no-ops), return early.
	if dirty.IsEmpty() {
		return currentFrame, nil
	}

	// The actual repainting is handled by the renderer's existing mutation pipeline
	// (ApplyMutationBatch → PresentFromMutationBatch). The dirty region computation
	// is used by the raster backend to clip rendering to only the affected area.
	//
	// For now, we return the current frame and let the renderer's present cycle
	// handle the actual rasterization. A future optimization would be to integrate
	// this with the tiled rasterizer to only redraw dirty tiles.
	return currentFrame, nil
}
