// Package raster provides the FontRegistry for resolving scalable font
// faces from font-family / weight / style descriptors.
//
// Prior to this registry, every glyph was rasterized via the
// 7x13 bitmap face in basicfont, which produced text that did not
// scale with font-size, did not honour font-family, and did not
// distinguish bold from regular. The registry loads the bundled
// Go fonts (golang.org/x/image/font/gofont) via the opentype
// parser so that:
//   - text scales with font-size (the headline visual regression
//     seen against Chromium / Playwright);
//   - font-family: sans-serif | serif | monospace routes to a
//     distinct face;
//   - font-weight: bold selects a bold face for headings, <b>,
//     <strong>, etc.
//
// The registry is intentionally simple: an in-memory LRU-ish cache
// keyed on (family, bold, italic, size). The first call for a given
// key parses the relevant TTF; subsequent calls reuse the
// parsed *opentype.Font and only build a new *font.Face when the
// size changes.
//
// On all platforms Go's bundled fonts are used by default. On
// macOS and Linux the registry also probes a small list of common
// system fonts and prefers them when available, falling back to
// the Go fonts if no system font is found. This lets the rendered
// output match the local Chromium look more closely without
// requiring bundled font files.
package raster

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	goitalic "golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/gofont/gomediumitalic"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/gomonobolditalic"
	gomonoitalic "golang.org/x/image/font/gofont/gomonoitalic"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

// FontFamily enumerates the supported CSS font-family buckets.
// We map user-facing family strings (sans-serif, serif, monospace,
// Arial, Helvetica, ...) onto one of these three so the registry
// only needs to keep three fonts in memory.
const (
	FamilySansSerif = "sans-serif"
	FamilySerif     = "serif"
	FamilyMonospace = "monospace"
)

// FontDescriptor is the public identifier for a requested face.
// It mirrors the CSS axes the renderer actually consumes today:
// family, weight, and style. Future fields (variant, stretch)
// can be added without breaking the cache key.
type FontDescriptor struct {
	Family string
	Bold   bool
	Italic bool
}

// FontMetrics holds the design-space metrics for a font. The
// renderer uses these for line-height / ascent / descent
// calculations so layout matches what is rasterized.
type FontMetrics struct {
	Ascent  float32
	Descent float32
	LineGap float32
}

// fontCacheKey is the lookup key for the face cache. Size is part
// of the key because each (font, size) pair produces a distinct
// *font.Face with different glyph scaling.
type fontCacheKey struct {
	family string
	bold   bool
	italic bool
	size   int // size in 1/64-point fixed units to keep the map key small
}

// cachedFace holds the parsed opentype.Font plus the lazily built
// *font.Face for the most recent size. We retain the parsed font
// so we can build additional sizes without re-reading the TTF.
type cachedFace struct {
	font    *opentype.Font
	lastKey fontCacheKey
	last    font.Face
}

// FontRegistry resolves font descriptors to scalable font.Face
// instances. It is safe for concurrent use.
type FontRegistry struct {
	mu    sync.RWMutex
	cache map[fontCacheKey]*cachedFace
	// systemPath is the OS-specific directory containing common
	// .ttf / .otf / .ttc files. Populated lazily on first probe.
	systemPathOnce sync.Once
	systemPath     string
	systemHas      map[string]string // family -> path
}

// NewFontRegistry returns a registry with the bundled Go fonts
// pre-loaded.
func NewFontRegistry() *FontRegistry {
	return &FontRegistry{
		cache:     make(map[fontCacheKey]*cachedFace),
		systemHas: make(map[string]string),
	}
}

// sharedFontRegistry is a package-level registry that the
// platform-specific raster backends (CPU + CoreGraphics) consult
// when no per-backend registry has been installed. Using a single
// shared registry avoids parsing the same TTF once per backend
// and keeps the CPU and CoreGraphics text output identical at
// the pixel level. The shared registry is created lazily on
// first access.
var (
	sharedFontRegistry     *FontRegistry
	sharedFontRegistryOnce sync.Once
)

