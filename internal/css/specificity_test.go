package css

import (
	"testing"
)

// TestComputeSpecificity verifies CSS specificity calculation per spec
func TestComputeSpecificity(t *testing.T) {
	tests := []struct {
		name        string
		selector    string
		specificity [3]uint16 // [id, class, tag]
	}{
		{
			name:        "universal selector",
			selector:    `* { color: red; }`,
			specificity: [3]uint16{0, 0, 0},
		},
		{
			name:        "type selector",
			selector:    `p { color: red; }`,
			specificity: [3]uint16{0, 0, 1},
		},
		{
			name:        "class selector",
			selector:    `.active { color: red; }`,
			specificity: [3]uint16{0, 1, 0},
		},
		{
			name:        "id selector",
			selector:    `#main { color: red; }`,
			specificity: [3]uint16{1, 0, 0},
		},
		{
			name:        "type + class",
			selector:    `p.active { color: red; }`,
			specificity: [3]uint16{0, 1, 1},
		},
		{
			name:        "type + id + class",
			selector:    `p#main.active { color: red; }`,
			specificity: [3]uint16{1, 1, 1},
		},
		{
			name:        "multiple classes",
			selector:    `.foo.bar.baz { color: red; }`,
			specificity: [3]uint16{0, 3, 0},
		},
		{
			name:        "attribute selector",
			selector:    `[type="text"] { color: red; }`,
			specificity: [3]uint16{0, 1, 0},
		},
		{
			name:        "type + attribute",
			selector:    `input[type="text"] { color: red; }`,
			specificity: [3]uint16{0, 1, 1},
		},
		{
			name:        "pseudo-class",
			selector:    `:hover { color: red; }`,
			specificity: [3]uint16{0, 1, 0},
		},
		{
			name:        "pseudo-element",
			selector:    `::before { content: ""; }`,
			specificity: [3]uint16{0, 0, 1},
		},
		{
			name:        "descendant combinator",
			selector:    `div p { color: red; }`,
			specificity: [3]uint16{0, 0, 2},
		},
		{
			name:        "child combinator",
			selector:    `div > p { color: red; }`,
			specificity: [3]uint16{0, 0, 2},
		},
		{
			name:        "complex selector",
			selector:    `div.container > p#intro.highlight:first-child { color: red; }`,
			specificity: [3]uint16{1, 3, 2}, // #intro + .container + .highlight + :first-child = 1,3,0 + div + p = 1,3,2
		},
		{
			name:        "adjacent sibling",
			selector:    `h1 + p { color: red; }`,
			specificity: [3]uint16{0, 0, 2},
		},
		{
			name:        "general sibling",
			selector:    `h1 ~ p { color: red; }`,
			specificity: [3]uint16{0, 0, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewParser(tt.selector)
			sheet, err := p.Parse()
			if err != nil {
				t.Fatalf("Parse() failed: %v", err)
			}
			if len(sheet.Rules) != 1 {
				t.Fatalf("expected 1 rule, got %d", len(sheet.Rules))
			}
			if len(sheet.Rules[0].Selectors) != 1 {
				t.Fatalf("expected 1 selector, got %d", len(sheet.Rules[0].Selectors))
			}

			spec := ComputeSpecificity(&sheet.Rules[0].Selectors[0])
			if spec != tt.specificity {
				t.Errorf("ComputeSpecificity() = %v, want %v", spec, tt.specificity)
			}
		})
	}
}

