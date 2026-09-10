package frame

// Color is a premultiplied RGBA pixel packed as R<<24|G<<16|B<<8|A so that a
// uint32 store is one aligned word and byte order in memory is RGBA on a little
// endian host, matching what a bitmap context expects.

type Color uint32

// RGBA builds a premultiplied color from straight components.
func RGBA(r, g, b, a uint8) Color {
	if a == 255 {
		return Color(uint32(r)<<24 | uint32(g)<<16 | uint32(b)<<8 | 255)
	}
	ra := uint32(r) * uint32(a) / 255
	ga := uint32(g) * uint32(a) / 255
	ba := uint32(b) * uint32(a) / 255
	return Color(ra<<24 | ga<<16 | ba<<8 | uint32(a))
}

// RGB builds an opaque color.
func RGB(r, g, b uint8) Color { return RGBA(r, g, b, 255) }

// A returns the alpha byte, which is also the coverage of a premultiplied color.
func (c Color) A() uint8 { return uint8(c & 0xFF) }

// R returns the premultiplied red byte.
func (c Color) R() uint8 { return uint8(c >> 24) }

// G returns the premultiplied green byte.
func (c Color) G() uint8 { return uint8(c >> 16 & 0xFF) }

// B returns the premultiplied blue byte.
func (c Color) B() uint8 { return uint8(c >> 8 & 0xFF) }

// Opaque returns whether the color covers a pixel completely. The blit paths
// have a whole-line copy fast path gated on this, so it is on the hot path.
func (c Color) Opaque() bool { return c.A() == 255 }

// TransparentBlack is the cleared-buffer value: no coverage, nothing to blend.
const TransparentBlack Color = 0

// BlendOver returns src composited over dst, both premultiplied. The +127 is
// the rounding term for the /255, without which repeated compositing drifts
// dark.
func BlendOver(dst, src Color) Color {
	sa := uint32(src.A())
	if sa == 255 {
		return src
	}
	if sa == 0 {
		return dst
	}
	inv := 255 - sa
	r := (uint32(src.R())*255 + uint32(dst.R())*inv + 127) / 255
	g := (uint32(src.G())*255 + uint32(dst.G())*inv + 127) / 255
	b := (uint32(src.B())*255 + uint32(dst.B())*inv + 127) / 255
	a := (uint32(src.A())*255 + uint32(dst.A())*inv + 127) / 255
	return Color(r<<24 | g<<16 | b<<8 | a)
}

// Bitmap is an owned RGBA buffer with an explicit stride. Stride is a field
// rather than W*4 so a sub-region or a platform-owned buffer can be wrapped
// without copying, and so every access path is exercised by the tests.
type Bitmap struct {
	RGBA   []byte
	W, H   int
	Stride int
}

// NewBitmap allocates a zeroed bitmap of the given size. Pooling is layered on
// top by BitmapPool; this constructor is the non-pooled path used at setup time.
func NewBitmap(w, h int) *Bitmap {
	if w <= 0 || h <= 0 {
		return &Bitmap{}
	}
	stride := w * 4
	return &Bitmap{RGBA: make([]byte, stride*h), W: w, H: h, Stride: stride}
}

// Empty reports whether the bitmap has no pixels.
func (b *Bitmap) Empty() bool { return b == nil || len(b.RGBA) == 0 || b.W <= 0 || b.H <= 0 }

// Bounds returns the bitmap's extent in its own local coordinate space.
func (b *Bitmap) Bounds() Rect { return Rect4(0, 0, int32(b.W), int32(b.H)) }

// Size returns the dimensions as a Size, the key used by BitmapPool.
func (b *Bitmap) Size() Size { return Size{W: int32(b.W), H: int32(b.H)} }

// Bytes returns the backing allocation size, which is what tile budgets track.
func (b *Bitmap) Bytes() int64 { return int64(len(b.RGBA)) }

// Reset zeroes every pixel, including padding bytes, so a buffer can be
// returned to the pool in a defined state.
func (b *Bitmap) Reset() { clear(b.RGBA) }

// Pix returns the 4 bytes at (x, y), which must be in bounds.
func (b *Bitmap) Pix(x, y int) []byte {
	o := y*b.Stride + x*4
	return b.RGBA[o : o+4 : o+4]
}

