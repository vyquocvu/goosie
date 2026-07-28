//go:build darwin && cgo

package raster

// #cgo LDFLAGS: -framework CoreGraphics
/*
#include <CoreGraphics/CoreGraphics.h>
#include <stdlib.h>

static inline int ctxIsNull(CGContextRef ctx)       { return ctx == NULL ? 1 : 0; }
static inline void ctxRelease(CGContextRef ctx)      { if (ctx) CGContextRelease(ctx); }

static inline CGContextRef createBitmapContext(void *pixels, int w, int h) {
	CGColorSpaceRef cs = CGColorSpaceCreateDeviceRGB();
	CGContextRef ctx = CGBitmapContextCreate(pixels, w, h, 8, w * 4, cs,
		kCGBitmapByteOrder32Big | kCGImageAlphaPremultipliedLast);
	CGColorSpaceRelease(cs);
	return ctx;
}

static inline void fillRectsBatch(CGContextRef ctx, const float *rects, const uint32_t *colors, int count) {
	for (int i = 0; i < count; i++) {
		const float *r = rects + i * 4;
		uint32_t c = colors[i];
		CGFloat a  = ((c >> 24) & 0xFF) / 255.0;
		CGFloat rr = ((c >> 16) & 0xFF) / 255.0;
		CGFloat gg = ((c >>  8) & 0xFF) / 255.0;
		CGFloat bb = (c        & 0xFF) / 255.0;
		CGContextSetRGBFillColor(ctx, rr, gg, bb, a);
		CGContextFillRect(ctx, CGRectMake(r[0], r[1], r[2], r[3]));
	}
}

static inline void cgSave(CGContextRef ctx)    { CGContextSaveGState(ctx); }
static inline void cgRestore(CGContextRef ctx) { CGContextRestoreGState(ctx); }

static inline void cgClipRect(CGContextRef ctx, float x, float y, float w, float h) {
	CGContextClipToRect(ctx, CGRectMake(x, y, w, h));
}

static inline void cgSetAlpha(CGContextRef ctx, float a) { CGContextSetAlpha(ctx, a); }

static inline void clearRect(CGContextRef ctx, float x, float y, float w, float h) {
	CGContextClearRect(ctx, CGRectMake(x, y, w, h));
}

static inline void cgTranslate(CGContextRef ctx, float tx, float ty) {
	CGContextTranslateCTM(ctx, tx, ty);
}
static inline void cgScale(CGContextRef ctx, float sx, float sy) {
	CGContextScaleCTM(ctx, sx, sy);
}

static inline void drawRGBA(CGContextRef ctx, uint8_t *pixels, int imgW, int imgH,
	float dx, float dy, float dw, float dh) {
	CGColorSpaceRef cs = CGColorSpaceCreateDeviceRGB();
	CGDataProviderRef provider = CGDataProviderCreateWithData(NULL, pixels,
		imgW * imgH * 4, NULL);
	if (!provider) { CGColorSpaceRelease(cs); return; }
	CGImageRef cgImg = CGImageCreate(imgW, imgH, 8, 32, imgW * 4, cs,
		kCGBitmapByteOrder32Big | kCGImageAlphaPremultipliedLast,
		provider, NULL, false, kCGRenderingIntentDefault);
	CGColorSpaceRelease(cs);
	if (cgImg) {
		cgSave(ctx);
		cgTranslate(ctx, dx, dy + dh);
		cgScale(ctx, 1, -1);
		CGContextDrawImage(ctx, CGRectMake(0, 0, dw, dh), cgImg);
		cgRestore(ctx);
	}
	CGDataProviderRelease(provider);
}
*/
import "C"
import (
	"bytes"
	"image"
	"math"
	"unsafe"

	"github.com/vyquocvu/goosie/internal/renderer/frame"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// cgBackend implements Backend using macOS CoreGraphics.
type cgBackend struct {
	ctx    C.CGContextRef
	hasCtx bool
	pix    unsafe.Pointer
	w, h   int
	vp     frame.Viewport

	batchRects  []float32
	batchColors []uint32

	closed bool
}

func newCGBackend(width, height int) (Backend, error) {
	if width <= 0 {
		width = 1
	}
	if height <= 0 {
		height = 1
	}
	return &cgBackend{}, nil
}

func (b *cgBackend) BeginFrame(vp frame.Viewport) error {
	if b.closed {
		return errBackendClosed
	}

	dw, dh := vp.DeviceSize()
	w, h := int(dw), int(dh)
	if w <= 0 {
		w = 1
	}
	if h <= 0 {
		h = 1
	}

	b.releaseCtx()

	b.pix = C.malloc(C.size_t(w * h * 4))
	if b.pix == nil {
		return backendError("cg: failed to allocate pixel buffer")
	}

	b.ctx = C.createBitmapContext(b.pix, C.int(w), C.int(h))
	if C.ctxIsNull(b.ctx) != 0 {
		C.free(b.pix)
		b.pix = nil
		return backendError("cg: failed to create bitmap context")
	}
	b.hasCtx = true

	b.w = w
	b.h = h
	b.vp = vp
	b.batchRects = b.batchRects[:0]
	b.batchColors = b.batchColors[:0]
	return nil
}

func (b *cgBackend) Rasterize(cmds []DisplayCmd, dirty []frame.Rect) (image.Image, error) {
	if b.closed {
		return nil, errBackendClosed
	}

	db := b.dirtyBounds(dirty)

	C.cgSave(b.ctx)

	// CG uses bottom-left origin; Go image uses top-left.
	// Flip the Y axis so all drawing coords match Go's convention.
	C.cgTranslate(b.ctx, 0, C.float(b.h))
	C.cgScale(b.ctx, 1, -1)

	if !db.IsEmpty() {
		C.cgClipRect(b.ctx, C.float(db.X), C.float(db.Y), C.float(db.W), C.float(db.H))
		C.clearRect(b.ctx, C.float(db.X), C.float(db.Y), C.float(db.W), C.float(db.H))
	}

	for _, cmd := range cmds {
		switch cmd.Kind {
		case CmdFill:
			b.batchFill(cmd.Rect, cmd.Color)

		case CmdBorder:
			b.flush()
			b.renderBorder(cmd.Rect, cmd.Border)

		case CmdClipPush:
			b.flush()
			C.cgSave(b.ctx)
			C.cgClipRect(b.ctx, C.float(cmd.Rect.X), C.float(cmd.Rect.Y),
				C.float(cmd.Rect.W), C.float(cmd.Rect.H))

		case CmdClipPop:
			b.flush()
			C.cgRestore(b.ctx)

		case CmdOpacityPush:
			b.flush()
			C.cgSave(b.ctx)
			C.cgSetAlpha(b.ctx, C.float(clampf32(cmd.Opacity, 0, 1)))

		case CmdOpacityPop:
			b.flush()
			C.cgRestore(b.ctx)

		case CmdText:
			b.flush()
			b.renderText(cmd.Rect, cmd.TextRun)

		case CmdImage:
			b.flush()
			b.renderImage(cmd.Rect, cmd.Image)
		}
	}
	b.flush()

	// Restore Y-flip and dirty clip back to pre-Rasterize state.
	C.cgRestore(b.ctx)

	pix := unsafe.Slice((*byte)(b.pix), b.w*b.h*4)
	return &image.RGBA{
		Pix:    pix,
		Stride: b.w * 4,
		Rect:   image.Rect(0, 0, b.w, b.h),
	}, nil
}

func (b *cgBackend) EndFrame() error {
	if b.closed {
		return errBackendClosed
	}
	b.flush()
	return nil
}

func (b *cgBackend) Close() error {
	if b.closed {
		return nil
	}
	b.closed = true
	b.releaseCtx()
	b.batchRects = nil
	b.batchColors = nil
	return nil
}

func (b *cgBackend) batchFill(r frame.Rect, c frame.Color) {
	b.batchRects = append(b.batchRects, r.X, r.Y, r.W, r.H)
	argb := (uint32(c.A()) << 24) | (uint32(c.R()) << 16) | (uint32(c.G()) << 8) | uint32(c.B())
	b.batchColors = append(b.batchColors, argb)
}

func (b *cgBackend) flush() {
	n := len(b.batchRects) / 4
	if n == 0 {
		return
	}
	C.fillRectsBatch(b.ctx,
		(*C.float)(unsafe.Pointer(&b.batchRects[0])),
		(*C.uint32_t)(unsafe.Pointer(&b.batchColors[0])),
		C.int(n))
	b.batchRects = b.batchRects[:0]
	b.batchColors = b.batchColors[:0]
}

func (b *cgBackend) renderBorder(bounds frame.Rect, border BorderSpec) {
	if border.Top.Width > 0 && !border.Top.Color.IsFullyTransparent() {
		b.batchFill(frame.Rect{X: bounds.X, Y: bounds.Y, W: bounds.W, H: border.Top.Width}, border.Top.Color)
	}
	if border.Bottom.Width > 0 && !border.Bottom.Color.IsFullyTransparent() {
		b.batchFill(frame.Rect{X: bounds.X, Y: bounds.Y + bounds.H - border.Bottom.Width, W: bounds.W, H: border.Bottom.Width}, border.Bottom.Color)
	}
	if border.Left.Width > 0 && !border.Left.Color.IsFullyTransparent() {
		b.batchFill(frame.Rect{X: bounds.X, Y: bounds.Y, W: border.Left.Width, H: bounds.H}, border.Left.Color)
	}
	if border.Right.Width > 0 && !border.Right.Color.IsFullyTransparent() {
		b.batchFill(frame.Rect{X: bounds.X + bounds.W - border.Right.Width, Y: bounds.Y, W: border.Right.Width, H: bounds.H}, border.Right.Color)
	}
	b.flush()
}

func (b *cgBackend) renderText(rect frame.Rect, textRun frame.TextRun) {
	if textRun.IsEmpty() {
		return
	}
	tw := int(rect.W)
	th := int(rect.H)
	if tw <= 0 || th <= 0 {
		return
	}

	temp := image.NewRGBA(image.Rect(0, 0, tw, th))

	// Resolve a scalable font face from the shared registry. The
	// CPU and CoreGraphics backends share the registry so the two
	// code paths produce equivalent text output. Falls back to
	// skipping the run when no face is available, rather than
	// drawing placeholder pixels at the wrong size.
	face := resolveTextFaceFromRegistry(textRun)
	if face == nil {
		return
	}
	d := &font.Drawer{
		Dst:  temp,
		Src:  image.NewUniform(textRun.Color.StdColor()),
		Face: face,
	}

	ps := b.vp.PixelScale
	for _, g := range textRun.Glyphs {
		gx := float64(g.XOffset) * float64(ps.Scale)
		gy := float64(textRun.FontSize)*0.75 + float64(g.YOffset)
		d.Dot = fixed.P(int(math.Round(gx)), int(math.Round(gy)))
		d.DrawString(string(rune(g.ID)))
	}

	C.drawRGBA(b.ctx, (*C.uint8_t)(unsafe.Pointer(&temp.Pix[0])),
		C.int(tw), C.int(th),
		C.float(rect.X), C.float(rect.Y), C.float(rect.W), C.float(rect.H))
}

func (b *cgBackend) renderImage(rect frame.Rect, imgSpec ImageSpec) {
	var img image.Image
	if imgSpec.Img != nil {
		img = imgSpec.Img
	} else if len(imgSpec.Data) > 0 {
		rgba, err := decodeImageData(imgSpec.Data, int(rect.W), int(rect.H))
		if err != nil {
			return
		}
		img = rgba
	}
	if img == nil {
		return
	}

	rgba := toRGBA(img)
	if rgba == nil {
		return
	}

	C.drawRGBA(b.ctx,
		(*C.uint8_t)(unsafe.Pointer(&rgba.Pix[0])),
		C.int(rgba.Rect.Dx()), C.int(rgba.Rect.Dy()),
		C.float(rect.X), C.float(rect.Y),
		C.float(rect.W), C.float(rect.H))
}

func (b *cgBackend) releaseCtx() {
	if b.hasCtx {
		C.ctxRelease(b.ctx)
		b.hasCtx = false
	}
	if b.pix != nil {
		C.free(b.pix)
		b.pix = nil
	}
}

func (b *cgBackend) dirtyBounds(dirty []frame.Rect) frame.Rect {
	if len(dirty) == 0 {
		return frame.Rect{X: 0, Y: 0, W: float32(b.w), H: float32(b.h)}
	}
	u := dirty[0]
	for _, d := range dirty[1:] {
		u = u.Union(d)
	}
	return u
}

func decodeImageData(data []byte, w, h int) (*image.RGBA, error) {
	if isSVGContent(data) {
		return RasterizeSVG(data, w, h)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return toRGBA(img), nil
}

func toRGBA(img image.Image) *image.RGBA {
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba
	}
	bounds := img.Bounds()
	rgba := image.NewRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			rgba.Set(x, y, img.At(x, y))
		}
	}
	return rgba
}
