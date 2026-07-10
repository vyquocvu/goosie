package renderer

import (
	"testing"
)

// --- TextShaper tests ---

func TestNewTextShaper(t *testing.T) {
	shaper := NewTextShaper()
	if shaper == nil {
		t.Fatal("NewTextShaper returned nil")
	}
}

func TestShapeTextBasicLatin(t *testing.T) {
	shaper := NewTextShaper()

	result := shaper.Shape("Hello", FontKey{Size: 16})
	if result == nil {
		t.Fatal("Shape returned nil")
	}
	if len(result.Glyphs) == 0 {
		t.Error("expected glyphs, got none")
	}
	if result.Width <= 0 {
		t.Errorf("expected positive width, got %f", result.Width)
	}
}

func TestShapeTextEmpty(t *testing.T) {
	shaper := NewTextShaper()

	result := shaper.Shape("", FontKey{Size: 16})
	if result == nil {
		t.Fatal("Shape returned nil for empty text")
	}
	if len(result.Glyphs) != 0 {
		t.Errorf("expected 0 glyphs for empty text, got %d", len(result.Glyphs))
	}
	if result.Width != 0 {
		t.Errorf("expected 0 width for empty text, got %f", result.Width)
	}
}

func TestShapeTextCaching(t *testing.T) {
	shaper := NewTextShaper()
	key := FontKey{Size: 16}

	// Shape same text twice
	result1 := shaper.Shape("Hello", key)
	result2 := shaper.Shape("Hello", key)

	// Should be the same cached result
	if result1 != result2 {
		t.Error("expected cached result to be returned")
	}
}

func TestShapeTextDifferentSizes(t *testing.T) {
	shaper := NewTextShaper()

	result16 := shaper.Shape("Hello", FontKey{Size: 16})
	result32 := shaper.Shape("Hello", FontKey{Size: 32})

	// Different sizes should produce different results
	if result16 == result32 {
		t.Error("different font sizes should produce different results")
	}
	if result32.Width <= result16.Width {
		t.Error("larger font size should produce wider text")
	}
}

func TestShapeTextDifferentFonts(t *testing.T) {
	shaper := NewTextShaper()

	resultNormal := shaper.Shape("Hello", FontKey{Size: 16, Bold: false, Italic: false})
	resultBold := shaper.Shape("Hello", FontKey{Size: 16, Bold: true, Italic: false})

	// Different styles should produce different results
	if resultNormal == resultBold {
		t.Error("different font styles should produce different results")
	}
}

func TestShapeTextDirection(t *testing.T) {
	shaper := NewTextShaper()

	resultLTR := shaper.Shape("Hello", FontKey{Size: 16, Direction: DirectionLTR})
	resultRTL := shaper.Shape("Hello", FontKey{Size: 16, Direction: DirectionRTL})

	// Both should produce results (direction affects glyph order)
	if resultLTR == nil || resultRTL == nil {
		t.Error("both directions should produce results")
	}
}

// --- FontKey tests ---

func TestFontKeyValid(t *testing.T) {
	key := FontKey{Size: 16}
	if !key.Valid() {
		t.Error("FontKey with positive size should be valid")
	}

	invalid := FontKey{Size: 0}
	if invalid.Valid() {
		t.Error("FontKey with zero size should not be valid")
	}

	negative := FontKey{Size: -1}
	if negative.Valid() {
		t.Error("FontKey with negative size should not be valid")
	}
}

func TestFontKeyCacheKey(t *testing.T) {
	key1 := FontKey{Size: 16, Bold: false}
	key2 := FontKey{Size: 16, Bold: false}
	key3 := FontKey{Size: 16, Bold: true}

	if key1.CacheKey() != key2.CacheKey() {
		t.Error("identical keys should have same cache key")
	}
	if key1.CacheKey() == key3.CacheKey() {
		t.Error("different keys should have different cache keys")
	}
}

// --- ShapedText tests ---

func TestShapedTextGlyphCount(t *testing.T) {
	shaper := NewTextShaper()

	result := shaper.Shape("Hello", FontKey{Size: 16})
	if result == nil {
		t.Fatal("Shape returned nil")
	}

	// "Hello" has 5 characters
	if len(result.Glyphs) != 5 {
		t.Errorf("expected 5 glyphs for 'Hello', got %d", len(result.Glyphs))
	}
}