// SharedFontRegistry returns the package-level font registry
// used by raster backends that do not install their own.
func SharedFontRegistry() *FontRegistry {
	sharedFontRegistryOnce.Do(func() {
		sharedFontRegistry = NewFontRegistry()
	})
	return sharedFontRegistry
}

// SetSharedFontRegistry replaces the package-level registry. Use
// this from tests that need to inject a custom registry.
func SetSharedFontRegistry(r *FontRegistry) {
	sharedFontRegistry = r
}

// Get returns a font.Face for the given descriptor and size. The
// returned face scales with `size` (units: CSS pixels). If the
// descriptor's family is unknown it is normalised to sans-serif.
func (r *FontRegistry) Get(d FontDescriptor, size float32) (font.Face, bool) {
	if size <= 0 {
		size = 14
	}
	family := NormaliseFamily(d.Family)

	// Probe system fonts once on first use so we honour local
	// installed fonts when available. Failures are non-fatal; the
	// bundled Go fonts always remain as a fallback.
	r.systemPathOnce.Do(func() {
		r.probeSystemFonts()
	})

	key := fontCacheKey{
		family: family,
		bold:   d.Bold,
		italic: d.Italic,
		size:   int(size * 64),
	}

	// Single critical section: avoid nested Lock / RLock on the
	// same mutex which would deadlock. The lookup and any cache
	// insertion happen under the write lock; concurrent callers
	// race for the lock but the work inside is fast.
	r.mu.Lock()
	defer r.mu.Unlock()

	if c, ok := r.cache[key]; ok {
		// Same size as last build? Reuse. Otherwise rebuild.
		if c.lastKey == key && c.last != nil {
			return c.last, true
		}
		// Same font, different size: rebuild face.
		if c.font != nil {
			face, err := opentype.NewFace(c.font, &opentype.FaceOptions{
				Size:    float64(size),
				DPI:     72,
				Hinting: font.HintingFull,
			})
			if err == nil {
				c.lastKey = key
				c.last = face
				return face, true
			}
		}
	}

	// Cache miss: load the right TTF, parse, build the face, store.
	ttf, _ := r.lookupTTFLocked(family, d.Bold, d.Italic)
	if ttf == nil {
		return nil, false
	}
	parsed, err := opentype.Parse(ttf)
	if err != nil {
		// System fonts occasionally ship as collections even when
		// the .ttf extension suggests a single font. Try the
		// collection parser and use the first font in that case.
		if col, colErr := sfnt.ParseCollection(ttf); colErr == nil && col.NumFonts() > 0 {
			if f, fontErr := col.Font(0); fontErr == nil {
				parsed = f
			} else {
				return nil, false
			}
		} else {
			return nil, false
		}
	}
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{
		Size:    float64(size),
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, false
	}
	if face == nil {
		return nil, false
	}
	r.cache[key] = &cachedFace{
		font:    parsed,
		lastKey: key,
		last:    face,
	}
	return face, true
}

// DesignMetrics returns the design-space ascent/descent/line-gap
// for the given family at the given size. We return metrics from
// the parsed opentype.Font so the layout engine can use the same
// numbers the raster backend uses. Falls back to a sane default
// when the font cannot be loaded.
func (r *FontRegistry) DesignMetrics(d FontDescriptor, size float32) FontMetrics {
	if size <= 0 {
		size = 14
	}
	family := NormaliseFamily(d.Family)

	r.systemPathOnce.Do(func() {
		r.probeSystemFonts()
	})

	r.mu.RLock()
	ttf, _ := r.lookupTTFLocked(family, d.Bold, d.Italic)
	r.mu.RUnlock()
	if ttf == nil {
		return defaultMetrics(size)
	}
	parsed, err := opentype.Parse(ttf)
	if err != nil {
		return defaultMetrics(size)
	}
	// Use the parsed metrics to compute ascent / descent for the
	// requested pixel size. The opentype.Face exposes a Scale
	// factor that maps font units to pixels.
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{
		Size:    float64(size),
		DPI:     72,
		Hinting: font.HintingNone,
	})
	if err != nil {
		return defaultMetrics(size)
	}
	m := face.Metrics()
	// The returned metrics are already in fixed-point pixels at
	// the requested size. Convert from 26.6 fixed to float32.
	const fixedOne = 64.0
	return FontMetrics{
		Ascent:  float32(m.Ascent) / fixedOne,
		Descent: float32(m.Descent) / fixedOne,
		LineGap: float32(m.Height-m.Ascent-m.Descent) / fixedOne,
	}
}

