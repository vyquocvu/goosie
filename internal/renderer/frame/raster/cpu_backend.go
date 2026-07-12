// Package raster provides a pure-Go CPU raster backend.
//
// M6.2: Implements the RasterBackend interface that consumes backend-neutral
// display commands and produces pixel frame buffers. Supports solid fills,
// borders, clipping, opacity, and dirty-region-only rasterization.
package raster

import (
	"bytes"
	"encoding/xml"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
	"github.com/vyquocvu/goosie/internal/renderer/frame"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// ---------------------------------------------------------------------------
// RasterBackend interface
// ---------------------------------------------------------------------------

// Backend is the interface that any raster backend (CPU, GPU) must implement.
// M6 roadmap: BeginFrame → Rasterize → EndFrame → Close.
type Backend interface {
	// BeginFrame prepares the backend for a new frame with the given viewport.
	BeginFrame(vp frame.Viewport) error

	// Rasterize processes display commands within dirty regions and writes
	// pixels into the frame buffer. Returns the rendered image.
	Rasterize(cmds []DisplayCmd, dirty []frame.Rect) (image.Image, error)

	// EndFrame finalizes the frame. The returned image remains valid until
	// the next BeginFrame.
	EndFrame() error

	// Close releases all resources held by the backend.
	Close() error
}

// ---------------------------------------------------------------------------
// DisplayCmd — backend-neutral command for the raster backend
// ---------------------------------------------------------------------------

// CmdKind identifies the type of display command for the raster backend.
type CmdKind uint8

const (
	CmdFill        CmdKind = iota // Fill a rectangle with solid color
	CmdBorder                     // Draw per-side borders
	CmdClipPush                   // Push a clipping rectangle
	CmdClipPop                    // Pop the most recent clip
	CmdOpacityPush                // Push an opacity multiplier
	CmdOpacityPop                 // Pop the most recent opacity
	CmdText                       // Draw a shaped text run
	CmdImage                      // Draw a decoded image or SVG
)

// DisplayCmd is a single raster command.
type DisplayCmd struct {
	Kind    CmdKind
	Rect    frame.Rect    // For CmdFill, CmdBorder bounds, CmdClipPush, CmdText/CmdImage bounds
	Color   frame.Color   // For CmdFill, CmdText
	Border  BorderSpec    // For CmdBorder
	Opacity float32       // For CmdOpacityPush (0.0–1.0)
	TextRun frame.TextRun // For CmdText
	Image   ImageSpec     // For CmdImage
}

// ImageSpec holds image data or pre-decoded image.
type ImageSpec struct {
	Data []byte      // Raw image data (SVG, PNG, etc.)
	Img  image.Image // Pre-decoded image
}

// BorderSpec describes per-side border properties for the raster backend.
type BorderSpec struct {
	Top    SideSpec
	Right  SideSpec
	Bottom SideSpec
	Left   SideSpec
}

// SideSpec describes one side of a border.
type SideSpec struct {
	Width float32
	Color frame.Color
}

// ---------------------------------------------------------------------------
// FrameBuffer — reusable pixel buffer
// ---------------------------------------------------------------------------

// FrameBuffer wraps an image.RGBA with reuse semantics. The buffer is
// allocated once and reused across frames to avoid per-frame allocation.
type FrameBuffer struct {
	img    *image.RGBA
	width  int
	height int
}

// NewFrameBuffer creates a frame buffer with the given device-pixel dimensions.
func NewFrameBuffer(width, height int) *FrameBuffer {
	if width <= 0 {
		width = 1
	}
	if height <= 0 {
		height = 1
	}
	return &FrameBuffer{
		img:    image.NewRGBA(image.Rect(0, 0, width, height)),
		width:  width,
		height: height,
	}
}

// Image returns the underlying image.
func (fb *FrameBuffer) Image() *image.RGBA { return fb.img }

// Bounds returns the pixel bounds.
func (fb *FrameBuffer) Bounds() image.Rectangle { return fb.img.Bounds() }

// Reset clears the buffer to transparent black without reallocating.
func (fb *FrameBuffer) Reset() {
	for i := range fb.img.Pix {
		fb.img.Pix[i] = 0
	}
}

// Resize reallocates the buffer if the dimensions changed. Returns true if
// reallocation occurred.
func (fb *FrameBuffer) Resize(width, height int) bool {
	if width <= 0 {
		width = 1
	}
	if height <= 0 {
		height = 1
	}
	if width == fb.width && height == fb.height {
		return false
	}
	fb.width = width
	fb.height = height
	fb.img = image.NewRGBA(image.Rect(0, 0, width, height))
	return true
}

// ---------------------------------------------------------------------------
// CPUBackend — pure-Go CPU rasterizer
// ---------------------------------------------------------------------------

// CPUBackend is a pure-Go CPU raster backend that processes display commands
// and writes pixels into a FrameBuffer.
type CPUBackend struct {
	fb           *FrameBuffer
	viewport     frame.Viewport
	clipStack    []frame.Rect
	opacityStack []float32
	closed       bool
}

// NewCPUBackend creates a CPU raster backend with the given initial dimensions.
func NewCPUBackend(width, height int) *CPUBackend {
	return &CPUBackend{
		fb:           NewFrameBuffer(width, height),
		clipStack:    make([]frame.Rect, 0, 8),
		opacityStack: make([]float32, 0, 8),
	}
}

// BeginFrame prepares for a new frame.
func (b *CPUBackend) BeginFrame(vp frame.Viewport) error {
	if b.closed {
		return errBackendClosed
	}
	b.viewport = vp
	dw, dh := vp.DeviceSize()
	if dw <= 0 {
		dw = 1
	}
	if dh <= 0 {
		dh = 1
	}
	b.fb.Resize(int(dw), int(dh))
	b.fb.Reset()
	b.clipStack = b.clipStack[:0]
	b.opacityStack = b.opacityStack[:0]
	// Push initial clip to full viewport.
	b.clipStack = append(b.clipStack, frame.Rect{
		X: 0, Y: 0,
		W: float32(dw), H: float32(dh),
	})
	return nil
}

// Rasterize processes commands and writes pixels. Only pixels within dirty
// regions are written (if dirty is non-empty). Returns the frame buffer image.
func (b *CPUBackend) Rasterize(cmds []DisplayCmd, dirty []frame.Rect) (image.Image, error) {
	if b.closed {
		return nil, errBackendClosed
	}

	ps := b.viewport.PixelScale
	if ps.Scale == 0 {
		ps.Scale = 1
	}

	// Compute effective dirty region.
	var dirtyBounds frame.Rect
	if len(dirty) > 0 {
		dirtyBounds = dirty[0]
		for _, d := range dirty[1:] {
			dirtyBounds = dirtyBounds.Union(d)
		}
	} else {
		// No dirty regions = full frame.
		dw, dh := b.fb.width, b.fb.height
		dirtyBounds = frame.Rect{X: 0, Y: 0, W: float32(dw), H: float32(dh)}
	}

	currentOpacity := float32(1.0)

	for _, cmd := range cmds {
		switch cmd.Kind {
		case CmdClipPush:
			// Intersect with current clip.
			cur := b.clipStack[len(b.clipStack)-1]
			clipped := cmd.Rect.Intersection(cur)
			b.clipStack = append(b.clipStack, clipped)

		case CmdClipPop:
			if len(b.clipStack) > 1 {
				b.clipStack = b.clipStack[:len(b.clipStack)-1]
			}

		case CmdOpacityPush:
			b.opacityStack = append(b.opacityStack, currentOpacity)
			currentOpacity *= clampf32(cmd.Opacity, 0, 1)

		case CmdOpacityPop:
			if len(b.opacityStack) > 0 {
				currentOpacity = b.opacityStack[len(b.opacityStack)-1]
				b.opacityStack = b.opacityStack[:len(b.opacityStack)-1]
			}

		case CmdFill:
			clip := b.clipStack[len(b.clipStack)-1]
			fillRect := cmd.Rect.Intersection(clip).Intersection(dirtyBounds)
			if !fillRect.IsEmpty() {
				c := applyOpacity(cmd.Color, currentOpacity)
				b.rasterFill(fillRect, c, ps)
			}

		case CmdBorder:
			clip := b.clipStack[len(b.clipStack)-1]
			b.rasterBorder(cmd.Rect, cmd.Border, clip, dirtyBounds, currentOpacity, ps)

		case CmdText:
			clip := b.clipStack[len(b.clipStack)-1]
			b.rasterText(cmd.Rect, cmd.TextRun, clip, dirtyBounds, currentOpacity, ps)

		case CmdImage:
			clip := b.clipStack[len(b.clipStack)-1]
			b.rasterImage(cmd.Rect, cmd.Image, clip, dirtyBounds, currentOpacity, ps)
		}
	}

	return b.fb.Image(), nil
}

// EndFrame finalizes the current frame.
func (b *CPUBackend) EndFrame() error {
	if b.closed {
		return errBackendClosed
	}
	b.clipStack = b.clipStack[:0]
	b.opacityStack = b.opacityStack[:0]
	return nil
}

// Close releases resources.
func (b *CPUBackend) Close() error {
	b.closed = true
	b.fb = nil
	b.clipStack = nil
	b.opacityStack = nil
	return nil
}

// FrameBuffer returns the underlying frame buffer (for testing).
func (b *CPUBackend) FrameBuffer() *FrameBuffer { return b.fb }

// ---------------------------------------------------------------------------
// rasterFill — fills a rectangle in device pixels
// ---------------------------------------------------------------------------

func (b *CPUBackend) rasterFill(r frame.Rect, c frame.Color, ps frame.PixelScale) {
	x0 := int(math.Round(float64(r.X)))
	y0 := int(math.Round(float64(r.Y)))
	x1 := int(math.Round(float64(r.X + r.W)))
	y1 := int(math.Round(float64(r.Y + r.H)))

	// Clamp to buffer bounds.
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > b.fb.width {
		x1 = b.fb.width
	}
	if y1 > b.fb.height {
		y1 = b.fb.height
	}

	if x0 >= x1 || y0 >= y1 {
		return
	}

	srgba := c.StdColor()
	for y := y0; y < y1; y++ {
		offset := (y*b.fb.width + x0) * 4
		for x := x0; x < x1; x++ {
			blendPixel(b.fb.img.Pix, offset, srgba)
			offset += 4
		}
	}
}

// ---------------------------------------------------------------------------
// rasterBorder — draws per-side borders
// ---------------------------------------------------------------------------

func (b *CPUBackend) rasterBorder(bounds frame.Rect, border BorderSpec, clip, dirty frame.Rect, opacity float32, ps frame.PixelScale) {
	// Top border
	if border.Top.Width > 0 && !border.Top.Color.IsFullyTransparent() {
		r := frame.Rect{X: bounds.X, Y: bounds.Y, W: bounds.W, H: border.Top.Width}
		r = r.Intersection(clip).Intersection(dirty)
		if !r.IsEmpty() {
			b.rasterFill(r, applyOpacity(border.Top.Color, opacity), ps)
		}
	}
	// Bottom border
	if border.Bottom.Width > 0 && !border.Bottom.Color.IsFullyTransparent() {
		r := frame.Rect{X: bounds.X, Y: bounds.Y + bounds.H - border.Bottom.Width, W: bounds.W, H: border.Bottom.Width}
		r = r.Intersection(clip).Intersection(dirty)
		if !r.IsEmpty() {
			b.rasterFill(r, applyOpacity(border.Bottom.Color, opacity), ps)
		}
	}
	// Left border
	if border.Left.Width > 0 && !border.Left.Color.IsFullyTransparent() {
		r := frame.Rect{X: bounds.X, Y: bounds.Y, W: border.Left.Width, H: bounds.H}
		r = r.Intersection(clip).Intersection(dirty)
		if !r.IsEmpty() {
			b.rasterFill(r, applyOpacity(border.Left.Color, opacity), ps)
		}
	}
	// Right border
	if border.Right.Width > 0 && !border.Right.Color.IsFullyTransparent() {
		r := frame.Rect{X: bounds.X + bounds.W - border.Right.Width, Y: bounds.Y, W: border.Right.Width, H: bounds.H}
		r = r.Intersection(clip).Intersection(dirty)
		if !r.IsEmpty() {
			b.rasterFill(r, applyOpacity(border.Right.Color, opacity), ps)
		}
	}
}

// ---------------------------------------------------------------------------
// pixel helpers
// ---------------------------------------------------------------------------

// blendPixel alpha-blends src over the existing pixel at the given offset.
func blendPixel(pix []byte, offset int, src color.RGBA) {
	if src.A == 0 {
		return
	}
	if src.A == 255 {
		pix[offset] = src.R
		pix[offset+1] = src.G
		pix[offset+2] = src.B
		pix[offset+3] = 255
		return
	}
	// Source-over compositing.
	sa := uint32(src.A)
	dr := uint32(pix[offset])
	dg := uint32(pix[offset+1])
	db := uint32(pix[offset+2])
	da := uint32(pix[offset+3])

	outA := sa + da*(255-sa)/255
	if outA == 0 {
		pix[offset] = 0
		pix[offset+1] = 0
		pix[offset+2] = 0
		pix[offset+3] = 0
		return
	}
	pix[offset] = byte((uint32(src.R)*sa + dr*da*(255-sa)/255) / outA)
	pix[offset+1] = byte((uint32(src.G)*sa + dg*da*(255-sa)/255) / outA)
	pix[offset+2] = byte((uint32(src.B)*sa + db*da*(255-sa)/255) / outA)
	pix[offset+3] = byte(outA)
}

// applyOpacity returns a new color with alpha multiplied by opacity.
func applyOpacity(c frame.Color, opacity float32) frame.Color {
	if opacity >= 1.0 {
		return c
	}
	if opacity <= 0.0 {
		return frame.Transparent
	}
	newA := uint8(math.Round(float64(c.A()) * float64(opacity)))
	return c.WithAlpha(newA)
}

func clampf32(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ---------------------------------------------------------------------------
// errors
// ---------------------------------------------------------------------------

type backendError string

func (e backendError) Error() string { return string(e) }

const errBackendClosed = backendError("raster: backend is closed")

// ---------------------------------------------------------------------------
// rasterText — renders a shaped text run using basicfont
// ---------------------------------------------------------------------------

func (b *CPUBackend) rasterText(rect frame.Rect, textRun frame.TextRun, clip, dirty frame.Rect, opacity float32, ps frame.PixelScale) {
	if textRun.IsEmpty() {
		return
	}

	c := applyOpacity(textRun.Color, opacity)
	if c.IsFullyTransparent() {
		return
	}

	clipRect := clip.Intersection(dirty)
	if clipRect.IsEmpty() {
		return
	}

	tx0 := int(math.Round(float64(rect.X * ps.Scale)))
	ty0 := int(math.Round(float64(rect.Y * ps.Scale)))
	tw := int(math.Round(float64(rect.W * ps.Scale)))
	th := int(math.Round(float64(rect.H * ps.Scale)))
	if tw <= 0 || th <= 0 {
		return
	}

	temp := image.NewRGBA(image.Rect(0, 0, tw, th))
	d := &font.Drawer{
		Dst:  temp,
		Src:  image.NewUniform(c.StdColor()),
		Face: basicfont.Face7x13,
	}

	for _, g := range textRun.Glyphs {
		r := rune(g.ID)
		gx := g.XOffset * ps.Scale
		gy := (textRun.FontSize*0.75 + g.YOffset) * ps.Scale
		d.Dot = fixed.P(int(math.Round(float64(gx))), int(math.Round(float64(gy))))
		d.DrawString(string(r))
	}

	cx0 := int(math.Round(float64(clipRect.X)))
	cy0 := int(math.Round(float64(clipRect.Y)))
	cx1 := int(math.Round(float64(clipRect.X + clipRect.W)))
	cy1 := int(math.Round(float64(clipRect.Y + clipRect.H)))

	for dy := cy0; dy < cy1; dy++ {
		sy := dy - ty0
		if sy < 0 || sy >= th {
			continue
		}
		for dx := cx0; dx < cx1; dx++ {
			sx := dx - tx0
			if sx < 0 || sx >= tw {
				continue
			}
			offsetSrc := (sy*tw + sx) * 4
			sr := temp.Pix[offsetSrc]
			sg := temp.Pix[offsetSrc+1]
			sb := temp.Pix[offsetSrc+2]
			sa := temp.Pix[offsetSrc+3]
			if sa == 0 {
				continue
			}

			offsetDst := (dy*b.fb.width + dx) * 4
			blendPixel(b.fb.img.Pix, offsetDst, color.RGBA{R: sr, G: sg, B: sb, A: sa})
		}
	}
}

// ---------------------------------------------------------------------------
// rasterImage — renders a decoded image or SVG
// ---------------------------------------------------------------------------

func (b *CPUBackend) rasterImage(rect frame.Rect, imgSpec ImageSpec, clip, dirty frame.Rect, opacity float32, ps frame.PixelScale) {
	clipRect := clip.Intersection(dirty)
	if clipRect.IsEmpty() {
		return
	}

	var img image.Image = imgSpec.Img
	if img == nil && len(imgSpec.Data) > 0 {
		if isSVGContent(imgSpec.Data) {
			w := int(math.Round(float64(rect.W * ps.Scale)))
			h := int(math.Round(float64(rect.H * ps.Scale)))
			var err error
			img, err = RasterizeSVG(imgSpec.Data, w, h)
			if err != nil {
				return
			}
		}
	}

	if img == nil {
		return
	}

	tx0 := int(math.Round(float64(rect.X * ps.Scale)))
	ty0 := int(math.Round(float64(rect.Y * ps.Scale)))
	tw := int(math.Round(float64(rect.W * ps.Scale)))
	th := int(math.Round(float64(rect.H * ps.Scale)))
	if tw <= 0 || th <= 0 {
		return
	}

	cx0 := int(math.Round(float64(clipRect.X)))
	cy0 := int(math.Round(float64(clipRect.Y)))
	cx1 := int(math.Round(float64(clipRect.X + clipRect.W)))
	cy1 := int(math.Round(float64(clipRect.Y + clipRect.H)))

	imgBounds := img.Bounds()
	iw := imgBounds.Dx()
	ih := imgBounds.Dy()
	if iw <= 0 || ih <= 0 {
		return
	}

	for dy := cy0; dy < cy1; dy++ {
		sy := dy - ty0
		if sy < 0 || sy >= th {
			continue
		}
		srcY := imgBounds.Min.Y + (sy * ih / th)
		if srcY < imgBounds.Min.Y || srcY >= imgBounds.Max.Y {
			continue
		}

		for dx := cx0; dx < cx1; dx++ {
			sx := dx - tx0
			if sx < 0 || sx >= tw {
				continue
			}
			srcX := imgBounds.Min.X + (sx * iw / tw)
			if srcX < imgBounds.Min.X || srcX >= imgBounds.Max.X {
				continue
			}

			c := img.At(srcX, srcY)
			sr, sg, sb, sa := c.RGBA()
			if sa == 0 {
				continue
			}
			r := uint8((sr * 255) / sa)
			g := uint8((sg * 255) / sa)
			bl := uint8((sb * 255) / sa)
			a := uint8(sa >> 8)

			rgba := applyOpacity(frame.NewColor(r, g, bl, a), opacity).StdColor()

			offsetDst := (dy*b.fb.width + dx) * 4
			blendPixel(b.fb.img.Pix, offsetDst, rgba)
		}
	}
}

// ---------------------------------------------------------------------------
// SVG Rasterization Helpers
// ---------------------------------------------------------------------------

func isSVGContent(data []byte) bool {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err != nil {
			return false
		}
		if se, ok := tok.(xml.StartElement); ok {
			return strings.EqualFold(se.Name.Local, "svg")
		}
	}
}

func RasterizeSVG(data []byte, w, h int) (*image.RGBA, error) {
	if !isSVGContent(data) {
		return nil, xml.UnmarshalError("svg parse: data does not contain an SVG root element")
	}

	icon, err := oksvg.ReadIconStream(bytes.NewReader(data), oksvg.WarnErrorMode)
	if err != nil {
		return nil, err
	}

	intrinsicW := int(icon.ViewBox.W)
	intrinsicH := int(icon.ViewBox.H)
	if intrinsicW <= 0 {
		intrinsicW = 100
	}
	if intrinsicH <= 0 {
		intrinsicH = 100
	}
	if w <= 0 {
		w = intrinsicW
	}
	if h <= 0 {
		h = intrinsicH
	}

	icon.SetTarget(0, 0, float64(w), float64(h))

	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(rgba, rgba.Bounds(), image.White, image.Point{}, draw.Src)

	scanner := rasterx.NewScannerGV(w, h, rgba, rgba.Bounds())
	raster := rasterx.NewDasher(w, h, scanner)
	icon.Draw(raster, 1.0)

	return rgba, nil
}
