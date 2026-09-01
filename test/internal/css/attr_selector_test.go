package css_test

import (
	"testing"
	"github.com/vyquocvu/goosie/internal/css"
)

type customTestElem struct {
	tagName string
	id      string
	classes []string
	attrs   map[string]string
}

func (e *customTestElem) TagName() string { return e.tagName }
func (e *customTestElem) ID() string      { return e.id }
func (e *customTestElem) Classes() []string { return e.classes }
func (e *customTestElem) GetAttribute(name string) (string, bool) {
	if e.attrs == nil {
		return "", false
	}
	v, ok := e.attrs[name]
	return v, ok
}
func (e *customTestElem) ParentElement() css.Element              { return nil }
func (e *customTestElem) PreviousSiblingElement() css.Element      { return nil }
func (e *customTestElem) ForEachChild(fn func(css.Element) bool)   {}
func (e *customTestElem) ForEachAncestor(fn func(css.Element) bool) {}
func (e *customTestElem) ForEachPrecedingSibling(fn func(css.Element) bool) {}

func TestDynamicAttributeSelectorCandidateCollection(t *testing.T) {
	cssText := `
		[name="q"] { color: blue; }
		[role="button"] { cursor: pointer; }
		[aria-label^="Close"] { display: none; }
		[data-custom-attribute="value123"] { font-weight: bold; }
	`
	parser := css.NewParser(cssText)
	sheet, err := parser.Parse()
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	compiled := css.CompileStyleSheet(sheet)

	// Test 1: input with name="q"
	inputElem := &customTestElem{
		tagName: "input",
		attrs:   map[string]string{"name": "q", "type": "text"},
	}
	matched1 := compiled.MatchElement(inputElem)
	if len(matched1) != 1 {
		t.Fatalf("Expected 1 match for [name='q'], got %d", len(matched1))
	}
	if matched1[0].Declarations[0].Value != "blue" {
		t.Errorf("Expected blue, got %s", matched1[0].Declarations[0].Value)
	}

	// Test 2: div with role="button"
	roleElem := &customTestElem{
		tagName: "div",
		attrs:   map[string]string{"role": "button"},
	}
	matched2 := compiled.MatchElement(roleElem)
	if len(matched2) != 1 {
		t.Fatalf("Expected 1 match for [role='button'], got %d", len(matched2))
	}
	if matched2[0].Declarations[0].Value != "pointer" {
		t.Errorf("Expected pointer, got %s", matched2[0].Declarations[0].Value)
	}

	// Test 3: button with aria-label="Close modal"
	ariaElem := &customTestElem{
		tagName: "button",
		attrs:   map[string]string{"aria-label": "Close modal"},
	}
	matched3 := compiled.MatchElement(ariaElem)
	if len(matched3) != 1 {
		t.Fatalf("Expected 1 match for [aria-label^='Close'], got %d", len(matched3))
	}

	// Test 4: custom data attribute
	dataElem := &customTestElem{
		tagName: "span",
		attrs:   map[string]string{"data-custom-attribute": "value123"},
	}
	matched4 := compiled.MatchElement(dataElem)
	if len(matched4) != 1 {
		t.Fatalf("Expected 1 match for data attribute, got %d", len(matched4))
	}
}