// lookupTTFLocked returns the raw TTF bytes for the requested family /
// weight / style. It tries the local system font map first, then
// falls back to the bundled Go fonts. The name is the human-
// readable font name (for future metrics reporting).
//
// Caller must hold r.mu (any mode). The function performs file I/O
// while holding the lock; this is intentional because it is called
// only on the cold cache-miss path, and serialising file I/O keeps
// the implementation simple.
func (r *FontRegistry) lookupTTFLocked(family string, bold, italic bool) ([]byte, string) {
	if path, ok := r.systemHas[family]; ok {
		if data, err := os.ReadFile(path); err == nil {
			return data, family
		}
	}

	// Bundled Go fonts: pick regular / bold / italic variants per
	// family. monospace uses the gomono set; sans-serif uses
	// goregular / gobold / goitalic; serif uses go medium italics
	// as a stand-in since no real serif is bundled.
	switch family {
	case FamilyMonospace:
		switch {
		case bold && italic:
			return gomonobolditalic.TTF, "gomonobolditalic"
		case bold:
			return gomonobold.TTF, "gomonobold"
		case italic:
			return gomonoitalic.TTF, "gomonoitalic"
		default:
			return gomono.TTF, "gomono"
		}
	case FamilySerif:
		// No real serif in gofont; gomedium is the closest
		// non-monospaced proportional face available, so we use
		// it for serif until a true serif is added.
		switch {
		case bold && italic:
			return gomediumitalic.TTF, "gomediumitalic"
		case bold:
			return gobold.TTF, "gobold"
		case italic:
			return goitalic.TTF, "goitalic"
		default:
			return gomedium.TTF, "gomedium"
		}
	default: // sans-serif
		switch {
		case bold && italic:
			return gobolditalic.TTF, "gobolditalic"
		case bold:
			return gobold.TTF, "gobold"
		case italic:
			return goitalic.TTF, "goitalic"
		default:
			return goregular.TTF, "goregular"
		}
	}
}

// probeSystemFonts walks the platform-specific system font
// directories and records the first matching file for each known
// family. Failures and missing directories are silent: the
// bundled Go fonts remain as a fallback.
//
// We only consider single-font files (.ttf, .otf). TrueType
// Collections (.ttc, .dfont) bundle multiple fonts and require a
// different parser; we skip them to keep the cold-path I/O
// straightforward. The bundled Go fonts always remain as a
// fallback if no single-font system font is found.
func (r *FontRegistry) probeSystemFonts() {
	dirs := systemFontDirs()
	aliases := systemFontAliases()

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			ext := filepath.Ext(name)
			// Only accept single-font files. Collections (.ttc,
			// .dfont) are skipped — handling them requires a
			// font-collection parser and additional selection logic
			// which is out of scope for the visual-regression fix.
			if ext != ".ttf" && ext != ".otf" {
				continue
			}
			stem := name[:len(name)-len(ext)]
			for family, aliasList := range aliases {
				if _, have := r.systemHas[family]; have {
					continue
				}
				for _, alias := range aliasList {
					if stem == alias {
						r.systemHas[family] = filepath.Join(dir, name)
						break
					}
				}
			}
		}
	}
}

// systemFontDirs returns the directories to scan for installed
// fonts on the current platform.
func systemFontDirs() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/System/Library/Fonts",
			"/System/Library/Fonts/Supplemental",
			"/Library/Fonts",
		}
	case "linux":
		return []string{
			"/usr/share/fonts",
			"/usr/local/share/fonts",
		}
	case "windows":
		return []string{`C:\Windows\Fonts`}
	default:
		return nil
	}
}

