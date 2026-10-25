package raster

import (
	"image"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
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
	Missing bool
}

// Bytes returns the mask's allocation, which is what the atlas budgets against.
// A glyph with no mask (a space) costs only its own struct.
func (g Glyph) Bytes() int64 {
	if g.Mask == nil {
		return 0
	}
	return int64(len(g.Mask.Pix))
}

// faceKey identifies one cached face: a slot at a device-pixel size.
//
// The size is all the key carries beyond the slot. The alternative - keying on
// (CSS size, scale) as well - splits the cache for no gain: 16px at DPR 1 and
// 16px at DPR 2 are the same bitmap, and paint.GlyphRun already documents its
// Size as "already scaled by the device ratio" for exactly this reason. Keeping
// the key narrow is what lets a page that changes zoom reuse every glyph it has
// already drawn.
type faceKey struct {
	slot frame.FontSlot
	size int32
}

// Fonts owns the parsed faces and the per-size faces derived from them.
//
// A slot with no host font is served by the embedded Go face instead of failing,
// so the set of families is a quality difference rather than a correctness one:
// the same binary on a fontless Ubuntu CI runner and on a Mac draws different
// typefaces but never missing text. GOOSIE_SYSTEM_FONTS=off pins every slot to
// the embedded face, which is what a run that compares rendered bytes needs on a
// host that has the fonts.
//
// An opentype.Face reuses internal scratch buffers between calls and is not safe
// for concurrent use, so faces are guarded here rather than shared. The
// rendering of a glyph happens once and its mask is copied into a private
// image.Alpha, so the lock covers a cache miss and never a warm tile raster -
// which is the difference between parallel rasterization and a mutex in the
// hot path.
//
// The lock is a RWMutex so warm lookups run concurrently: a hit in a face's
// glyph or advance memo only takes the read lock, while driving a face (a miss)
// takes the write lock. The per-face memos are only ever touched under this
// lock, so no per-face lock is needed.
type fallbackKey struct {
	idx  int
	size int32
}

type Fonts struct {
	mu      sync.RWMutex
	sources map[frame.FontSlot]*opentype.Font
	entries map[faceKey]*faceEntry
	// fbCache memoizes fallbackLocked's chain per requested (slot, size). It is
	// only read and written under the write lock, and it is dropped whenever a
	// new face entry appears, so a cached chain always equals a fresh one.
	fbCache      map[faceKey][]*faceEntry
	custom       []*opentype.Font
	fallbacks    []*opentype.Font
	fbEntries    map[fallbackKey]*faceEntry
	missingCount atomic.Int64
}

type faceEntry struct {
	face    font.Face
	glyphs  map[rune]Glyph
	missing map[rune]struct{}
	// adv memoizes GlyphAdvanceFixed's 26.6 result per rune, including the
	// fallback-resolved value; advMiss memoizes the "no face has it" answer.
	// Both are populated only by the GlyphAdvanceFixed path from the exact
	// values it used to compute, so a cached hit is bit-identical to a fresh
	// call. The Glyph path keeps its own glyphs/missing memo untouched.
	adv     map[rune]int32
	advMiss map[rune]struct{}
}

// systemFontDir holds the families macOS ships for documents.
const systemFontDir = "/System/Library/Fonts/Supplemental"

