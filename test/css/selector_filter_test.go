package css_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
)

func TestIsSelector(t *testing.T) {
	doc := dom.NewDocument()
	div := doc.NewElement("div")
	div.SetAttribute("class", "foo")
	doc.Node.AppendChild(div)

	span := doc.NewElement("span")
	span.SetAttribute("class", "bar")
	doc.Node.AppendChild(span)

	// Test :is() with multiple selectors.
	sel := css.ParseSelector(":is(.foo, .bar)")
	if !sel.Matches(div) {
		t.Error(":is(.foo, .bar) should match div.foo")
	}
	if !sel.Matches(span) {
		t.Error(":is(.foo, .bar) should match span.bar")
	}

	// Test :is() with tag names.
	sel2 := css.ParseSelector(":is(div, span)")
	if !sel2.Matches(div) {
		t.Error(":is(div, span) should match div")
	}
	if !sel2.Matches(span) {
		t.Error(":is(div, span) should match span")
	}

	// Test :is() with non-matching selectors.
	p := doc.NewElement("p")
	doc.Node.AppendChild(p)
	if sel.Matches(p) {
		t.Error(":is(.foo, .bar) should not match p")
	}
}

func TestWhereSelector(t *testing.T) {
	doc := dom.NewDocument()
	div := doc.NewElement("div")
	div.SetAttribute("class", "foo")
	doc.Node.AppendChild(div)

	// Test :where() with multiple selectors.
	sel := css.ParseSelector(":where(.foo, .bar)")
	if !sel.Matches(div) {
		t.Error(":where(.foo, .bar) should match div.foo")
	}

	// Test :where() with non-matching selectors.
	p := doc.NewElement("p")
	doc.Node.AppendChild(p)
	if sel.Matches(p) {
		t.Error(":where(.foo, .bar) should not match p")
	}
}

func TestIsSpecificity(t *testing.T) {
	// :is() should take the highest specificity of its arguments.
	sel1 := css.ParseSelector(":is(#id, .class)")
	a, b, c := sel1.Specificity()
	// #id has specificity (1, 0, 0), .class has (0, 1, 0).
	// :is() should take the max, which is (1, 0, 0).
	if a != 1 || b != 0 || c != 0 {
		t.Errorf(":is(#id, .class) specificity = (%d, %d, %d), want (1, 0, 0)", a, b, c)
	}

	sel2 := css.ParseSelector(":is(.class1, .class2)")
	a, b, c = sel2.Specificity()
	// Both are (0, 1, 0), so max is (0, 1, 0).
	if a != 0 || b != 1 || c != 0 {
		t.Errorf(":is(.class1, .class2) specificity = (%d, %d, %d), want (0, 1, 0)", a, b, c)
	}
}

func TestWhereSpecificity(t *testing.T) {
	// :where() should have zero specificity.
	sel := css.ParseSelector(":where(#id, .class)")
	a, b, c := sel.Specificity()
	if a != 0 || b != 0 || c != 0 {
		t.Errorf(":where(#id, .class) specificity = (%d, %d, %d), want (0, 0, 0)", a, b, c)
	}
}

func TestFilterParsing(t *testing.T) {
	tests := []struct {
		input    string
		expected []css.FilterFunc
	}{
		{
			input:    "none",
			expected: nil,
		},
		{
			input: "blur(5px)",
			expected: []css.FilterFunc{
				{Name: "blur", Arg: 5},
			},
		},
		{
			input: "brightness(1.5)",
			expected: []css.FilterFunc{
				{Name: "brightness", Arg: 1.5},
			},
		},
		{
			input: "contrast(200%)",
			expected: []css.FilterFunc{
				{Name: "contrast", Arg: 2.0}, // 200% = 2.0
			},
		},
		{
			input: "blur(5px) brightness(1.5)",
			expected: []css.FilterFunc{
				{Name: "blur", Arg: 5},
				{Name: "brightness", Arg: 1.5},
			},
		},
	}

	for _, tt := range tests {
		result := css.ParseFilter(tt.input)
		if len(result) != len(tt.expected) {
			t.Errorf("ParseFilter(%q) returned %d funcs, want %d", tt.input, len(result), len(tt.expected))
			continue
		}
		for i, f := range result {
			if f.Name != tt.expected[i].Name {
				t.Errorf("ParseFilter(%q)[%d].Name = %q, want %q", tt.input, i, f.Name, tt.expected[i].Name)
			}
			if f.Arg != tt.expected[i].Arg {
				t.Errorf("ParseFilter(%q)[%d].Arg = %v, want %v", tt.input, i, f.Arg, tt.expected[i].Arg)
			}
		}
	}
}

func TestSplitSelectorList(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{
			input:    ".foo, .bar",
			expected: []string{".foo", ".bar"},
		},
		{
			input:    ".foo, .bar, .baz",
			expected: []string{".foo", ".bar", ".baz"},
		},
		{
			input:    ".foo:not(.bar), .baz",
			expected: []string{".foo:not(.bar)", ".baz"},
		},
		{
			input:    "  .foo  ,  .bar  ",
			expected: []string{".foo", ".bar"},
		},
	}

	for _, tt := range tests {
		result := splitSelectorList(tt.input)
		if len(result) != len(tt.expected) {
			t.Errorf("splitSelectorList(%q) returned %d selectors, want %d", tt.input, len(result), len(tt.expected))
			continue
		}
		for i, s := range result {
			if s != tt.expected[i] {
				t.Errorf("splitSelectorList(%q)[%d] = %q, want %q", tt.input, i, s, tt.expected[i])
			}
		}
	}
}

// splitSelectorList is exported from css package for testing.
func splitSelectorList(s string) []string {
	return css.SplitSelectorList(s)
}
