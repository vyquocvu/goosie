package css_test

import (
	"testing"
	"github.com/vyquocvu/goosie/internal/css"
)

// TestAdversarialCascadeSpecificity verifies spec-compliant specificity calculations
// across complex selector chains, combinators, IDs, classes, attributes, and pseudo-classes.
func TestAdversarialCascadeSpecificity(t *testing.T) {
	tests := []struct {
		name        string
		selector    string
		specificity [3]uint16 // [id, class/attr/pseudo-class, tag/pseudo-element]
	}{
		{
			name:        "Complex multi-combinator selector with multiple IDs across levels",
			selector:    `#header.main > nav#primary.nav.active[data-role="menu"]:first-child:hover div.item > span.label::before { color: red; }`,
			// IDs: #header (1), #primary (1) = 2
			// Classes/attrs/pseudos: .main, .nav, .active, [data-role="menu"], :first-child, :hover, .item, .label = 8
			// Tags/pseudo-elements: nav, div, span, ::before = 4
			specificity: [3]uint16{2, 8, 4},
		},
		{
			name:        "Multiple attribute operators on element",
			selector:    `a[href^="https://"][href$=".pdf"][title*="report"][rel~="nofollow"] { color: blue; }`,
			// IDs: 0
			// Classes/attrs/pseudos: [href^=], [href$=], [title*=], [rel~=] = 4
			// Tags: a = 1
			specificity: [3]uint16{0, 4, 1},
		},
		{
			name:        "Deep combinator chain with universal and tags",
			selector:    `body > div.main section#hero + article ~ footer p * { margin: 0; }`,
			// IDs: #hero = 1
			// Classes: .main = 1
			// Tags: body, div, section, article, footer, p = 6 (* is universal = 0)
			specificity: [3]uint16{1, 1, 6},
		},
		{
			name:        "Repeated identical classes and attributes",
			selector:    `.btn.btn.btn.btn[disabled][disabled] { opacity: 0.5; }`,
			// IDs: 0
			// Classes/attrs: 4 classes + 2 attrs = 6
			// Tags: 0
			specificity: [3]uint16{0, 6, 0},
		},
		{
			name:        "Sibling combinators with ID and pseudo-classes",
			selector:    `h1#title:first-child + p.lead:not(.draft) ~ div[data-content] { font-size: 16px; }`,
			// IDs: #title = 1
			// Classes/attrs/pseudos: :first-child, .lead, :not, [data-content] = 4
			// Tags: h1, p, div = 3
			specificity: [3]uint16{1, 4, 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := css.NewParser(tt.selector)
			sheet, err := p.Parse()
			if err != nil {
				t.Fatalf("Parse() failed for '%s': %v", tt.selector, err)
			}
			if len(sheet.Rules) != 1 || len(sheet.Rules[0].Selectors) != 1 {
				t.Fatalf("Unexpected rule structure: %+v", sheet.Rules)
			}
			got := css.ComputeSpecificity(&sheet.Rules[0].Selectors[0])
			if got != tt.specificity {
				t.Errorf("Specificity mismatch for '%s': got %v, want %v", tt.selector, got, tt.specificity)
			}
		})
	}
}

// TestAdversarialCascadeOrdering verifies that CompareSpecificity correctly establishes
// strict total order between subtle specificity differences.
func TestAdversarialCascadeOrdering(t *testing.T) {
	// A: 1 ID, 0 classes, 0 tags  -> [1, 0, 0]
	// B: 0 IDs, 10 classes, 10 tags -> [0, 10, 10]
	// C: 0 IDs, 10 classes, 9 tags  -> [0, 10, 9]
	// D: 0 IDs, 10 classes, 10 tags -> [0, 10, 10]
	specA := [3]uint16{1, 0, 0}
	specB := [3]uint16{0, 10, 10}
	specC := [3]uint16{0, 10, 9}
	specD := [3]uint16{0, 10, 10}

	if css.CompareSpecificity(specA, specB) <= 0 {
		t.Errorf("Expected specA [1,0,0] > specB [0,10,10]")
	}
	if css.CompareSpecificity(specB, specC) <= 0 {
		t.Errorf("Expected specB [0,10,10] > specC [0,10,9]")
	}
	if css.CompareSpecificity(specB, specD) != 0 {
		t.Errorf("Expected specB [0,10,10] == specD [0,10,10]")
	}
	if css.CompareSpecificity(specC, specA) >= 0 {
		t.Errorf("Expected specC [0,10,9] < specA [1,0,0]")
	}
}
