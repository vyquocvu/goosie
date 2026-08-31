package css_test

import (
	"testing"
	"github.com/vyquocvu/goosie/internal/css"
)

// BenchmarkComputeSpecificity benchmarks specificity computation
func BenchmarkComputeSpecificity(b *testing.B) {
	rawCSS := `div.container > p#intro.highlight:first-child { color: red; }`
	p := css.NewParser(rawCSS)
	sheet, _ := p.Parse()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		css.ComputeSpecificity(&sheet.Rules[0].Selectors[0])
	}
}

// BenchmarkCompileStyleSheet benchmarks stylesheet compilation
func BenchmarkCompileStyleSheet(b *testing.B) {
	b.ReportAllocs()

	b.Run("Small", func(b *testing.B) {
		p := css.NewParser(smallCSS)
		sheet, _ := p.Parse()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			css.CompileStyleSheet(sheet)
		}
	})

	b.Run("Medium", func(b *testing.B) {
		p := css.NewParser(mediumCSS)
		sheet, _ := p.Parse()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			css.CompileStyleSheet(sheet)
		}
	})

	b.Run("Large", func(b *testing.B) {
		p := css.NewParser(largeCSS)
		sheet, _ := p.Parse()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			css.CompileStyleSheet(sheet)
		}
	})

	b.Run("SelectorComplex", func(b *testing.B) {
		p := css.NewParser(selectorComplexCSS)
		sheet, _ := p.Parse()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			css.CompileStyleSheet(sheet)
		}
	})

	b.Run("SelectorHeavy", func(b *testing.B) {
		p := css.NewParser(selectorHeavyCSS)
		sheet, _ := p.Parse()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			css.CompileStyleSheet(sheet)
		}
	})
}

// BenchmarkMatchElement benchmarks element matching
func BenchmarkMatchElement(b *testing.B) {
	// Build a test stylesheet with various selector types
	rawCSS := `
		h1 { color: red; }
		.container { max-width: 1200px; }
		#main { background: white; }
		* { margin: 0; }
		[type="text"] { border: 1px solid; }
		div > p { margin: 10px; }
		h1 + p { margin-top: 0; }
		ul li:first-child { font-weight: bold; }
	`
	p := css.NewParser(rawCSS)
	sheet, _ := p.Parse()
	compiled := css.CompileStyleSheet(sheet)

	b.ReportAllocs()

	b.Run("ByID", func(b *testing.B) {
		elem := &testElement{tag: "div", id: "main", classes: nil}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			compiled.MatchElement(elem)
		}
	})

	b.Run("ByClass", func(b *testing.B) {
		elem := &testElement{tag: "div", id: "", classes: []string{"container"}}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			compiled.MatchElement(elem)
		}
	})

	b.Run("ByTag", func(b *testing.B) {
		elem := &testElement{tag: "h1", id: "", classes: nil}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			compiled.MatchElement(elem)
		}
	})

	b.Run("Compound", func(b *testing.B) {
		parent := &testElement{tag: "div", id: "", classes: nil}
		elem := &testElement{tag: "p", id: "", classes: []string{"text"}, parent: parent}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			compiled.MatchElement(elem)
		}
	})

	b.Run("Descendant", func(b *testing.B) {
		grandparent := &testElement{tag: "ul", id: "", classes: nil}
		parent := &testElement{tag: "li", id: "", classes: nil, parent: grandparent}
		elem := &testElement{tag: "a", id: "", classes: nil, parent: parent}
		grandparent.children = []*testElement{parent}
		parent.children = []*testElement{elem}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			compiled.MatchElement(elem)
		}
	})

	b.Run("NoMatch", func(b *testing.B) {
		elem := &testElement{tag: "span", id: "other", classes: []string{"unknown"}}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			compiled.MatchElement(elem)
		}
	})
}

