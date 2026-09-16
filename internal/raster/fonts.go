package raster

import (
	"image"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/vyquocvu/goosie/internal/frame"
)

// faceDPI is the resolution faces are created at. opentype converts a point size
// to pixels as Size * DPI / 72, so holding DPI at 72 makes FaceOptions.Size the
// pixel size outright. That is what lets a GlyphRun carry a device-pixel size
// and the rasterizer to do no unit conversion at all.
const faceDPI = 72

// Glyph is one rasterized glyph in a face at one size.
//
// Bounds is the mask's box relative to the pen position, in device pixels: a
// glyph that hangs below the baseline has a positive Y1, one with a left
// side bearing has a positive X0. Mask is a private copy of the coverage the
// face produced, which is what makes a Glyph safe to hand to any number of
// workers at once.
type Glyph struct {
	Mask    *image.Alpha
	Bounds  frame.Rect
	Advance int32
	Ok      bool
}

// Bytes returns the mask's allocation, which is what the atlas budgets against.
// A glyph with no mask (a space) costs only its own struct.
func (g Glyph) Bytes() int64 {
	if g.Mask == nil {
		return 0
	}
	return int64(len(g.Mask.Pix))
}

// faceKey identifies one cached face.
//
// A device-pixel size is the whole key. The alternative - keying on
// (CSS size, scale) as well - splits the cache for no gain: 16px at DPR 1 and
// 16px at DPR 2 are the same bitmap, and paint.GlyphRun already documents its
// Size as "already scaled by the device ratio" for exactly this reason. Keeping
// the key narrow is what lets a page that changes zoom reuse every glyph it has
// already drawn.
type faceKey struct {
	size int32
}

// Fonts owns the parsed font and the faces derived from it.
//
// Exactly one font exists in M1. Go Regular is embedded, so glyph raster is a
// pure function of (rune, size): the same binary on a fontless Ubuntu CI runner
// and on a Mac produces byte-identical tiles, which is what lets the frame gate
// compare hashes instead of eyeballing screenshots.
//
// An opentype.Face reuses internal scratch buffers between calls and is not safe
// for concurrent use, so faces are guarded here rather than shared. The
// rendering of a glyph happens once and its mask is copied into a private
// image.Alpha, so the lock covers a cache miss and never a warm tile raster -
// which is the difference between parallel rasterization and a mutex in the
// hot path.
type Fonts struct {
	mu      sync.Mutex
	src     *opentype.Font
	entries map[faceKey]*faceEntry
}

type faceEntry struct {
	face    font.Face
	glyphs  map[rune]Glyph
	missing map[rune]struct{}
}

// NewFonts parses the embedded font. It is called once at startup; the parse
// cost is a few hundred microseconds and it is the only place the 900 KB font
// bytes are read.
func NewFonts() (*Fonts, error) {
	f, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return nil, err
	}
	return &Fonts{src: f, entries: map[faceKey]*faceEntry{}}, nil
}

// Face returns the cached face for a device-pixel size.
//
// The returned face is shared and must not be driven concurrently: use Glyph
// unless the caller already holds this Fonts lock. A size at or below zero
// yields nil, which callers treat as "draw nothing" rather than a default size,
// so a layout bug shows up as missing text instead of text at the wrong scale.
func (f *Fonts) Face(size int32) font.Face {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e := f.faceLocked(size); e != nil {
		return e.face
	}
	return nil
}

func (f *Fonts) faceLocked(size int32) *faceEntry {
	if size <= 0 {
		return nil
	}
	k := faceKey{size: size}
	if e, ok := f.entries[k]; ok {
		return e
	}
	face, err := opentype.NewFace(f.src, &opentype.FaceOptions{
		Size: float64(size),
		DPI:  faceDPI,
		// No hinting on purpose. Hinting adjusts stem positions to the pixel
		// grid, and the grid differs between a Retina display and a 1x monitor,
		// so a hinted rendering is not a function of size alone. Determinism
		// across machines is worth more here than a slightly crisper stem.
		Hinting: font.HintingNone,
	})
	if err != nil {
		return nil
	}
	e := &faceEntry{face: face, glyphs: map[rune]Glyph{}, missing: map[rune]struct{}{}}
	f.entries[k] = e
	return e
}

// Glyph rasterizes r at a device-pixel size, returning a mask the caller owns
// (and may keep in a cache). The zero Glyph, with Ok false, means the font has
// no such glyph, which the text path treats as whitespace rather than an error.
func (f *Fonts) Glyph(size int32, r rune) Glyph {
	f.mu.Lock()
	defer f.mu.Unlock()
	e := f.faceLocked(size)
	if e == nil {
		return Glyph{}
	}
	if g, ok := e.glyphs[r]; ok {
		return g
	}
	if _, bad := e.missing[r]; bad {
		return Glyph{}
	}
	g := renderGlyph(e.face, r)
	if !g.Ok {
		e.missing[r] = struct{}{}
		return Glyph{}
	}
	e.glyphs[r] = g
	return g
}

// renderGlyph copies one glyph's coverage out of a face.
//
// The copy is mandatory rather than defensive: a face's own mask buffer is
// documented to change on the next Glyph call, and a tile cache holds masks for
// as long as the tiles do.
func renderGlyph(face font.Face, r rune) Glyph {
	dr, mask, maskp, advance, ok := face.Glyph(fixed.Point26_6{}, r)
	adv := int32((advance + 32) >> 6)
	if !ok {
		return Glyph{Advance: adv}
	}
	w, h := dr.Dx(), dr.Dy()
	g := Glyph{
		Bounds:  frame.Rect4(int32(dr.Min.X), int32(dr.Min.Y), int32(dr.Max.X), int32(dr.Max.Y)),
		Advance: adv,
		Ok:      true,
	}
	if w <= 0 || h <= 0 {
		return g
	}
	m := image.NewAlpha(image.Rect(0, 0, w, h))
	if am, isAlpha := mask.(*image.Alpha); isAlpha {
		// DrawMask's convention, which Glyph's return values are built for: the
		// mask pixel covering destination pixel (x, y) in dr is
		// (x-dr.Min.X+maskp.X, y-dr.Min.Y+maskp.Y). Indexing the copy by its own
		// pixel therefore needs maskp alone, not dr.Min as well - adding both
		// shifts every mask off by its own height and renders blank glyphs.
		mb := am.Bounds()
		for y := 0; y < h; y++ {
			my := y + maskp.Y
			if my < mb.Min.Y || my >= mb.Max.Y {
				continue
			}
			row := m.Pix[y*m.Stride : y*m.Stride+w]
			for x := 0; x < w; x++ {
				mx := x + maskp.X
				if mx < mb.Min.X || mx >= mb.Max.X {
					continue
				}
				row[x] = am.Pix[my*am.Stride+mx]
			}
		}
	} else {
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				_, _, _, a := mask.At(x+maskp.X, y+maskp.Y).RGBA()
				m.Pix[y*m.Stride+x] = byte(a >> 8)
			}
		}
	}
	g.Mask = m
	return g
}