// systemFontFiles maps a CSS-resolved slot onto the host file that draws it.
// A missing or unreadable file is normal - it is what a Linux host looks like -
// and leaves the slot on the embedded face.
var systemFontFiles = []struct {
	slot frame.FontSlot
	file string
}{
	// Chromium's default serif on macOS is Times New Roman, and the score says
	// so: Apple's Times from Times.ttc renders the same pages ~5pp worse against
	// Playwright references.
	{frame.FontSlot{Family: frame.FontTimes}, "Times New Roman.ttf"},
	{frame.FontSlot{Family: frame.FontTimes, Bold: true}, "Times New Roman Bold.ttf"},
	{frame.FontSlot{Family: frame.FontTimes, Italic: true}, "Times New Roman Italic.ttf"},
	{frame.FontSlot{Family: frame.FontTimes, Bold: true, Italic: true}, "Times New Roman Bold Italic.ttf"},
	{frame.FontSlot{Family: frame.FontArial}, "Arial.ttf"},
	{frame.FontSlot{Family: frame.FontArial, Bold: true}, "Arial Bold.ttf"},
	{frame.FontSlot{Family: frame.FontArial, Italic: true}, "Arial Italic.ttf"},
	{frame.FontSlot{Family: frame.FontArial, Bold: true, Italic: true}, "Arial Bold Italic.ttf"},
	{frame.FontSlot{Family: frame.FontCourier}, "Courier New.ttf"},
	{frame.FontSlot{Family: frame.FontCourier, Bold: true}, "Courier New Bold.ttf"},
	{frame.FontSlot{Family: frame.FontCourier, Italic: true}, "Courier New Italic.ttf"},
	{frame.FontSlot{Family: frame.FontCourier, Bold: true, Italic: true}, "Courier New Bold Italic.ttf"},
	{frame.FontSlot{Family: frame.FontGeorgia}, "Georgia.ttf"},
	{frame.FontSlot{Family: frame.FontGeorgia, Bold: true}, "Georgia Bold.ttf"},
	{frame.FontSlot{Family: frame.FontGeorgia, Italic: true}, "Georgia Italic.ttf"},
	{frame.FontSlot{Family: frame.FontGeorgia, Bold: true, Italic: true}, "Georgia Bold Italic.ttf"},
	{frame.FontSlot{Family: frame.FontVerdana}, "Verdana.ttf"},
	{frame.FontSlot{Family: frame.FontVerdana, Bold: true}, "Verdana Bold.ttf"},
	{frame.FontSlot{Family: frame.FontVerdana, Italic: true}, "Verdana Italic.ttf"},
	{frame.FontSlot{Family: frame.FontVerdana, Bold: true, Italic: true}, "Verdana Bold Italic.ttf"},
}

// systemFontCollections loads faces that only exist inside a macOS font
// collection (.ttc), which opentype.Parse cannot read as a single font. Each
// entry names a face by its index in the collection. The indices are stable for
// a given macOS build; a host without the file simply leaves the slot on the
// embedded face, exactly as a missing .ttf does.
//
// Helvetica Neue is the family Chromium falls back to for a "Helvetica Neue",
// Helvetica CSS stack on macOS, so goosie has to reach for the same face to line
// its glyph advances up with the reference renders.
var systemFontCollections = []struct {
	slot frame.FontSlot
	path string
	idx  int
}{
	{frame.FontSlot{Family: frame.FontHelvetica}, "/System/Library/Fonts/HelveticaNeue.ttc", 2},
	{frame.FontSlot{Family: frame.FontHelvetica, Bold: true}, "/System/Library/Fonts/HelveticaNeue.ttc", 10},
	{frame.FontSlot{Family: frame.FontHelvetica, Light: true}, "/System/Library/Fonts/HelveticaNeue.ttc", 12},
}

// systemFallbackFiles lists system font files used as wide-coverage fallbacks
// when no document family or standard system font carries a glyph.
// They are appended at the end of the fallback chain so that CSS-specified
// families and standard system slots always take priority.
var systemFallbackFiles = []string{
	// macOS
	"/System/Library/Fonts/Supplemental/Arial Unicode.ttf",
	"/System/Library/Fonts/Apple Symbols.ttf",
	"/Library/Fonts/Arial Unicode.ttf",
	// Linux
	"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	"/usr/share/fonts/dejavu/DejaVuSans.ttf",
	"/usr/share/fonts/truetype/noto/NotoSansSymbols2-Regular.ttf",
	"/usr/share/fonts/truetype/noto/NotoSansSymbols-Regular.ttf",
	"/usr/share/fonts/noto/NotoSansSymbols2-Regular.ttf",
	"/usr/share/fonts/noto/NotoSansSymbols-Regular.ttf",
	"/usr/share/fonts/opentype/noto/NotoSansSymbols2-Regular.otf",
	"/usr/share/fonts/truetype/freefont/FreeSans.ttf",
}

// NewFonts parses the embedded font and every host font it can read. It is
// called once at startup; the parse cost is a few hundred microseconds per face
// and it is the only place the font bytes are read.
func NewFonts() (*Fonts, error) {
	f, err := NewEmbeddedFonts()
	if err != nil {
		return nil, err
	}
	if systemFontsDisabled() {
		return f, nil
	}
	for _, sf := range systemFontFiles {
		data, err := os.ReadFile(systemFontDir + "/" + sf.file)
		if err != nil {
			continue
		}
		parsed, err := opentype.Parse(data)
		if err != nil {
			continue
		}
		f.sources[sf.slot] = parsed
	}
	for _, sc := range systemFontCollections {
		data, err := os.ReadFile(sc.path)
		if err != nil {
			continue
		}
		col, err := opentype.ParseCollection(data)
		if err != nil || sc.idx >= col.NumFonts() {
			continue
		}
		parsed, err := col.Font(sc.idx)
		if err != nil {
			continue
		}
		f.sources[sc.slot] = parsed
	}
	for _, path := range systemFallbackFiles {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		parsed, err := opentype.Parse(data)
		if err != nil {
			continue
		}
		f.fallbacks = append(f.fallbacks, parsed)
	}
	return f, nil
}