func TestShapedTextGlyphPositions(t *testing.T) {
	shaper := NewTextShaper()

	result := shaper.Shape("ABC", FontKey{Size: 16})
	if result == nil {
		t.Fatal("Shape returned nil")
	}

	// Glyphs should be positioned left to right
	for i := 1; i < len(result.Glyphs); i++ {
		if result.Glyphs[i].X <= result.Glyphs[i-1].X {
			t.Errorf("glyph %d should be to the right of glyph %d", i, i-1)
		}
	}
}

// --- Text wrapping tests ---

func TestMeasureTextWrapped(t *testing.T) {
	shaper := NewTextShaper()

	// Long text that should wrap
	text := "This is a long text that should wrap at the specified width"
	maxWidth := float32(100)

	lines := shaper.MeasureWrapped(text, FontKey{Size: 16}, maxWidth)
	if len(lines) == 0 {
		t.Error("expected at least one line")
	}
	if len(lines) == 1 {
		t.Error("expected multiple lines for long text with limited width")
	}
}

func TestMeasureTextWrappedShortText(t *testing.T) {
	shaper := NewTextShaper()

	// Short text that should not wrap
	text := "Short"
	maxWidth := float32(200)

	lines := shaper.MeasureWrapped(text, FontKey{Size: 16}, maxWidth)
	if len(lines) != 1 {
		t.Errorf("expected 1 line for short text, got %d", len(lines))
	}
}

func TestMeasureTextWrappedEmpty(t *testing.T) {
	shaper := NewTextShaper()

	lines := shaper.MeasureWrapped("", FontKey{Size: 16}, 100)
	if len(lines) != 0 {
		t.Errorf("expected 0 lines for empty text, got %d", len(lines))
	}
}

func TestMeasureTextWrappedLongWord(t *testing.T) {
	shaper := NewTextShaper()

	// Single long word that exceeds max width
	text := "Supercalifragilisticexpialidocious"
	maxWidth := float32(50)

	lines := shaper.MeasureWrapped(text, FontKey{Size: 16}, maxWidth)
	// Should still produce at least one line (word doesn't break)
	if len(lines) == 0 {
		t.Error("expected at least one line even for long word")
	}
}

// --- Whitespace mode tests ---

func TestMeasureTextWrappedWhiteSpacePre(t *testing.T) {
	shaper := NewTextShaper()

	// Text with newlines in pre mode
	text := "Line 1\nLine 2\nLine 3"
	maxWidth := float32(200)

	lines := shaper.MeasureWrapped(text, FontKey{Size: 16}, maxWidth)
	// Should preserve newlines
	if len(lines) != 3 {
		t.Errorf("expected 3 lines for pre-formatted text, got %d", len(lines))
	}
}

func TestMeasureTextWrappedWhiteSpaceNoWrap(t *testing.T) {
	shaper := NewTextShaper()

	// Long text in no-wrap mode
	text := "This is a long text that should not wrap"
	maxWidth := float32(50)

	lines := shaper.MeasureWrappedNoWrap(text, FontKey{Size: 16}, maxWidth)
	// Should not wrap
	if len(lines) != 1 {
		t.Errorf("expected 1 line for no-wrap mode, got %d", len(lines))
	}
}

// --- Mixed styles tests ---

func TestShapeTextMixedStyles(t *testing.T) {
	shaper := NewTextShaper()

	// Shape text with different styles
	result1 := shaper.Shape("Hello", FontKey{Size: 16, Bold: true})
	result2 := shaper.Shape(" World", FontKey{Size: 16, Italic: true})

	if result1 == nil || result2 == nil {
		t.Error("both shapes should produce results")
	}

	// Combined width should be sum
	totalWidth := result1.Width + result2.Width
	if totalWidth <= 0 {
		t.Error("combined width should be positive")
	}
}

// --- Benchmarks ---

func BenchmarkTextShaperShape(b *testing.B) {
	shaper := NewTextShaper()
	key := FontKey{Size: 16}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		shaper.Shape("Hello, World!", key)
	}
}

func BenchmarkTextShaperShapeCached(b *testing.B) {
	shaper := NewTextShaper()
	key := FontKey{Size: 16}

	// Warm cache
	shaper.Shape("Hello, World!", key)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		shaper.Shape("Hello, World!", key)
	}
}

func BenchmarkTextShaperMeasureWrapped(b *testing.B) {
	shaper := NewTextShaper()
	key := FontKey{Size: 16}
	text := "This is a long text that should wrap at the specified width for benchmarking"

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		shaper.MeasureWrapped(text, key, 100)
	}
}

func BenchmarkFontKeyCacheKey(b *testing.B) {
	key := FontKey{Size: 16, Bold: true, Italic: false}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		key.CacheKey()
	}
}
