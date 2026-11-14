package frame

import (
	"image"
	"image/color"
)

type Color uint32

func RGBA(r, g, b, a uint8) Color {
	if a == 255 {
		return Color(uint32(r)<<24 | uint32(g)<<16 | uint32(b)<<8 | 255)
	}
	ra := uint32(r) * uint32(a) / 255
	ga := uint32(g) * uint32(a) / 255
	ba := uint32(b) * uint32(a) / 255
	return Color(ra<<24 | ga<<16 | ba<<8 | uint32(a))
}

func RGB(r, g, b uint8) Color { return RGBA(r, g, b, 255) }

func (c Color) A() uint8 { return uint8(c & 0xFF) }

func (c Color) R() uint8 { return uint8(c >> 24) }

func (c Color) G() uint8 { return uint8(c >> 16 & 0xFF) }

func (c Color) B() uint8 { return uint8(c >> 8 & 0xFF) }

func (c Color) RGBA() color.RGBA { return color.RGBA{R: c.R(), G: c.G(), B: c.B(), A: c.A()} }

func AsColor(c color.RGBA) Color {
	return Color(uint32(c.R)<<24 | uint32(c.G)<<16 | uint32(c.B)<<8 | uint32(c.A))
}

func (c Color) Opaque() bool { return c.A() == 255 }

const TransparentBlack Color = 0

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

func BlendOverCoverage(dst Color, c Color, cov uint8) Color {
	if cov == 255 {
		return c
	}
	if cov == 0 {
		return dst
	}
	ca := uint32(cov)
	inv := 255 - ca
	r := (uint32(c.R())*ca + uint32(dst.R())*inv + 127) / 255
	g := (uint32(c.G())*ca + uint32(dst.G())*inv + 127) / 255
	b := (uint32(c.B())*ca + uint32(dst.B())*inv + 127) / 255
	a := (uint32(c.A())*ca + uint32(dst.A())*inv + 127) / 255
	return Color(r<<24 | g<<16 | b<<8 | a)
}

type Bitmap struct {
	RGBA   []byte
	W, H   int
	Stride int
}

func NewBitmap(w, h int) *Bitmap {
	if w <= 0 || h <= 0 {
		return &Bitmap{}
	}
	stride := w * 4
	return &Bitmap{RGBA: make([]byte, stride*h), W: w, H: h, Stride: stride}
}

func (b *Bitmap) Empty() bool { return b == nil || len(b.RGBA) == 0 || b.W <= 0 || b.H <= 0 }

func (b *Bitmap) Bounds() Rect { return Rect4(0, 0, int32(b.W), int32(b.H)) }

func (b *Bitmap) Size() Size { return Size{W: int32(b.W), H: int32(b.H)} }

func (b *Bitmap) Bytes() int64 { return int64(len(b.RGBA)) }

func (b *Bitmap) AsRGBA() *image.RGBA {
	return &image.RGBA{Pix: b.RGBA, Stride: b.Stride, Rect: image.Rect(0, 0, b.W, b.H)}
}

func (b *Bitmap) Reset() { clear(b.RGBA) }

func (b *Bitmap) Pix(x, y int) []byte {
	o := y*b.Stride + x*4
	return b.RGBA[o : o+4 : o+4]
}

func (b *Bitmap) At(x, y int) Color {
	if x < 0 || y < 0 || x >= b.W || y >= b.H {
		return TransparentBlack
	}
	p := b.Pix(x, y)
	return Color(uint32(p[0])<<24 | uint32(p[1])<<16 | uint32(p[2])<<8 | uint32(p[3]))
}

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
			for n := 4; n < len(row); n *= 2 {
				copy(row[n:], row[:n])
			}
		}
		return
	}
	for y := int(cl.Y0); y < int(cl.Y1); y++ {
		rowBase := y*b.Stride + int(cl.X0)*4
		row := b.RGBA[rowBase : rowBase+int(cl.X1-cl.X0)*4]
		o := 0
		for x := int(cl.X0); x < int(cl.X1); x++ {
			p := row[o : o+4 : o+4]
			dst := Color(uint32(p[0])<<24 | uint32(p[1])<<16 | uint32(p[2])<<8 | uint32(p[3]))
			out := BlendOver(dst, c)
			p[0] = byte(out >> 24)
			p[1] = byte(out >> 16)
			p[2] = byte(out >> 8)
			p[3] = byte(out)
			o += 4
		}
	}
}

func (b *Bitmap) BlitOver(src *Bitmap, dstOrigin Point, srcRect, clip Rect, writes *int64) {
	if src == nil || src.Empty() {
		return
	}
	dr := RectAt(dstOrigin, srcRect.Size())
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