// NewEmbeddedFonts returns a Fonts that serves every slot from the embedded Go
// faces. A host with no system fonts is in this state anyway, which is why it is
// a supported configuration rather than a test-only fixture.
func NewEmbeddedFonts() (*Fonts, error) {
	regular, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return nil, err
	}
	bold, err := opentype.Parse(gobold.TTF)
	if err != nil {
		return nil, err
	}
	return &Fonts{
		sources: map[frame.FontSlot]*opentype.Font{
			{}:           regular,
			{Bold: true}: bold,
		},
		entries:   map[faceKey]*faceEntry{},
		fbCache:   map[faceKey][]*faceEntry{},
		fbEntries: map[fallbackKey]*faceEntry{},
	}, nil
}

// AddFallback appends an opentype.Font to the wide-coverage fallback chain.
// It is thread-safe and invalidates any cached fallback chains.
func (f *Fonts) AddFallback(font *opentype.Font) {
	if font == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fallbacks = append(f.fallbacks, font)
	f.fbCache = map[faceKey][]*faceEntry{}
}

// fallbackFaceLocked returns the cached faceEntry for a fallback font at the given size.
// Callers hold f.mu (read or write).
func (f *Fonts) fallbackFaceLocked(idx int, size int32, src *opentype.Font) *faceEntry {
	if size <= 0 || src == nil {
		return nil
	}
	k := fallbackKey{idx: idx, size: size}
	if e, ok := f.fbEntries[k]; ok {
		return e
	}
	face, err := opentype.NewFace(src, &opentype.FaceOptions{
		Size:    float64(size),
		DPI:     faceDPI,
		Hinting: font.HintingNone,
	})
	if err != nil {
		return nil
	}
	e := &faceEntry{
		face:    face,
		glyphs:  map[rune]Glyph{},
		missing: map[rune]struct{}{},
		adv:     map[rune]int32{},
		advMiss: map[rune]struct{}{},
	}
	if f.fbEntries == nil {
		f.fbEntries = map[fallbackKey]*faceEntry{}
	}
	f.fbEntries[k] = e
	return e
}

