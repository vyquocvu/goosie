// Package renderer provides text measurement and shaping abstraction for the Goosie engine.
//
// M4.3: Add text measurement and shaping abstraction
//
// The TextShaper provides a backend-neutral interface for measuring and shaping
// text. It caches shaped text runs by text, font, size, direction, and relevant
// features to avoid redundant computation.
//
// Design:
//   - FontKey identifies a unique font configuration (size, weight, style, direction)
//   - ShapedText contains glyphs with positions and metrics
//   - Cache keyed by (text, FontKey) to avoid re-shaping identical runs
//   - Basic Latin support first; advanced shaping via go-text/typesetting is optional
//   - Whitespace-aware text wrapping for line layout
//
// This is additive infrastructure. The existing FontMetrics continues to work.
// The TextShaper provides the foundation for M4.4 (incremental layout).

package renderer

import (
	"strings"
	"sync"
)

// Direction represents text direction.
type Direction uint8

const (
	// DirectionLTR is left-to-right text.
	DirectionLTR Direction = iota
	// DirectionRTL is right-to-left text.
	DirectionRTL
)

// FontKey uniquely identifies a font configuration.
type FontKey struct {
	Size      float32
	Bold      bool
	Italic    bool
	Direction Direction
	Family    string // Font family (empty = default)
}

// Valid reports whether this font key has a positive size.
func (k FontKey) Valid() bool {
	return k.Size > 0
}

// CacheKey returns a string key for caching shaped text.
func (k FontKey) CacheKey() string {
	var sb strings.Builder
	sb.WriteString(k.Family)
	sb.WriteByte('|')
	sb.WriteByte(byte(k.Size))
	if k.Bold {
		sb.WriteByte('B')
	}
	if k.Italic {
		sb.WriteByte('I')
	}
	sb.WriteByte(byte(k.Direction))
	return sb.String()
}

// Glyph represents a single shaped glyph.
type Glyph struct {
	ID     uint32  // Glyph ID (Unicode codepoint for basic Latin)
	X      float32 // X position relative to text origin
	Y      float32 // Y position relative to baseline
	Width  float32 // Glyph advance width
	Height float32 // Glyph height
}

// ShapedText represents the result of shaping a text run.
type ShapedText struct {
	Text    string
	Font    FontKey
	Glyphs  []Glyph
	Width   float32
	Height  float32
	Ascent  float32
	Descent float32
}

// TextShaper shapes and measures text with caching.
type TextShaper struct {
	mu    sync.RWMutex
	cache map[string]*ShapedText // Cache key -> shaped text
}

// NewTextShaper creates a new text shaper.
func NewTextShaper() *TextShaper {
	return &TextShaper{
		cache: make(map[string]*ShapedText),
	}
}

// Shape shapes the given text with the given font configuration.
// Returns a cached result if available.
func (s *TextShaper) Shape(text string, font FontKey) *ShapedText {
	if text == "" {
		return &ShapedText{
			Text:   text,
			Font:   font,
			Glyphs: nil,
			Width:  0,
			Height: font.Size * 1.2,
		}
	}

	// Check cache
	cacheKey := text + "|" + font.CacheKey()
	s.mu.RLock()
	cached, ok := s.cache[cacheKey]
	s.mu.RUnlock()
	if ok {
		return cached
	}

	// Shape the text
	result := s.shapeText(text, font)

	// Cache the result
	s.mu.Lock()
	s.cache[cacheKey] = result
	s.mu.Unlock()

	return result
}

// shapeText performs the actual text shaping.
// For basic Latin, we use simple character-by-character positioning.
func (s *TextShaper) shapeText(text string, font FontKey) *ShapedText {
	runes := []rune(text)
	glyphs := make([]Glyph, len(runes))

	// Base character width (average for proportional fonts)
	baseCharWidth := font.Size * 0.5

	// Adjust for bold
	if font.Bold {
		baseCharWidth *= 1.1
	}

	x := float32(0)
	for i, r := range runes {
		glyphs[i] = Glyph{
			ID:     uint32(r),
			X:      x,
			Y:      0,
			Width:  baseCharWidth,
			Height: font.Size,
		}
		x += baseCharWidth
	}

	height := font.Size * 1.2
	ascent := font.Size * 0.75
	descent := font.Size * 0.25

	return &ShapedText{
		Text:    text,
		Font:    font,
		Glyphs:  glyphs,
		Width:   x,
		Height:  height,
		Ascent:  ascent,
		Descent: descent,
	}
}

// MeasureWrapped measures text with wrapping at the given width.
// Returns a list of line widths.
func (s *TextShaper) MeasureWrapped(text string, font FontKey, maxWidth float32) []float32 {
	if text == "" {
		return nil
	}

	// Split by explicit newlines first
	paragraphs := strings.Split(text, "\n")
	var lines []float32

	for _, para := range paragraphs {
		if para == "" {
			lines = append(lines, 0)
			continue
		}

		// Wrap paragraph at word boundaries
		paraLines := s.wrapParagraph(para, font, maxWidth)
		lines = append(lines, paraLines...)
	}

	return lines
}

// wrapParagraph wraps a single paragraph at word boundaries.
func (s *TextShaper) wrapParagraph(text string, font FontKey, maxWidth float32) []float32 {
	words := strings.Fields(text)
	if len(words) == 0 {
		return []float32{0}
	}

	var lines []float32
	currentWidth := float32(0)
	spaceWidth := font.Size * 0.25 // Approximate space width

	for i, word := range words {
		wordWidth := s.Shape(word, font).Width

		// First word on line
		if currentWidth == 0 {
			lines = append(lines, wordWidth)
			currentWidth = wordWidth
			continue
		}

		// Try to fit word on current line
		newWidth := currentWidth + spaceWidth + wordWidth
		if newWidth <= maxWidth {
			lines[len(lines)-1] = newWidth
			currentWidth = newWidth
		} else {
			// Start new line
			lines = append(lines, wordWidth)
			currentWidth = wordWidth
		}

		_ = i // Avoid unused variable warning
	}

	return lines
}

// MeasureWrappedNoWrap measures text without wrapping (single line).
func (s *TextShaper) MeasureWrappedNoWrap(text string, font FontKey, maxWidth float32) []float32 {
	if text == "" {
		return nil
	}

	width := s.Shape(text, font).Width
	return []float32{width}
}

// ClearCache clears the shaped text cache.
func (s *TextShaper) ClearCache() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache = make(map[string]*ShapedText)
}

// CacheSize returns the number of cached shaped texts.
func (s *TextShaper) CacheSize() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.cache)
}
