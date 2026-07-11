package css

import (
	"image/color"
	"testing"
)

// --- InheritedStyle tests ---

func TestInheritedStyleDefaults(t *testing.T) {
	s := DefaultInheritedStyle()
	if s.Color != color.Black {
		t.Errorf("expected default Color to be black, got %v", s.Color)
	}
	if s.FontSize != 16.0 {
		t.Errorf("expected default FontSize 16, got %f", s.FontSize)
	}
	if s.FontWeight != "normal" {
		t.Errorf("expected default FontWeight 'normal', got %q", s.FontWeight)
	}
	if s.FontFamily != "" {
		t.Errorf("expected default FontFamily empty, got %q", s.FontFamily)
	}
	if s.FontStyle != "normal" {
		t.Errorf("expected default FontStyle 'normal', got %q", s.FontStyle)
	}
	if s.LineHeight != 0 {
		t.Errorf("expected default LineHeight 0, got %f", s.LineHeight)
	}
	if s.TextAlign != "left" {
		t.Errorf("expected default TextAlign 'left', got %q", s.TextAlign)
	}
	if s.TextTransform != "none" {
		t.Errorf("expected default TextTransform 'none', got %q", s.TextTransform)
	}
	if s.Visibility != "visible" {
		t.Errorf("expected default Visibility 'visible', got %q", s.Visibility)
	}
	if s.Opacity != 1.0 {
		t.Errorf("expected default Opacity 1.0, got %f", s.Opacity)
	}
}

func TestInheritedStyleEquality(t *testing.T) {
	a := DefaultInheritedStyle()
	b := DefaultInheritedStyle()
	if !a.Equal(&b) {
		t.Error("two default InheritedStyle should be equal")
	}

	b.Color = color.RGBA{R: 255, G: 0, B: 0, A: 255}
	if a.Equal(&b) {
		t.Error("different Color should make them unequal")
	}
}

// --- ComputedStyle tests ---

func TestComputedStyleDefaults(t *testing.T) {
	cs := DefaultComputedStyle()
	if cs.NonInherited.Display != "inline" {
		t.Errorf("expected default Display 'inline', got %q", cs.NonInherited.Display)
	}
	if cs.NonInherited.Position != "static" {
		t.Errorf("expected default Position 'static', got %q", cs.NonInherited.Position)
	}
	if cs.Inherited.Visibility != "visible" {
		t.Errorf("expected inherited Visibility 'visible', got %q", cs.Inherited.Visibility)
	}
	if cs.Inherited.FontSize != 16.0 {
		t.Errorf("expected inherited FontSize 16, got %f", cs.Inherited.FontSize)
	}
}

func TestComputedStyleInheritance(t *testing.T) {
	parent := DefaultComputedStyle()
	parent.Inherited.Color = color.RGBA{R: 128, G: 64, B: 32, A: 255}
	parent.Inherited.FontSize = 24.0
	parent.Inherited.FontWeight = "bold"
	parent.Inherited.FontFamily = "Arial"
	parent.Inherited.TextAlign = "center"

	child := DefaultComputedStyle()
	child.InheritFrom(&parent.Inherited)

	if child.Inherited.Color != parent.Inherited.Color {
		t.Errorf("child Color should inherit from parent")
	}
	if child.Inherited.FontSize != parent.Inherited.FontSize {
		t.Errorf("child FontSize should inherit from parent")
	}
	if child.Inherited.FontWeight != parent.Inherited.FontWeight {
		t.Errorf("child FontWeight should inherit from parent")
	}
	if child.Inherited.FontFamily != parent.Inherited.FontFamily {
		t.Errorf("child FontFamily should inherit from parent")
	}
	if child.Inherited.TextAlign != parent.Inherited.TextAlign {
		t.Errorf("child TextAlign should inherit from parent")
	}

	// Non-inherited properties should NOT propagate
	if child.NonInherited.Display != "inline" {
		t.Errorf("child Display should remain default, got %q", child.NonInherited.Display)
	}
	if child.NonInherited.Position != "static" {
		t.Errorf("child Position should remain default, got %q", child.NonInherited.Position)
	}
}