// BenchmarkMatchVsLinear compares bucketed matching vs linear scan
func BenchmarkMatchVsLinear(b *testing.B) {
	// Use the selector-heavy CSS for realistic comparison
	p := css.NewParser(selectorHeavyCSS)
	sheet, _ := p.Parse()
	compiled := css.CompileStyleSheet(sheet)

	// Build a representative element
	parent := &testElement{tag: "div", id: "page", classes: []string{"section"}}
	elem := &testElement{
		tag:     "p",
		id:      "",
		classes: []string{"card", "card-body"},
		parent:  parent,
		attrs:   map[string]string{"data-type": "article"},
	}

	b.Run("Bucketed", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			compiled.MatchElement(elem)
		}
	})

	b.Run("Linear", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			linearMatchAll(sheet.Rules, elem)
		}
	})
}

// linearMatchAll performs a linear scan of all rules (baseline for comparison)
func linearMatchAll(rules []css.Rule, elem css.Element) int {
	count := 0
	for _, rule := range rules {
		for _, seq := range rule.Selectors {
			if linearMatchSequence(&seq, elem) {
				count++
				break
			}
		}
	}
	return count
}

// linearMatchSequence matches using the old linked-list approach
func linearMatchSequence(seq *css.SelectorSequence, elem css.Element) bool {
	return linearMatchFromRight(seq, elem)
}

func linearMatchFromRight(seq *css.SelectorSequence, elem css.Element) bool {
	if seq.Next == nil {
		return linearMatchSimple(&seq.Simple, elem)
	}
	return linearMatchWithCombinator(seq, elem)
}

func linearMatchWithCombinator(seq *css.SelectorSequence, elem css.Element) bool {
	if !linearMatchFromRight(seq.Next, elem) {
		return false
	}

	switch seq.Combinator {
	case " ":
		var found bool
		elem.ForEachAncestor(func(ancestor css.Element) bool {
			if linearMatchSimple(&seq.Simple, ancestor) {
				found = true
				return false
			}
			return true
		})
		return found
	case ">":
		parent := elem.ParentElement()
		if parent == nil {
			return false
		}
		return linearMatchSimple(&seq.Simple, parent)
	case "+":
		sibling := elem.PreviousSiblingElement()
		if sibling == nil {
			return false
		}
		return linearMatchSimple(&seq.Simple, sibling)
	case "~":
		var found bool
		elem.ForEachPrecedingSibling(func(sibling css.Element) bool {
			if linearMatchSimple(&seq.Simple, sibling) {
				found = true
				return false
			}
			return true
		})
		return found
	}
	return false
}

func linearMatchSimple(sel *css.SimpleSelector, elem css.Element) bool {
	if sel.Universal && sel.TagName == "" && sel.ID == "" &&
		len(sel.Classes) == 0 && len(sel.PseudoClasses) == 0 &&
		len(sel.Attributes) == 0 && len(sel.PseudoElements) == 0 {
		return true
	}

	if sel.TagName != "" {
		if !equalFold(sel.TagName, elem.TagName()) {
			return false
		}
	}

	if sel.ID != "" {
		if elem.ID() != sel.ID {
			return false
		}
	}

	if len(sel.Classes) > 0 {
		elemClasses := elem.Classes()
		for _, reqClass := range sel.Classes {
			found := false
			for _, c := range elemClasses {
				if c == reqClass {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}

	for _, attr := range sel.Attributes {
		value, exists := elem.GetAttribute(attr.Name)
		if !exists {
			return false
		}
		if attr.Operator == "" {
			continue
		}
		switch attr.Operator {
		case "=":
			if value != attr.Value {
				return false
			}
		case "^=":
			if len(value) < len(attr.Value) || value[:len(attr.Value)] != attr.Value {
				return false
			}
		default:
			// Simplified for benchmark
		}
	}

	return true
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		// Case-insensitive comparison
		return len(a) == len(b) && equalFoldSlow(a, b)
	}
	return equalFoldSlow(a, b)
}

func equalFoldSlow(a, b string) bool {
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