// Register parses a font file's bytes and returns a 1-based index the style
// resolver threads through FontSlot.CustomIdx. A @font-face rule that names
// this family sets that index on every ComputedStyle that matches, which is
// how a face the engine did not ship with ends up rasterizing glyphs.
func (f *Fonts) Register(data []byte) (uint16, error) {
	parsed, err := opentype.Parse(data)
	if err != nil {
		return 0, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.custom = append(f.custom, parsed)
	return uint16(len(f.custom)), nil
}

// systemFontsDisabled reports whether the environment asks for the embedded face
// in every slot. A gate that compares rendered bytes across machines needs the
// font set, like the rasterizer, to be a function of the binary alone.
func systemFontsDisabled() bool {
	switch strings.ToLower(os.Getenv("GOOSIE_SYSTEM_FONTS")) {
	case "0", "off", "no", "false":
		return true
	}
	return false
}

// Face returns the cached face for a slot at a device-pixel size.
//
// The returned face is shared and must not be driven concurrently: use Glyph
// unless the caller already holds this Fonts lock. A size at or below zero
// yields nil, which callers treat as "draw nothing" rather than a default size,
// so a layout bug shows up as missing text instead of text at the wrong scale.
func (f *Fonts) Face(size int32, slot frame.FontSlot) font.Face {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e := f.faceLocked(size, slot); e != nil {
		return e.face
	}
	return nil
}

func (f *Fonts) faceLocked(size int32, slot frame.FontSlot) *faceEntry {
	if size <= 0 {
		return nil
	}
	slot, src := f.resolveLocked(slot)
	if src == nil {
		return nil
	}
	k := faceKey{slot: slot, size: size}
	if e, ok := f.entries[k]; ok {
		return e
	}
	face, err := opentype.NewFace(src, &opentype.FaceOptions{
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
	e := &faceEntry{face: face, glyphs: map[rune]Glyph{}, missing: map[rune]struct{}{}, adv: map[rune]int32{}, advMiss: map[rune]struct{}{}}
	f.entries[k] = e
	// A new face can join a previously computed fallback chain, so any chain
	// cached before it existed is stale. Face creation happens a handful of
	// times per document (one per slot/size), so dropping the whole memo is
	// cheaper than versioning it.
	f.fbCache = map[faceKey][]*faceEntry{}
	return e
}

// resolveLocked maps a requested slot onto the slot and parsed font that serves
// it, applying the same degradation faceLocked does. It only reads shared maps,
// so it is safe under either the read or the write lock. Callers hold f.mu.
func (f *Fonts) resolveLocked(slot frame.FontSlot) (frame.FontSlot, *opentype.Font) {
	var src *opentype.Font
	if slot.CustomIdx != 0 && int(slot.CustomIdx-1) < len(f.custom) {
		src = f.custom[slot.CustomIdx-1]
	} else {
		src = f.sources[slot]
		// A slot the host cannot serve still has to draw: first the same family
		// without the slant it is missing, then the embedded face at the same
		// weight. The cache is keyed by the face that ends up being used rather than
		// by the request, which is what stops five unavailable families from each
		// holding a private copy of every glyph.
		if src == nil && slot.Italic {
			slot.Italic = false
			src = f.sources[slot]
		}
		// A light weight is served only by a family the host has a light face for;
		// everywhere else it settles back onto that family's regular face rather than
		// dropping the whole family to the embedded face.
		if src == nil && slot.Light {
			slot.Light = false
			src = f.sources[slot]
		}
		if src == nil {
			slot = frame.FontSlot{Bold: slot.Bold}
			src = f.sources[slot]
		}
	}
	return slot, src
}

// lookupLocked returns the cached face entry without creating one, for read-lock
// fast paths. A nil result means the caller must take the write lock and go
// through faceLocked. Callers hold f.mu (read or write).
func (f *Fonts) lookupLocked(size int32, slot frame.FontSlot) *faceEntry {
	if size <= 0 {
		return nil
	}
	slot, src := f.resolveLocked(slot)
	if src == nil {
		return nil
	}
	return f.entries[faceKey{slot: slot, size: size}]
}

// fallbackLocked returns the faces to try for a rune the requested face does not
// carry, in a fixed order: the embedded symbol face first, then every other
// family the host served. The order is sorted rather than map-random because a
// substitute glyph has to be the same glyph on every run of one document.
//
// Callers hold f.mu for writing: resolving the chain can create faces.
// faceLocked degrades a slot it cannot serve, so the same
// face can come back for several slots and is offered once.
//
// The chain for a requested (size, slot) never changes without a new face entry
// appearing - the sources map is fixed after NewFonts - and faceLocked drops the
// memo whenever that happens, so a cached chain is the chain a fresh computation
// would return, including its order. The returned slice is shared and must only
// be iterated, never mutated.
func (f *Fonts) fallbackLocked(size int32, slot frame.FontSlot) []*faceEntry {
	k := faceKey{slot: slot, size: size}
	if fb, ok := f.fbCache[k]; ok {
		return fb
	}
	var out []*faceEntry
	if fb := f.faceLocked(size, frame.FontSlot{Bold: slot.Bold}); fb != nil {
		out = append(out, fb)
	}
	slots := make([]frame.FontSlot, 0, len(f.sources))
	for s := range f.sources {
		if s == slot || s == (frame.FontSlot{Bold: slot.Bold}) {
			continue
		}
		slots = append(slots, s)
	}
	sort.Slice(slots, func(i, j int) bool {
		a, b := slots[i], slots[j]
		if a.Family != b.Family {
			return a.Family < b.Family
		}
		if a.Bold != b.Bold {
			return b.Bold
		}
		if a.Italic != b.Italic {
			return a.Italic
		}
		return a.Light && !b.Light
	})
	for _, s := range slots {
		fb := f.faceLocked(size, s)
		if fb == nil {
			continue
		}
		seen := false
		for _, e := range out {
			if e == fb {
				seen = true
				break
			}
		}
		if !seen {
			out = append(out, fb)
		}
	}
	for i, fbSrc := range f.fallbacks {
		fb := f.fallbackFaceLocked(i, size, fbSrc)
		if fb == nil {
			continue
		}
		seen := false
		for _, e := range out {
			if e == fb {
				seen = true
				break
			}
		}
		if !seen {
			out = append(out, fb)
		}
	}
	f.fbCache[k] = out
	return out
}

// isDefaultIgnorable reports whether r is a default-ignorable code point
// per Unicode (such as variation selectors and zero-width spaces/joiners)
// which should produce no visible glyph and zero advance when unhandled by font tables.
func isDefaultIgnorable(r rune) bool {
	return (r >= 0xFE00 && r <= 0xFE0F) ||
		(r >= 0xE0100 && r <= 0xE01EF) ||
		r == 0x200B || r == 0x200C || r == 0x200D || r == 0xFEFF
}

// tofuAdvanceFixed returns the advance of a missing glyph in 26.6 fixed-point units.
// The advance is stable and non-zero (1 em = size pixels), matching the standard
// square geometry of emoji and .notdef glyphs so layout does not shift when
// a real emoji face is provided.
func tofuAdvanceFixed(size int32) int32 {
	if size <= 0 {
		return 0
	}
	return size << 6
}

// tofuGlyph generates a visible missing glyph (tofu): a bordered rectangle
// with non-zero ink and a stable advance width.
func tofuGlyph(size int32) Glyph {
	if size <= 0 {
		return Glyph{}
	}
	adv := size
	w := (size * 3) / 4
	if w < 2 {
		w = 2
	}
	pad := (adv - w) / 2
	top := -size
	if top >= 0 {
		top = -1
	}
	bottom := size / 8
	if bottom <= 0 {
		bottom = 1
	}
	h := bottom - top
	if h < 2 {
		h = 2
	}
	bounds := frame.Rect4(pad, top, pad+w, bottom)
	m := image.NewAlpha(image.Rect(0, 0, int(w), int(h)))
	stroke := int32(1)
	if size >= 32 {
		stroke = size / 16
	}
	for y := 0; y < int(h); y++ {
		row := m.Pix[y*m.Stride : y*m.Stride+int(w)]
		for x := 0; x < int(w); x++ {
			if x < int(stroke) || x >= int(w)-int(stroke) || y < int(stroke) || y >= int(h)-int(stroke) {
				row[x] = 0xFF
			}
		}
	}
	return Glyph{
		Mask:    m,
		Bounds:  bounds,
		Advance: adv,
		Ok:      true,
		Missing: true,
	}
}

// Glyph rasterizes r in a slot at a device-pixel size, returning a mask the
// caller owns (and may keep in a cache). If no face in the fallback chain has
// such a glyph, it returns a visible tofu glyph (a bordered box) with a stable
// non-zero advance and sets Missing=true, rather than treating missing text as
// whitespace.
func (f *Fonts) Glyph(size int32, r rune, slot frame.FontSlot) Glyph {
	if size <= 0 {
		return Glyph{}
	}
	// Fast path: a cached hit needs no face and no writer.
	f.mu.RLock()
	if e := f.lookupLocked(size, slot); e != nil {
		if g, ok := e.glyphs[r]; ok {
			if g.Missing {
				f.missingCount.Add(1)
			}
			f.mu.RUnlock()
			return g
		}
	}
	f.mu.RUnlock()
	f.mu.Lock()
	defer f.mu.Unlock()
	e := f.faceLocked(size, slot)
	if e == nil {
		return Glyph{}
	}
	if g, ok := e.glyphs[r]; ok {
		if g.Missing {
			f.missingCount.Add(1)
		}
		return g
	}
	g := renderGlyph(e.face, r)
	if !g.Ok {
		// A text family that does not cover a rune must not swallow it: the
		// embedded face carries the arrows, math operators and symbols the
		// document faces do not, and the host's other families cover what it
		// does not. Layout measures through this same function, so the
		// substitute glyph and the advance it was broken with agree.
		for _, fb := range f.fallbackLocked(size, slot) {
			if g = renderGlyph(fb.face, r); g.Ok {
				break
			}
		}
	}
	if !g.Ok {
		if isDefaultIgnorable(r) {
			ignorable := Glyph{Ok: true}
			e.glyphs[r] = ignorable
			return ignorable
		}
		e.missing[r] = struct{}{}
		f.missingCount.Add(1)
		tofu := tofuGlyph(size)
		e.glyphs[r] = tofu
		return tofu
	}
	e.glyphs[r] = g
	return g
}

// GlyphAdvance reports the width r occupies in a slot at a device-pixel size,
// in device pixels. It satisfies layout.Metrics, which is why this method exists
// as a one-liner beside Glyph: layout measures words with the same numbers the
// rasterizer draws them with, without layout importing raster.
func (f *Fonts) GlyphAdvance(size int32, r rune, slot frame.FontSlot) int32 {
	return f.Glyph(size, r, slot).Advance
}

// GlyphAdvanceFixed reports r's advance in 26.6 fixed-point units (1/64 px) as
// an int32, the raw number the face carries before any rounding. It exists for
// the layout/paint measurement contract: a browser sums fractional advances
// across a run and rounds only the pen position at each glyph, so callers that
// place or measure more than one glyph must accumulate these, not the rounded
// GlyphAdvance values.
func (f *Fonts) GlyphAdvanceFixed(size int32, r rune, slot frame.FontSlot) int32 {
	if size <= 0 {
		return 0
	}
	// Fast path: a memoized advance is the value a fresh call would
	// compute, with no face involved, so readers share the lock.
	f.mu.RLock()
	if e := f.lookupLocked(size, slot); e != nil {
		if adv, ok := e.adv[r]; ok {
			if _, bad := e.advMiss[r]; bad {
				f.missingCount.Add(1)
			}
			f.mu.RUnlock()
			return adv
		}
	}
	f.mu.RUnlock()
	f.mu.Lock()
	defer f.mu.Unlock()
	e := f.faceLocked(size, slot)
	if e == nil {
		return 0
	}
	if adv, ok := e.adv[r]; ok {
		if _, bad := e.advMiss[r]; bad {
			f.missingCount.Add(1)
		}
		return adv
	}
	adv, ok := e.face.GlyphAdvance(r)
	if !ok {
		// The same substitution Glyph makes for a rune the document face does
		// not cover, so the measured width and the drawn glyph agree.
		for _, fb := range f.fallbackLocked(size, slot) {
			if adv, ok = fb.face.GlyphAdvance(r); ok {
				break
			}
		}
		if !ok {
			if isDefaultIgnorable(r) {
				e.adv[r] = 0
				return 0
			}
			f.missingCount.Add(1)
			fixedAdv := tofuAdvanceFixed(size)
			e.advMiss[r] = struct{}{}
			e.adv[r] = fixedAdv
			return fixedAdv
		}
	}
	v := int32(adv)
	e.adv[r] = v
	return v
}

// MissingCount returns the total number of missing glyph lookups that fell back to tofu.
func (f *Fonts) MissingCount() int64 {
	return f.missingCount.Load()
}

// ResetMissingCount resets the missing glyph counter to zero.
func (f *Fonts) ResetMissingCount() {
	f.missingCount.Store(0)
}

// HasGlyph reports whether face contains a rendered glyph for rune r.
func HasGlyph(face font.Face, r rune) bool {
	if face == nil {
		return false
	}
	_, _, _, _, ok := face.Glyph(fixed.Point26_6{}, r)
	return ok
}

// FaceForRune returns the font face that covers r for the given size and slot,
// following the fallback chain. If no face in the chain covers r, it returns
// (nil, false), indicating that the rune is missing across all faces.
func (f *Fonts) FaceForRune(size int32, r rune, slot frame.FontSlot) (font.Face, bool) {
	if size <= 0 {
		return nil, false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	e := f.faceLocked(size, slot)
	if e == nil {
		return nil, false
	}
	if HasGlyph(e.face, r) {
		return e.face, true
	}
	for _, fb := range f.fallbackLocked(size, slot) {
		if HasGlyph(fb.face, r) {
			return fb.face, true
		}
	}
	return nil, false
}

// LineMetrics reports a face's ascent, descent and normal line height in device
// pixels. Each value is rounded on its own, the way a browser rounds them: the
// content area is the rounded ascent plus the rounded descent, while a normal
// line box is the font's own leading-inclusive height, which is a pixel taller
// for Times than the content area and the same height as it for Courier.
//
// A slot the host cannot serve answers 0,0,0 and the caller falls back to its
// own estimate, which is what keeps a display-less host laying out at all.
func (f *Fonts) LineMetrics(size int32, slot frame.FontSlot) (int32, int32, int32) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e := f.faceLocked(size, slot)
	if e == nil {
		return 0, 0, 0
	}
	m := e.face.Metrics()
	round := func(v fixed.Int26_6) int32 { return int32((int64(v) + 32) >> 6) }
	return round(m.Ascent), round(m.Descent), round(m.Height)
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