func TestNonInheritedPropertiesNotPropagated(t *testing.T) {
	parent := DefaultComputedStyle()
	parent.NonInherited.Display = "flex"
	parent.NonInherited.Position = "absolute"
	parent.NonInherited.MarginTop = "10px"
	parent.NonInherited.PaddingLeft = "5px"
	parent.NonInherited.ZIndex = 5
	parent.NonInherited.Float = "left"

	child := DefaultComputedStyle()
	child.InheritFrom(&parent.Inherited)

	if child.NonInherited.Display == "flex" {
		t.Error("Display should NOT be inherited")
	}
	if child.NonInherited.Position == "absolute" {
		t.Error("Position should NOT be inherited")
	}
	if child.NonInherited.MarginTop == "10px" {
		t.Error("MarginTop should NOT be inherited")
	}
	if child.NonInherited.PaddingLeft == "5px" {
		t.Error("PaddingLeft should NOT be inherited")
	}
	if child.NonInherited.ZIndex == 5 {
		t.Error("ZIndex should NOT be inherited")
	}
	if child.NonInherited.Float == "left" {
		t.Error("Float should NOT be inherited")
	}
}

// --- Fingerprint tests ---

func TestInheritedStyleFingerprint(t *testing.T) {
	a := DefaultInheritedStyle()
	b := DefaultInheritedStyle()

	fpA := a.Fingerprint()
	fpB := b.Fingerprint()
	if fpA != fpB {
		t.Error("identical InheritedStyle should have same fingerprint")
	}

	b.Color = color.RGBA{R: 255, G: 0, B: 0, A: 255}
	fpC := b.Fingerprint()
	if fpA == fpC {
		t.Error("different InheritedStyle should have different fingerprint")
	}
}

func TestFingerprintDeterministic(t *testing.T) {
	s := DefaultInheritedStyle()
	s.FontSize = 20.0
	s.Color = color.RGBA{R: 100, G: 200, B: 50, A: 255}

	fp1 := s.Fingerprint()
	fp2 := s.Fingerprint()
	if fp1 != fp2 {
		t.Error("fingerprint should be deterministic")
	}
}

// --- StylePool tests ---

func TestStylePoolDeduplication(t *testing.T) {
	pool := NewStylePool()

	a := DefaultInheritedStyle()
	b := DefaultInheritedStyle()

	refA := pool.Intern(&a)
	refB := pool.Intern(&b)

	if refA != refB {
		t.Error("identical InheritedStyle should return same reference")
	}

	stats := pool.Stats()
	if stats.Entries != 1 {
		t.Errorf("expected 1 entry in pool, got %d", stats.Entries)
	}
	if stats.DedupCount != 1 {
		t.Errorf("expected 1 dedup, got %d", stats.DedupCount)
	}
}

func TestStylePoolDistinctEntries(t *testing.T) {
	pool := NewStylePool()

	a := DefaultInheritedStyle()
	b := DefaultInheritedStyle()
	b.Color = color.RGBA{R: 255, G: 0, B: 0, A: 255}

	refA := pool.Intern(&a)
	refB := pool.Intern(&b)

	if refA == refB {
		t.Error("different InheritedStyle should return different references")
	}

	stats := pool.Stats()
	if stats.Entries != 2 {
		t.Errorf("expected 2 entries in pool, got %d", stats.Entries)
	}
	if stats.DedupCount != 0 {
		t.Errorf("expected 0 dedup, got %d", stats.DedupCount)
	}
}

func TestStylePoolBoundedEviction(t *testing.T) {
	pool := NewStylePoolWithLimit(4)

	// Insert 4 distinct styles
	for i := 0; i < 4; i++ {
		s := DefaultInheritedStyle()
		s.FontSize = float32(10 + i)
		pool.Intern(&s)
	}

	stats := pool.Stats()
	if stats.Entries != 4 {
		t.Errorf("expected 4 entries, got %d", stats.Entries)
	}

	// Insert one more — should evict oldest
	s := DefaultInheritedStyle()
	s.FontSize = 99
	pool.Intern(&s)

	stats = pool.Stats()
	if stats.Entries > 4 {
		t.Errorf("pool should not exceed limit, got %d entries", stats.Entries)
	}
}