// At returns the pixel at (x, y), or TransparentBlack when out of bounds. The
// bounds check stays here rather than at call sites because the sampling paths
// legitimately read past an edge.
func (b *Bitmap) At(x, y int) Color {
	if x < 0 || y < 0 || x >= b.W || y >= b.H {
		return TransparentBlack
	}
	p := b.Pix(x, y)
	return Color(uint32(p[0])<<24 | uint32(p[1])<<16 | uint32(p[2])<<8 | uint32(p[3]))
}

// Set writes c at (x, y), ignoring out-of-bounds coordinates.
func (b *Bitmap) Set(x, y int, c Color) {
	if x < 0 || y < 0 || x >= b.W || y >= b.H {
		return
	}
	p := b.Pix(x, y)
	p[0] = byte(c >> 24)
	p[1] = byte(c >> 16)
	p[2] = byte(c >> 8)
	p[3] = byte(c)
}

// FillRect writes c over the clipped region. An opaque color takes a
// memset-style line path; a translucent one blends per pixel. writes, when
// non-nil, counts touched pixels and is how the frame gate proves a warm
// scroll did minimal pixel work.
func (b *Bitmap) FillRect(r Rect, c Color, writes *int64) {
	cl := r.Intersection(b.Bounds())
	if cl.Empty() {
		return
	}
	if writes != nil {
		*writes += int64(cl.W()) * int64(cl.H())
	}
	if c.Opaque() {
		line := []byte{c.R(), c.G(), c.B(), c.A()}
		for y := int(cl.Y0); y < int(cl.Y1); y++ {
			row := b.RGBA[y*b.Stride+int(cl.X0)*4 : y*b.Stride+int(cl.X1)*4]
			for i := range row[:4] {
				row[i] = line[i]
			}
			// Double the written prefix until the row is covered: one memory
			// write pass per row instead of per pixel.
			for n := 4; n < len(row); n *= 2 {
				copy(row[n:], row[:n])
			}
		}
		return
	}
	for y := int(cl.Y0); y < int(cl.Y1); y++ {
		for x := int(cl.X0); x < int(cl.X1); x++ {
			b.Set(x, y, BlendOver(b.At(x, y), c))
		}
	}
}

// BlitOver composites src's srcRect onto b at dstOrigin, clipped to clip and to
// both buffers. Opaque source rows take a whole-row copy; translucent rows
// blend. It is the primitive the composer uses to paste tiles into the backing
// store, and the one that has to be right for damage blitting to be trustworthy.
func (b *Bitmap) BlitOver(src *Bitmap, dstOrigin Point, srcRect, clip Rect, writes *int64) {
	if src == nil || src.Empty() {
		return
	}
	dr := RectAt(dstOrigin, srcRect.Size())
	// Also intersect with the region of dst that src's own pixels can reach, so
	// a srcRect extending past src's bounds clips instead of indexing out of range.
	reachable := RectAt(
		dstOrigin.Translate(-srcRect.X0, -srcRect.Y0),
		Size{W: int32(src.W), H: int32(src.H)},
	)
	dr = dr.Intersection(clip).Intersection(b.Bounds()).Intersection(reachable)
	if dr.Empty() {
		return
	}
	if writes != nil {
		*writes += int64(dr.W()) * int64(dr.H())
	}
	for y := int(dr.Y0); y < int(dr.Y1); y++ {
		sy := int(srcRect.Y0) + (y - int(dstOrigin.Y))
		sx := int(srcRect.X0) + (int(dr.X0) - int(dstOrigin.X))
		drow := b.RGBA[y*b.Stride+int(dr.X0)*4 : y*b.Stride+int(dr.X1)*4]
		srow := src.RGBA[sy*src.Stride+sx*4 : sy*src.Stride+sx*4+len(drow)]
		for x := 0; x < len(drow); x += 4 {
			sa := srow[x+3]
			switch {
			case sa == 0:
				continue
			case sa == 255:
				drow[x], drow[x+1], drow[x+2], drow[x+3] = srow[x], srow[x+1], srow[x+2], srow[x+3]
			default:
				dst := Color(uint32(drow[x])<<24 | uint32(drow[x+1])<<16 | uint32(drow[x+2])<<8 | uint32(drow[x+3]))
				out := BlendOver(dst, Color(uint32(srow[x])<<24|uint32(srow[x+1])<<16|uint32(srow[x+2])<<8|uint32(sa)))
				drow[x] = byte(out >> 24)
				drow[x+1] = byte(out >> 16)
				drow[x+2] = byte(out >> 8)
				drow[x+3] = byte(out)
			}
		}
	}
}
