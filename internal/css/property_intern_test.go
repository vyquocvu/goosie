package css

import (
	"testing"
)

// TestM3.1_PropertyInterning verifies that property names are interned
func TestPropertyInterning(t *testing.T) {
	css := `p { color: red; font-size: 16px; margin: 10px; }`
	p := NewParser(css)
	sheet, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	if len(sheet.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(sheet.Rules))
	}

	rule := sheet.Rules[0]
	if len(rule.Declarations) != 3 {
		t.Fatalf("expected 3 declarations, got %d", len(rule.Declarations))
	}

	// Verify all properties are interned (non-zero atom)
	for _, decl := range rule.Declarations {
		if decl.PropertyAtom == 0 {
			t.Errorf("property %q not interned (atom is 0)", decl.Property)
		}
		// Verify PropertyAtom can be resolved back to string
		resolved := GetPropertyName(decl.PropertyAtom)
		if resolved != decl.Property {
			t.Errorf("PropertyAtom resolved to %q, expected %q", resolved, decl.Property)
		}
	}
}

// TestM3.1_HotColdClassification verifies hot/cold property classification
func TestHotColdClassification(t *testing.T) {
	css := `p { color: red; font-size: 16px; -webkit-transform: rotate(45deg); }`
	p := NewParser(css)
	sheet, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	rule := sheet.Rules[0]

	// color and font-size are hot
	if !rule.Declarations[0].IsHot {
		t.Errorf("expected 'color' to be hot")
	}
	if !rule.Declarations[1].IsHot {
		t.Errorf("expected 'font-size' to be hot")
	}
	// -webkit-transform is cold (not in hot set)
	if rule.Declarations[2].IsHot {
		t.Errorf("expected '-webkit-transform' to be cold")
	}
}

// TestM3.1_SourceOrder verifies source order tracking
func TestSourceOrder(t *testing.T) {
	css := `
		h1 { color: red; }
		p { font-size: 16px; }
		div { margin: 10px; }
	`
	p := NewParser(css)
	sheet, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	if len(sheet.Rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(sheet.Rules))
	}

	// Verify source order is monotonic
	for i, rule := range sheet.Rules {
		expected := uint32(i + 1)
		if rule.SourceOrder != expected {
			t.Errorf("rule %d: expected SourceOrder %d, got %d", i, expected, rule.SourceOrder)
		}
	}
}

// TestM3.1_Origin verifies origin assignment
func TestOrigin(t *testing.T) {
	css := `p { color: red; }`

	// Test default origin (Author)
	p := NewParser(css)
	sheet, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}
	if sheet.Rules[0].Origin != OriginAuthor {
		t.Errorf("expected OriginAuthor, got %d", sheet.Rules[0].Origin)
	}

	// Test custom origin
	config := ParseConfig{Origin: OriginUserAgent}
	p2 := NewParserWithConfig(css, config)
	sheet2, err := p2.Parse()
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}
	if sheet2.Rules[0].Origin != OriginUserAgent {
		t.Errorf("expected OriginUserAgent, got %d", sheet2.Rules[0].Origin)
	}
}

// TestM3.1_MaxBytes verifies byte limit enforcement
func TestMaxBytes(t *testing.T) {
	css := `p { color: red; } div { margin: 10px; }`

	// Parse with limit that cuts off second rule
	config := ParseConfig{MaxBytes: 20}
	p := NewParserWithConfig(css, config)
	sheet, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	// Should only parse first rule due to byte limit
	if len(sheet.Rules) != 1 {
		t.Errorf("expected 1 rule (limited by MaxBytes), got %d", len(sheet.Rules))
	}
}

// TestM3.1_BackwardCompatibility verifies existing API still works
func TestBackwardCompatibility(t *testing.T) {
	css := `h1 { display: block; font-size: 32px; }`
	p := NewParser(css)
	sheet, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	rule := sheet.Rules[0]
	decl := rule.Declarations[0]

	// Old API should still work
	if decl.Property != "display" {
		t.Errorf("expected Property 'display', got %q", decl.Property)
	}
	if decl.Value != "block" {
		t.Errorf("expected Value 'block', got %q", decl.Value)
	}
}

// TestM3.1_EmptyProperty verifies empty property handling
func TestEmptyProperty(t *testing.T) {
	css := `p { : red; }`
	p := NewParser(css)
	sheet, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	// Empty property should be skipped
	if len(sheet.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(sheet.Rules))
	}
	if len(sheet.Rules[0].Declarations) != 0 {
		t.Errorf("expected 0 declarations (empty property skipped), got %d", len(sheet.Rules[0].Declarations))
	}
}

// TestM3.1_PropertyAtomLookup verifies atom lookup
func TestPropertyAtomLookup(t *testing.T) {
	// Test lookup of known property
	a := lookupPropertyAtom("color")
	if a == 0 {
		t.Errorf("expected to find 'color' in property table")
	}

	// Test lookup of unknown property
	a2 := lookupPropertyAtom("unknown-property-xyz")
	if a2 != 0 {
		t.Errorf("expected AtomNone for unknown property, got %d", a2)
	}
}

// TestM3.1_HotPropertySet verifies hot property classification function
func TestHotPropertySet(t *testing.T) {
	hotProps := []string{
		"display", "color", "font-size", "margin", "padding",
		"width", "height", "background", "border", "flex",
	}

	for _, prop := range hotProps {
		if !isHotProperty(prop) {
			t.Errorf("expected %q to be hot", prop)
		}
	}

	coldProps := []string{
		"-webkit-transform", "animation", "transition", "filter",
	}

	for _, prop := range coldProps {
		if isHotProperty(prop) {
			t.Errorf("expected %q to be cold", prop)
		}
	}
}

// TestM3.1_ParseStyleAttribute verifies style attribute parsing with new fields
func TestParseStyleAttributeWithM31(t *testing.T) {
	decls, err := ParseStyleAttribute("color: red; font-size: 16px;")
	if err != nil {
		t.Fatalf("ParseStyleAttribute() failed: %v", err)
	}

	if len(decls) != 2 {
		t.Fatalf("expected 2 declarations, got %d", len(decls))
	}

	// Verify both have atoms and hot classification
	for _, decl := range decls {
		if decl.PropertyAtom == 0 {
			t.Errorf("property %q not interned", decl.Property)
		}
		if !decl.IsHot {
			t.Errorf("expected %q to be hot", decl.Property)
		}
	}
}