func TestStylePoolConcurrency(t *testing.T) {
	pool := NewStylePool()
	done := make(chan struct{})

	// Concurrent intern from multiple goroutines
	for i := 0; i < 8; i++ {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			s := DefaultInheritedStyle()
			s.FontSize = float32(10 + id)
			pool.Intern(&s)
		}(i)
	}

	for i := 0; i < 8; i++ {
		<-done
	}

	stats := pool.Stats()
	if stats.Entries != 8 {
		t.Errorf("expected 8 entries after concurrent insert, got %d", stats.Entries)
	}
}

// --- ApplyDeclarations tests ---

func TestApplyDeclarationsToInherited(t *testing.T) {
	decls := []Declaration{
		{Property: "color", Value: "red", PropertyAtom: internPropertyName("color"), IsHot: true},
		{Property: "font-size", Value: "24px", PropertyAtom: internPropertyName("font-size"), IsHot: true},
		{Property: "font-weight", Value: "bold", PropertyAtom: internPropertyName("font-weight"), IsHot: true},
		{Property: "text-align", Value: "center", PropertyAtom: internPropertyName("text-align"), IsHot: true},
	}

	s := DefaultInheritedStyle()
	ApplyDeclarationsToInherited(&s, decls)

	if s.FontSize != 24.0 {
		t.Errorf("expected FontSize 24, got %f", s.FontSize)
	}
	if s.FontWeight != "bold" {
		t.Errorf("expected FontWeight 'bold', got %q", s.FontWeight)
	}
	if s.TextAlign != "center" {
		t.Errorf("expected TextAlign 'center', got %q", s.TextAlign)
	}
}

func TestApplyDeclarationsToNonInherited(t *testing.T) {
	decls := []Declaration{
		{Property: "display", Value: "flex", PropertyAtom: internPropertyName("display"), IsHot: true},
		{Property: "position", Value: "absolute", PropertyAtom: internPropertyName("position"), IsHot: true},
		{Property: "margin-top", Value: "10px", PropertyAtom: internPropertyName("margin-top"), IsHot: true},
		{Property: "z-index", Value: "5", PropertyAtom: internPropertyName("z-index"), IsHot: true},
	}

	cs := DefaultComputedStyle()
	ApplyDeclarationsToNonInherited(&cs.NonInherited, decls)

	if cs.NonInherited.Display != "flex" {
		t.Errorf("expected Display 'flex', got %q", cs.NonInherited.Display)
	}
	if cs.NonInherited.Position != "absolute" {
		t.Errorf("expected Position 'absolute', got %q", cs.NonInherited.Position)
	}
	if cs.NonInherited.MarginTop != "10px" {
		t.Errorf("expected MarginTop '10px', got %q", cs.NonInherited.MarginTop)
	}
	if cs.NonInherited.ZIndex != 5 {
		t.Errorf("expected ZIndex 5, got %d", cs.NonInherited.ZIndex)
	}
}

func TestApplyDeclarationsEmptyValue(t *testing.T) {
	decls := []Declaration{
		{Property: "font-size", Value: "", PropertyAtom: internPropertyName("font-size"), IsHot: true},
	}

	s := DefaultInheritedStyle()
	ApplyDeclarationsToInherited(&s, decls)

	// Empty value should not change the default
	if s.FontSize != 16.0 {
		t.Errorf("empty value should not change FontSize, got %f", s.FontSize)
	}
}

func TestApplyDeclarationsColdProperty(t *testing.T) {
	decls := []Declaration{
		{Property: "-webkit-transform", Value: "rotate(45deg)", PropertyAtom: 0, IsHot: false},
	}

	s := DefaultInheritedStyle()
	ApplyDeclarationsToInherited(&s, decls)
	// Cold properties are ignored in typed struct — no panic
}

// --- IsInheritedProperty tests ---

func TestIsInheritedProperty(t *testing.T) {
	inherited := []string{
		"color", "font-size", "font-weight", "font-family", "font-style",
		"line-height", "text-align", "text-transform", "text-indent",
		"letter-spacing", "word-spacing", "white-space", "visibility",
		"opacity", "list-style-type", "border-collapse", "border-spacing",
		"vertical-align", "text-decoration",
	}
	for _, prop := range inherited {
		if !IsInheritedProperty(prop) {
			t.Errorf("expected %q to be inherited", prop)
		}
	}

	nonInherited := []string{
		"display", "position", "margin", "padding", "border",
		"width", "height", "float", "clear", "z-index",
		"overflow", "top", "right", "bottom", "left",
	}
	for _, prop := range nonInherited {
		if IsInheritedProperty(prop) {
			t.Errorf("expected %q to be non-inherited", prop)
		}
	}
}