// systemFontAliases lists the file stems (without extension) that
// map to each Family*. Only files explicitly named here are
// considered, so a future font drop on the system cannot surprise
// the renderer with a different look.
func systemFontAliases() map[string][]string {
	return map[string][]string{
		// sans-serif: prefer Helvetica on macOS, DejaVu/Liberation
		// on Linux.
		FamilySansSerif: {
			"HelveticaNeue", "Helvetica", "SFNSDisplay",
			"Arial",
			"DejaVuSans", "LiberationSans",
		},
		// serif: prefer Times on macOS, DejaVu/Liberation Serif
		// on Linux.
		FamilySerif: {
			"TimesNewRoman", "Times", "Charter",
			"DejaVuSerif", "LiberationSerif",
		},
		// monospace: prefer SF Mono on macOS, DejaVu/Liberation
		// Mono on Linux.
		FamilyMonospace: {
			"SFNSMono", "Menlo", "CourierNew", "Courier",
			"DejaVuSansMono", "LiberationMono",
		},
	}
}

// NormaliseFamily maps a CSS font-family value (which may include
// unquoted generics, quoted specific names, or comma-separated
// fallbacks) to one of the Family* constants. The first matching
// token wins; anything unrecognised collapses to sans-serif.
//
// Comparison is case-insensitive and ignores whitespace inside
// tokens so that authors can write `Times New Roman`, `TimesNewRoman`,
// or `TIMESNEWROMAN` interchangeably. The fallback chain matches
// the CSS family-with-fallback rule: the first recognised token
// wins, generics act as last-resort buckets.
func NormaliseFamily(family string) string {
	if family == "" {
		return FamilySansSerif
	}
	for _, part := range splitFontFamily(family) {
		key := normaliseToken(part)
		switch key {
		case "sansserif":
			return FamilySansSerif
		case "serif":
			return FamilySerif
		case "monospace":
			return FamilyMonospace
		case "arial", "helvetica", "helveticaeneue", "inter", "roboto",
			"dejavusans", "liberationsans":
			return FamilySansSerif
		case "times", "timesnewroman", "timesnewromans", "charter", "georgia",
			"dejavuserif", "liberationserif":
			return FamilySerif
		case "menlo", "courier", "couriernew", "consolas",
			"dejavusansmono", "liberationmono", "sfnsmono":
			return FamilyMonospace
		}
	}
	return FamilySansSerif
}

// normaliseToken lower-cases the token and removes all whitespace
// so that "Times New Roman" and "TimesNewRoman" compare equal.
func normaliseToken(s string) string {
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			continue
		}
		if ch >= 'A' && ch <= 'Z' {
			ch += 'a' - 'A'
		}
		b = append(b, ch)
	}
	return string(b)
}

// splitFontFamily splits a CSS font-family list on commas and
// trims quotes / whitespace from each candidate. The first
// candidate is the most-preferred.
func splitFontFamily(family string) []string {
	out := []string{}
	cur := ""
	inQuote := byte(0)
	for i := 0; i < len(family); i++ {
		ch := family[i]
		switch {
		case inQuote != 0:
			if ch == inQuote {
				inQuote = 0
			} else {
				cur += string(ch)
			}
		case ch == '"' || ch == '\'':
			inQuote = ch
		case ch == ',':
			if cur = trim(cur); cur != "" {
				out = append(out, cur)
			}
			cur = ""
		default:
			cur += string(ch)
		}
	}
	if cur = trim(cur); cur != "" {
		out = append(out, cur)
	}
	return out
}

func trim(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

// defaultMetrics returns sane ascent/descent/line-gap values when
// the requested font cannot be loaded. The values approximate
// Helvetica/DejaVu Sans so layout is stable across font failures.
func defaultMetrics(size float32) FontMetrics {
	return FontMetrics{
		Ascent:  size * 0.75,
		Descent: size * 0.25,
		LineGap: size * 0.10,
	}
}