// TestCompareSpecificity verifies specificity comparison
func TestCompareSpecificity(t *testing.T) {
	tests := []struct {
		name string
		a, b [3]uint16
		want int // -1 if a < b, 0 if equal, 1 if a > b
	}{
		{"equal", [3]uint16{1, 2, 3}, [3]uint16{1, 2, 3}, 0},
		{"id wins", [3]uint16{1, 0, 0}, [3]uint16{0, 10, 10}, 1},
		{"class wins over tag", [3]uint16{0, 1, 0}, [3]uint16{0, 0, 100}, 1},
		{"less specific id", [3]uint16{0, 5, 0}, [3]uint16{1, 0, 0}, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CompareSpecificity(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("CompareSpecificity(%v, %v) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// TestCompileStyleSheet verifies basic compilation
func TestCompileStyleSheet(t *testing.T) {
	css := `
		h1 { color: red; }
		.container { max-width: 1200px; }
		#main { background: white; }
		* { margin: 0; }
		[type="text"] { border: 1px solid; }
	`
	p := NewParser(css)
	sheet, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	compiled := CompileStyleSheet(sheet)

	// Verify all rules are compiled
	totalRules := len(compiled.idBucket) +
		len(compiled.classBucket) +
		len(compiled.tagBucket) +
		len(compiled.attrBucket) +
		len(compiled.universalBucket)

	if totalRules != 5 {
		t.Errorf("expected 5 total compiled rules, got %d", totalRules)
	}

	// Verify ID bucketing
	if len(compiled.idBucket["main"]) != 1 {
		t.Errorf("expected 1 rule in #main bucket, got %d", len(compiled.idBucket["main"]))
	}

	// Verify class bucketing
	if len(compiled.classBucket["container"]) != 1 {
		t.Errorf("expected 1 rule in .container bucket, got %d", len(compiled.classBucket["container"]))
	}

	// Verify tag bucketing
	if len(compiled.tagBucket["h1"]) != 1 {
		t.Errorf("expected 1 rule in h1 bucket, got %d", len(compiled.tagBucket["h1"]))
	}

	// Verify universal bucketing
	if len(compiled.universalBucket) != 1 {
		t.Errorf("expected 1 rule in universal bucket, got %d", len(compiled.universalBucket))
	}
}

// TestCompileSpecificityPrecompute verifies specificity is precomputed
func TestCompileSpecificityPrecompute(t *testing.T) {
	css := `
		p { color: red; }
		p.active { color: blue; }
		#main p { color: green; }
	`
	p := NewParser(css)
	sheet, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	compiled := CompileStyleSheet(sheet)

	if len(compiled.rules) != 3 {
		t.Fatalf("expected 3 compiled rules, got %d", len(compiled.rules))
	}

	// p: specificity [0,0,1]
	if compiled.rules[0].Specificity != [3]uint16{0, 0, 1} {
		t.Errorf("rule 0: expected specificity [0,0,1], got %v", compiled.rules[0].Specificity)
	}

	// p.active: specificity [0,1,1]
	if compiled.rules[1].Specificity != [3]uint16{0, 1, 1} {
		t.Errorf("rule 1: expected specificity [0,1,1], got %v", compiled.rules[1].Specificity)
	}

	// #main p: specificity [1,0,1]
	if compiled.rules[2].Specificity != [3]uint16{1, 0, 1} {
		t.Errorf("rule 2: expected specificity [1,0,1], got %v", compiled.rules[2].Specificity)
	}
}

// TestCompiledMatchByID verifies matching by ID selector
func TestCompiledMatchByID(t *testing.T) {
	css := `#main { color: red; }`
	p := NewParser(css)
	sheet, _ := p.Parse()
	compiled := CompileStyleSheet(sheet)

	// Element with matching ID
	elem := &testElement{tag: "div", id: "main", classes: nil}
	rules := compiled.MatchElement(elem)
	if len(rules) != 1 {
		t.Errorf("expected 1 matching rule, got %d", len(rules))
	}

	// Element without matching ID
	elem2 := &testElement{tag: "div", id: "other", classes: nil}
	rules2 := compiled.MatchElement(elem2)
	if len(rules2) != 0 {
		t.Errorf("expected 0 matching rules, got %d", len(rules2))
	}
}

// TestCompiledMatchByClass verifies matching by class selector
func TestCompiledMatchByClass(t *testing.T) {
	css := `.active { color: green; }`
	p := NewParser(css)
	sheet, _ := p.Parse()
	compiled := CompileStyleSheet(sheet)

	// Element with matching class
	elem := &testElement{tag: "div", id: "", classes: []string{"active", "visible"}}
	rules := compiled.MatchElement(elem)
	if len(rules) != 1 {
		t.Errorf("expected 1 matching rule, got %d", len(rules))
	}

	// Element without matching class
	elem2 := &testElement{tag: "div", id: "", classes: []string{"hidden"}}
	rules2 := compiled.MatchElement(elem2)
	if len(rules2) != 0 {
		t.Errorf("expected 0 matching rules, got %d", len(rules2))
	}
}

// TestCompiledMatchByTag verifies matching by tag selector
func TestCompiledMatchByTag(t *testing.T) {
	css := `p { margin: 10px; }`
	p := NewParser(css)
	sheet, _ := p.Parse()
	compiled := CompileStyleSheet(sheet)

	// Element with matching tag
	elem := &testElement{tag: "p", id: "", classes: nil}
	rules := compiled.MatchElement(elem)
	if len(rules) != 1 {
		t.Errorf("expected 1 matching rule, got %d", len(rules))
	}

	// Element with non-matching tag
	elem2 := &testElement{tag: "div", id: "", classes: nil}
	rules2 := compiled.MatchElement(elem2)
	if len(rules2) != 0 {
		t.Errorf("expected 0 matching rules, got %d", len(rules2))
	}
}

// TestCompiledMatchUniversal verifies universal selector matches all
func TestCompiledMatchUniversal(t *testing.T) {
	css := `* { margin: 0; }`
	p := NewParser(css)
	sheet, _ := p.Parse()
	compiled := CompileStyleSheet(sheet)

	elem := &testElement{tag: "anything", id: "", classes: nil}
	rules := compiled.MatchElement(elem)
	if len(rules) != 1 {
		t.Errorf("expected 1 matching rule for universal selector, got %d", len(rules))
	}
}

// TestCompiledMatchCompound verifies compound selector matching
func TestCompiledMatchCompound(t *testing.T) {
	css := `p.highlight { background: yellow; }`
	p := NewParser(css)
	sheet, _ := p.Parse()
	compiled := CompileStyleSheet(sheet)

	// Element matching both tag and class
	elem := &testElement{tag: "p", id: "", classes: []string{"highlight"}}
	rules := compiled.MatchElement(elem)
	if len(rules) != 1 {
		t.Errorf("expected 1 matching rule, got %d", len(rules))
	}

	// Element matching tag but not class
	elem2 := &testElement{tag: "p", id: "", classes: nil}
	rules2 := compiled.MatchElement(elem2)
	if len(rules2) != 0 {
		t.Errorf("expected 0 matching rules (class mismatch), got %d", len(rules2))
	}
}

// TestCompiledMatchAttribute verifies attribute selector matching
func TestCompiledMatchAttribute(t *testing.T) {
	css := `[type="text"] { border: 1px solid; }`
	p := NewParser(css)
	sheet, _ := p.Parse()
	compiled := CompileStyleSheet(sheet)

	// Element with matching attribute
	elem := &testElement{tag: "input", id: "", classes: nil, attrs: map[string]string{"type": "text"}}
	rules := compiled.MatchElement(elem)
	if len(rules) != 1 {
		t.Errorf("expected 1 matching rule, got %d", len(rules))
	}

	// Element without matching attribute
	elem2 := &testElement{tag: "input", id: "", classes: nil, attrs: map[string]string{"type": "checkbox"}}
	rules2 := compiled.MatchElement(elem2)
	if len(rules2) != 0 {
		t.Errorf("expected 0 matching rules, got %d", len(rules2))
	}
}

// TestCompiledMatchMultipleRules verifies multiple rules match correctly
func TestCompiledMatchMultipleRules(t *testing.T) {
	css := `
		p { margin: 10px; }
		.text { color: black; }
		#content { background: white; }
		* { box-sizing: border-box; }
	`
	p := NewParser(css)
	sheet, _ := p.Parse()
	compiled := CompileStyleSheet(sheet)

	// Element matching all rules
	elem := &testElement{tag: "p", id: "content", classes: []string{"text"}}
	rules := compiled.MatchElement(elem)
	if len(rules) != 4 {
		t.Errorf("expected 4 matching rules, got %d", len(rules))
	}
}

// TestCompiledMatchDescendant verifies descendant combinator matching
func TestCompiledMatchDescendant(t *testing.T) {
	css := `div p { color: red; }`
	p := NewParser(css)
	sheet, _ := p.Parse()
	compiled := CompileStyleSheet(sheet)

	// Build element tree: div > p
	parent := &testElement{tag: "div", id: "", classes: nil}
	child := &testElement{tag: "p", id: "", classes: nil, parent: parent}

	rules := compiled.MatchElement(child)
	if len(rules) != 1 {
		t.Errorf("expected 1 matching rule for descendant, got %d", len(rules))
	}

	// p without div ancestor should not match
	orphan := &testElement{tag: "p", id: "", classes: nil, parent: nil}
	rules2 := compiled.MatchElement(orphan)
	if len(rules2) != 0 {
		t.Errorf("expected 0 matching rules for orphan, got %d", len(rules2))
	}
}

// TestCompiledMatchChild verifies child combinator matching
func TestCompiledMatchChild(t *testing.T) {
	css := `div > p { color: red; }`
	p := NewParser(css)
	sheet, _ := p.Parse()
	compiled := CompileStyleSheet(sheet)

	// Direct child
	parent := &testElement{tag: "div", id: "", classes: nil}
	child := &testElement{tag: "p", id: "", classes: nil, parent: parent}

	rules := compiled.MatchElement(child)
	if len(rules) != 1 {
		t.Errorf("expected 1 matching rule for child, got %d", len(rules))
	}

	// Grandchild should not match
	grandchild := &testElement{tag: "p", id: "", classes: nil, parent: child}
	rules2 := compiled.MatchElement(grandchild)
	if len(rules2) != 0 {
		t.Errorf("expected 0 matching rules for grandchild, got %d", len(rules2))
	}
}

// TestCompiledMatchAdjacentSibling verifies adjacent sibling combinator
func TestCompiledMatchAdjacentSibling(t *testing.T) {
	css := `h1 + p { margin-top: 0; }`
	p := NewParser(css)
	sheet, _ := p.Parse()
	compiled := CompileStyleSheet(sheet)

	// Build: parent > [h1, p]
	parent := &testElement{tag: "div", id: "", classes: nil}
	h1 := &testElement{tag: "h1", id: "", classes: nil, parent: parent}
	pElem := &testElement{tag: "p", id: "", classes: nil, parent: parent, prevSibling: h1}
	parent.children = []*testElement{h1, pElem}

	rules := compiled.MatchElement(pElem)
	if len(rules) != 1 {
		t.Errorf("expected 1 matching rule for adjacent sibling, got %d", len(rules))
	}
}

// TestCompiledMatchGeneralSibling verifies general sibling combinator
func TestCompiledMatchGeneralSibling(t *testing.T) {
	css := `h1 ~ p { color: gray; }`
	p := NewParser(css)
	sheet, _ := p.Parse()
	compiled := CompileStyleSheet(sheet)

	// Build: parent > [h1, span, p]
	parent := &testElement{tag: "div", id: "", classes: nil}
	h1 := &testElement{tag: "h1", id: "", classes: nil, parent: parent}
	span := &testElement{tag: "span", id: "", classes: nil, parent: parent, prevSibling: h1}
	pElem := &testElement{tag: "p", id: "", classes: nil, parent: parent, prevSibling: span}
	parent.children = []*testElement{h1, span, pElem}

	rules := compiled.MatchElement(pElem)
	if len(rules) != 1 {
		t.Errorf("expected 1 matching rule for general sibling, got %d", len(rules))
	}
}

// TestCompiledMatchCaseInsensitiveTag verifies tag matching is case-insensitive
func TestCompiledMatchCaseInsensitiveTag(t *testing.T) {
	css := `P { color: red; }`
	p := NewParser(css)
	sheet, _ := p.Parse()
	compiled := CompileStyleSheet(sheet)

	// Lowercase element should match uppercase selector
	elem := &testElement{tag: "p", id: "", classes: nil}
	rules := compiled.MatchElement(elem)
	if len(rules) != 1 {
		t.Errorf("expected 1 matching rule for case-insensitive tag, got %d", len(rules))
	}
}

// TestCompileEmptyStyleSheet verifies compiling empty stylesheet
func TestCompileEmptyStyleSheet(t *testing.T) {
	sheet := &StyleSheet{}
	compiled := CompileStyleSheet(sheet)

	if len(compiled.rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(compiled.rules))
	}

	elem := &testElement{tag: "div", id: "", classes: nil}
	rules := compiled.MatchElement(elem)
	if len(rules) != 0 {
		t.Errorf("expected 0 matching rules, got %d", len(rules))
	}
}

// TestCompileMultipleSelectorsPerRule verifies comma-separated selectors
func TestCompileMultipleSelectorsPerRule(t *testing.T) {
	css := `h1, h2, h3 { font-family: Arial; }`
	p := NewParser(css)
	sheet, _ := p.Parse()
	compiled := CompileStyleSheet(sheet)

	// Should have 1 compiled rule with 3 selectors
	if len(compiled.rules) != 1 {
		t.Errorf("expected 1 compiled rule, got %d", len(compiled.rules))
	}
	if len(compiled.rules[0].Selectors) != 3 {
		t.Errorf("expected 3 selectors in compiled rule, got %d", len(compiled.rules[0].Selectors))
	}

	// Each tag should match
	for _, tag := range []string{"h1", "h2", "h3"} {
		elem := &testElement{tag: tag, id: "", classes: nil}
		rules := compiled.MatchElement(elem)
		if len(rules) != 1 {
			t.Errorf("expected 1 matching rule for %s, got %d", tag, len(rules))
		}
	}
}

// TestCompiledMatchAttributeOperators verifies different attribute operators
func TestCompiledMatchAttributeOperators(t *testing.T) {
	tests := []struct {
		name     string
		css      string
		attrVal  string
		expected int
	}{
		{"equals", `[type="text"] { }`, "text", 1},
		{"equals no match", `[type="text"] { }`, "checkbox", 0},
		{"starts with", `[href^="https"] { }`, "https://example.com", 1},
		{"starts with no match", `[href^="https"] { }`, "http://example.com", 0},
		{"ends with", `[src$=".png"] { }`, "image.png", 1},
		{"ends with no match", `[src$=".png"] { }`, "image.jpg", 0},
		{"contains", `[title*="example"] { }`, "this is example text", 1},
		{"contains no match", `[title*="example"] { }`, "no match here", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewParser(tt.css)
			sheet, _ := p.Parse()
			compiled := CompileStyleSheet(sheet)

			// Determine attribute name from CSS
			attrName := sheet.Rules[0].Selectors[0].Simple.Attributes[0].Name
			elem := &testElement{tag: "div", id: "", classes: nil, attrs: map[string]string{attrName: tt.attrVal}}
			rules := compiled.MatchElement(elem)
			if len(rules) != tt.expected {
				t.Errorf("expected %d matching rules, got %d", tt.expected, len(rules))
			}
		})
	}
}

// testElement is a minimal Element implementation for testing
type testElement struct {
	tag         string
	id          string
	classes     []string
	attrs       map[string]string
	parent      *testElement
	children    []*testElement
	prevSibling *testElement
}

func (e *testElement) TagName() string   { return e.tag }
func (e *testElement) ID() string        { return e.id }
func (e *testElement) Classes() []string { return e.classes }
func (e *testElement) GetAttribute(name string) (string, bool) {
	if e.attrs == nil {
		return "", false
	}
	v, ok := e.attrs[name]
	return v, ok
}
func (e *testElement) ParentElement() Element {
	if e.parent == nil {
		return nil
	}
	return e.parent
}
func (e *testElement) PreviousSiblingElement() Element {
	if e.prevSibling == nil {
		return nil
	}
	return e.prevSibling
}
func (e *testElement) ForEachChild(fn func(Element) bool) {
	for _, c := range e.children {
		if !fn(c) {
			return
		}
	}
}
func (e *testElement) ForEachAncestor(fn func(Element) bool) {
	for p := e.parent; p != nil; p = p.parent {
		if !fn(p) {
			return
		}
	}
}
func (e *testElement) ForEachPrecedingSibling(fn func(Element) bool) {
	for s := e.prevSibling; s != nil; s = s.prevSibling {
		if !fn(s) {
			return
		}
	}
}
