package renderer

import (
	"image"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"github.com/vyquocvu/goosie/internal/renderer/frame"
	"github.com/vyquocvu/goosie/internal/renderer/frame/raster"
)

// FyneAdapter presents CPU-rasterized frame buffers through Fyne.
//
// M6.4: This adapter bridges the backend-neutral raster output (image.RGBA)
// to Fyne's canvas system. It maintains a single canvas.Image that is updated
// in-place when new frames arrive, avoiding widget tree rebuilds on scroll.
//
// UI-thread constraints:
//   - PresentFrame must be called on the Fyne UI thread (use fyne.Do).
//   - The adapter is safe for concurrent frame production — the CPU backend
//     renders off-thread, and PresentFrame publishes the result to Fyne.
type FyneAdapter struct {
	mu sync.Mutex

	// canvasImage is the single Fyne image object that displays the raster
	// output. It is created once and updated in-place via SetImage content.
	canvasImage *canvas.Image

	// contentRoot is the stable container holding the canvas image.
	contentRoot *fyne.Container

	// currentFrame holds the latest presented frame buffer.
	currentFrame *raster.FrameBuffer

	// viewport tracks the current viewport for scroll-only updates.
	viewport frame.Viewport

	// size tracks the adapter's allocated size.
	size fyne.Size
}

// NewFyneAdapter creates a Fyne presentation adapter.
func NewFyneAdapter() *FyneAdapter {
	img := canvas.NewImageFromImage(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	img.FillMode = canvas.ImageFillOriginal
	img.SetMinSize(fyne.NewSize(1, 1))

	root := container.NewWithoutLayout(img)
	return &FyneAdapter{
		canvasImage: img,
		contentRoot: root,
	}
}

// PresentFrame updates the displayed image with a new frame buffer.
// The frame buffer's image is set directly on the canvas.Image — no new
// Fyne objects are created. This must be called on the UI thread.
//
// If the frame dimensions changed, the min size is updated.
func (a *FyneAdapter) PresentFrame(fb *raster.FrameBuffer) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.currentFrame = fb
	bounds := fb.Bounds()
	w := float32(bounds.Dx())
	h := float32(bounds.Dy())

	a.canvasImage.Image = fb.Image()
	a.canvasImage.Refresh()
	a.canvasImage.SetMinSize(fyne.NewSize(w, h))
}

// Content returns the Fyne canvas object tree for embedding in a window.
// The returned object is stable — it is not rebuilt on scroll or frame updates.
func (a *FyneAdapter) Content() fyne.CanvasObject {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.contentRoot
}

// SetViewport updates the viewport for scroll-only updates.
// This does NOT rebuild the widget tree — the next PresentFrame call
// will update the image in-place.
func (a *FyneAdapter) SetViewport(vp frame.Viewport) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.viewport = vp
}

// Viewport returns the current viewport.
func (a *FyneAdapter) Viewport() frame.Viewport {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.viewport
}

// CurrentFrame returns the most recently presented frame buffer (or nil).
func (a *FyneAdapter) CurrentFrame() *raster.FrameBuffer {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.currentFrame
}

// Resize handles container resize events. Updates the canvas image size
// without rebuilding the widget tree.
func (a *FyneAdapter) Resize(size fyne.Size) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.size = size
	a.canvasImage.Resize(size)
}