// --- Edge case tests ---

func TestComputedStyleClone(t *testing.T) {
	cs := DefaultComputedStyle()
	cs.NonInherited.Display = "block"
	cs.Inherited.Color = color.RGBA{R: 100, G: 100, B: 100, A: 255}

	clone := cs.Clone()
	if clone.NonInherited.Display != cs.NonInherited.Display {
		t.Error("clone should have same Display")
	}
	if clone.Inherited.Color != cs.Inherited.Color {
		t.Error("clone should have same Color")
	}

	// Modifying clone should not affect original
	clone.NonInherited.Display = "inline"
	if cs.NonInherited.Display == "inline" {
		t.Error("modifying clone should not affect original")
	}
}

func TestStylePoolReset(t *testing.T) {
	pool := NewStylePool()
	s := DefaultInheritedStyle()
	pool.Intern(&s)

	pool.Reset()
	stats := pool.Stats()
	if stats.Entries != 0 {
		t.Errorf("expected 0 entries after reset, got %d", stats.Entries)
	}
}

func TestInheritedStyleNilEqual(t *testing.T) {
	a := DefaultInheritedStyle()
	if a.Equal(nil) {
		t.Error("non-nil should not equal nil")
	}
}

func TestStylePoolInternNil(t *testing.T) {
	pool := NewStylePool()
	ref := pool.Intern(nil)
	if ref != nil {
		t.Error("Intern(nil) should return nil")
	}
}

func TestComputedStyleInheritFromNil(t *testing.T) {
	cs := DefaultComputedStyle()
	original := cs.Inherited
	cs.InheritFrom(nil)
	// Should not modify when parent is nil
	if cs.Inherited.FontSize != original.FontSize {
		t.Error("InheritFrom(nil) should not modify style")
	}
}

// --- Benchmarks ---

func BenchmarkInheritedStyleFingerprint(b *testing.B) {
	s := DefaultInheritedStyle()
	s.Color = color.RGBA{R: 128, G: 64, B: 32, A: 255}
	s.FontSize = 20.0
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = s.Fingerprint()
	}
}

func BenchmarkInheritedStyleEqual(b *testing.B) {
	a := DefaultInheritedStyle()
	bStyle := DefaultInheritedStyle()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = a.Equal(&bStyle)
	}
}

func BenchmarkStylePoolInternHit(b *testing.B) {
	pool := NewStylePool()
	s := DefaultInheritedStyle()
	pool.Intern(&s) // warm up
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pool.Intern(&s)
	}
}

func BenchmarkStylePoolInternMiss(b *testing.B) {
	pool := NewStylePool()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s := DefaultInheritedStyle()
		s.FontSize = float32(i % 100)
		pool.Intern(&s)
	}
}

func BenchmarkApplyDeclarationsInherited(b *testing.B) {
	decls := []Declaration{
		{Property: "color", Value: "red", PropertyAtom: internPropertyName("color"), IsHot: true},
		{Property: "font-size", Value: "24px", PropertyAtom: internPropertyName("font-size"), IsHot: true},
		{Property: "font-weight", Value: "bold", PropertyAtom: internPropertyName("font-weight"), IsHot: true},
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s := DefaultInheritedStyle()
		ApplyDeclarationsToInherited(&s, decls)
	}
}

func BenchmarkApplyDeclarationsNonInherited(b *testing.B) {
	decls := []Declaration{
		{Property: "display", Value: "flex", PropertyAtom: internPropertyName("display"), IsHot: true},
		{Property: "margin-top", Value: "10px", PropertyAtom: internPropertyName("margin-top"), IsHot: true},
		{Property: "padding-left", Value: "5px", PropertyAtom: internPropertyName("padding-left"), IsHot: true},
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		cs := DefaultComputedStyle()
		ApplyDeclarationsToNonInherited(&cs.NonInherited, decls)
	}
}

func BenchmarkComputedStyleInherit(b *testing.B) {
	parent := DefaultComputedStyle()
	parent.Inherited.Color = color.RGBA{R: 128, G: 64, B: 32, A: 255}
	parent.Inherited.FontSize = 24.0
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		child := DefaultComputedStyle()
		child.InheritFrom(&parent.Inherited)
	}
}
