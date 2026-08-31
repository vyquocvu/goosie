package css_test

import (
	"strings"
	"testing"
	"github.com/vyquocvu/goosie/internal/css"
)

func TestParserAcceptsUTF8BOM(t *testing.T) {
	sheet, err := css.NewParser("\ufeff.hidden { display: none; }").Parse()
	if err != nil {
		t.Fatalf("Parse() returned error for UTF-8 BOM: %v", err)
	}
	if len(sheet.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(sheet.Rules))
	}
}

func TestParserPreservesNestedMediaRules(t *testing.T) {
	sheet, err := css.NewParser(`
		@media screen {
			.base { display: block; }
			@media (max-width: 600px) {
				.narrow { display: block; }
			}
		}`).Parse()
	if err != nil {
		t.Fatalf("Parse() returned error for nested media: %v", err)
	}
	if len(sheet.AtRules) != 1 || len(sheet.AtRules[0].AtRules) != 1 {
		t.Fatalf("expected one nested at-rule, got %#v", sheet.AtRules)
	}
}

func TestParser(t *testing.T) {
	cssText := `
		h1 {
			display: block;
			font-size: 32px;
		}
	`
	p := css.NewParser(cssText)
	stylesheet, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}
	if len(stylesheet.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(stylesheet.Rules))
	}
	rule := stylesheet.Rules[0]
	if len(rule.Selectors) != 1 {
		t.Fatalf("expected 1 selector, got %d", len(rule.Selectors))
	}
	selector := rule.Selectors[0].Simple
	if selector.TagName != "h1" {
		t.Errorf("expected tag name 'h1', got '%s'", selector.TagName)
	}
	if len(rule.Declarations) != 2 {
		t.Fatalf("expected 2 declarations, got %d", len(rule.Declarations))
	}
	decl1 := rule.Declarations[0]
	if decl1.Property != "display" || decl1.Value != "block" {
		t.Errorf("unexpected declaration: %s: %s", decl1.Property, decl1.Value)
	}
	decl2 := rule.Declarations[1]
	if decl2.Property != "font-size" || decl2.Value != "32px" {
		t.Errorf("unexpected declaration: %s: %s", decl2.Property, decl2.Value)
	}
}

func TestParserKeyframes(t *testing.T) {
	cssText := `
@keyframes my-animation {
	from { opacity: 0; }
	50% { opacity: 0.5; }
	to { opacity: 1; }
}
.class { color: red; }
`
	p := css.NewParser(cssText)
	stylesheet, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}
	if len(stylesheet.AtRules) != 1 {
		t.Fatalf("expected 1 at-rule, got %d", len(stylesheet.AtRules))
	}
	if len(stylesheet.Rules) != 1 {
		t.Fatalf("expected 1 rule (after keyframes), got %d", len(stylesheet.Rules))
	}

	kf := stylesheet.AtRules[0]
	if kf.Name != "keyframes" {
		t.Errorf("expected keyframes, got %s", kf.Name)
	}
	// We expect 3 rules inside keyframes: from, 50%, to
	if len(kf.Rules) != 3 {
		t.Errorf("expected 3 keyframe rules, got %d", len(kf.Rules))
	}
	// Check first rule selector (from)
	if len(kf.Rules[0].Selectors) > 0 {
		if kf.Rules[0].Selectors[0].Simple.TagName != "from" {
			t.Errorf("expected 'from', got '%s'", kf.Rules[0].Selectors[0].Simple.TagName)
		}
	}
}

func TestParserEscapedIdentifier(t *testing.T) {
	cssText := `.w-3\/4 { width: 75%; }`
	p := css.NewParser(cssText)
	stylesheet, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}
	if len(stylesheet.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(stylesheet.Rules))
	}
	rule := stylesheet.Rules[0]
	if len(rule.Selectors) != 1 {
		t.Fatalf("expected 1 selector, got %d", len(rule.Selectors))
	}
	if len(rule.Selectors[0].Simple.Classes) != 1 {
		t.Fatalf("expected 1 class, got %d", len(rule.Selectors[0].Simple.Classes))
	}
	className := rule.Selectors[0].Simple.Classes[0]
	if className != "w-3/4" {
		t.Errorf("expected class 'w-3/4', got '%s'", className)
	}
}

func TestParserCombinedSelector(t *testing.T) {
	cssText := `
		h1.title {
			color: blue;
		}
	`
	p := css.NewParser(cssText)
	stylesheet, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}
	if len(stylesheet.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(stylesheet.Rules))
	}
	rule := stylesheet.Rules[0]
	if len(rule.Selectors) != 1 {
		t.Fatalf("expected 1 selector, got %d", len(rule.Selectors))
	}
	selector := rule.Selectors[0].Simple
	if selector.TagName != "h1" {
		t.Errorf("expected tag name 'h1', got '%s'", selector.TagName)
	}
	if len(selector.Classes) != 1 || selector.Classes[0] != "title" {
		t.Errorf("expected class 'title', got %v", selector.Classes)
	}
	if len(rule.Declarations) != 1 {
		t.Fatalf("expected 1 declaration, got %d", len(rule.Declarations))
	}
	decl := rule.Declarations[0]
	if decl.Property != "color" || decl.Value != "blue" {
		t.Errorf("unexpected declaration: %s: %s", decl.Property, decl.Value)
	}
}

func TestParserComments(t *testing.T) {
	cssText := `
		/* This is a comment */
		h1 {
			color: red; /* inline comment */
		}
		/* Another comment */
	`
	p := css.NewParser(cssText)
	stylesheet, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}
	if len(stylesheet.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(stylesheet.Rules))
	}
}

func TestParserDescendantSelector(t *testing.T) {
	cssText := `
		div p {
			color: blue;
		}
	`
	p := css.NewParser(cssText)
	stylesheet, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}
	if len(stylesheet.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(stylesheet.Rules))
	}
	rule := stylesheet.Rules[0]
	if len(rule.Selectors) != 1 {
		t.Fatalf("expected 1 selector, got %d", len(rule.Selectors))
	}
	seq := rule.Selectors[0]
	// Selector is stored left-to-right: div (combinator: " ") -> p
	if seq.Simple.TagName != "div" {
		t.Errorf("expected leftmost selector to be 'div', got '%s'", seq.Simple.TagName)
	}
	if seq.Combinator != " " {
		t.Errorf("expected descendant combinator, got '%s'", seq.Combinator)
	}
	if seq.Next == nil || seq.Next.Simple.TagName != "p" {
		t.Errorf("expected rightmost selector to be 'p'")
	}
}

func TestParserChildSelector(t *testing.T) {
	cssText := `
		div > p {
			margin: 10px;
		}
	`
	p := css.NewParser(cssText)
	stylesheet, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}
	if len(stylesheet.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(stylesheet.Rules))
	}
	rule := stylesheet.Rules[0]
	seq := rule.Selectors[0]
	if seq.Combinator != ">" {
		t.Errorf("expected child combinator, got '%s'", seq.Combinator)
	}
}

func TestParserAdjacentSiblingSelector(t *testing.T) {
	cssText := `
		h1 + p {
			font-weight: bold;
		}
	`
	p := css.NewParser(cssText)
	stylesheet, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}
	if len(stylesheet.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(stylesheet.Rules))
	}
	rule := stylesheet.Rules[0]
	seq := rule.Selectors[0]
	if seq.Combinator != "+" {
		t.Errorf("expected adjacent sibling combinator, got '%s'", seq.Combinator)
	}
}

func TestParserGeneralSiblingSelector(t *testing.T) {
	cssText := `
		h1 ~ p {
			color: gray;
		}
	`
	p := css.NewParser(cssText)
	stylesheet, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}
	if len(stylesheet.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(stylesheet.Rules))
	}
	rule := stylesheet.Rules[0]
	seq := rule.Selectors[0]
	if seq.Combinator != "~" {
		t.Errorf("expected general sibling combinator, got '%s'", seq.Combinator)
	}
}

func TestParserAttributeSelector(t *testing.T) {
	tests := []struct {
		name     string
		css      string
		attrName string
		operator string
		value    string
	}{
		{
			name:     "attribute exists",
			css:      `[disabled] { opacity: 0.5; }`,
			attrName: "disabled",
			operator: "",
			value:    "",
		},
		{
			name:     "attribute equals",
			css:      `[type="text"] { border: 1px solid; }`,
			attrName: "type",
			operator: "=",
			value:    "text",
		},
		{
			name:     "attribute contains word",
			css:      `[class~="active"] { color: red; }`,
			attrName: "class",
			operator: "~=",
			value:    "active",
		},
		{
			name:     "attribute starts with",
			css:      `[href^="https"] { color: green; }`,
			attrName: "href",
			operator: "^=",
			value:    "https",
		},
		{
			name:     "attribute ends with",
			css:      `[src$=".png"] { display: block; }`,
			attrName: "src",
			operator: "$=",
			value:    ".png",
		},
		{
			name:     "attribute contains",
			css:      `[title*="example"] { font-weight: bold; }`,
			attrName: "title",
			operator: "*=",
			value:    "example",
		},
		{
			name:     "attribute starts with https combined",
			css:      `a[href^="https"] { font-weight: bold; }`,
			attrName: "href",
			operator: "^=",
			value:    "https",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := css.NewParser(tt.css)
			stylesheet, err := p.Parse()
			if err != nil {
				t.Fatalf("Parse() failed: %v", err)
			}
			if len(stylesheet.Rules) != 1 {
				t.Fatalf("expected 1 rule, got %d", len(stylesheet.Rules))
			}
			rule := stylesheet.Rules[0]
			selector := rule.Selectors[0].Simple
			if len(selector.Attributes) != 1 {
				t.Fatalf("expected 1 attribute selector, got %d", len(selector.Attributes))
			}
			attr := selector.Attributes[0]
			if attr.Name != tt.attrName {
				t.Errorf("expected attribute name '%s', got '%s'", tt.attrName, attr.Name)
			}
			if attr.Operator != tt.operator {
				t.Errorf("expected operator '%s', got '%s'", tt.operator, attr.Operator)
			}
			if attr.Value != tt.value {
				t.Errorf("expected value '%s', got '%s'", tt.value, attr.Value)
			}
		})
	}
}

func TestParserUniversalSelector(t *testing.T) {
	cssText := `
		* {
			margin: 0;
			padding: 0;
		}
	`
	p := css.NewParser(cssText)
	stylesheet, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}
	if len(stylesheet.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(stylesheet.Rules))
	}
	rule := stylesheet.Rules[0]
	selector := rule.Selectors[0].Simple
	if !selector.Universal {
		t.Errorf("expected universal selector")
	}
}

func TestParserPseudoClasses(t *testing.T) {
	tests := []struct {
		name        string
		css         string
		pseudoClass string
	}{
		{
			name:        "hover",
			css:         `a:hover { color: red; }`,
			pseudoClass: "hover",
		},
		{
			name:        "first-child",
			css:         `li:first-child { font-weight: bold; }`,
			pseudoClass: "first-child",
		},
		{
			name:        "nth-child",
			css:         `tr:nth-child(2) { background: gray; }`,
			pseudoClass: "nth-child(2)",
		},
		{
			name:        "nth-child-formula",
			css:         `li:nth-child(2n+1) { background: red; }`,
			pseudoClass: "nth-child(2n+1)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := css.NewParser(tt.css)
			stylesheet, err := p.Parse()
			if err != nil {
				t.Fatalf("Parse() failed: %v", err)
			}
			if len(stylesheet.Rules) != 1 {
				t.Fatalf("expected 1 rule, got %d", len(stylesheet.Rules))
			}
			rule := stylesheet.Rules[0]
			selector := rule.Selectors[0].Simple
			if len(selector.PseudoClasses) != 1 {
				t.Fatalf("expected 1 pseudo-class, got %d", len(selector.PseudoClasses))
			}
			if selector.PseudoClasses[0] != tt.pseudoClass {
				t.Errorf("expected pseudo-class '%s', got '%s'", tt.pseudoClass, selector.PseudoClasses[0])
			}
		})
	}
}

func TestParserPseudoElements(t *testing.T) {
	cssText := `
		p::before {
			content: ">";
		}
		a::after {
			content: attr(href);
		}
	`
	p := css.NewParser(cssText)
	stylesheet, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}
	if len(stylesheet.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(stylesheet.Rules))
	}
	rule := stylesheet.Rules[0]
	selector := rule.Selectors[0].Simple
	if len(selector.PseudoElements) != 1 {
		t.Fatalf("expected 1 pseudo-element, got %d", len(selector.PseudoElements))
	}
	if selector.PseudoElements[0] != "before" {
		t.Errorf("expected pseudo-element 'before', got '%s'", selector.PseudoElements[0])
	}

	rule2 := stylesheet.Rules[1]
	selector2 := rule2.Selectors[0].Simple
	if len(selector2.PseudoElements) != 1 {
		t.Fatalf("expected 1 pseudo-element on second rule, got %d", len(selector2.PseudoElements))
	}
	if selector2.PseudoElements[0] != "after" {
		t.Errorf("expected pseudo-element 'after', got '%s'", selector2.PseudoElements[0])
	}
	if len(rule2.Declarations) != 1 || rule2.Declarations[0].Property != "content" || rule2.Declarations[0].Value != "attr(href)" {
		t.Errorf("expected content: attr(href), got %#v", rule2.Declarations)
	}
}

func TestParserMultipleSelectors(t *testing.T) {
	cssText := `
		h1, h2, h3 {
			font-family: Arial;
		}
	`
	p := css.NewParser(cssText)
	stylesheet, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}
	if len(stylesheet.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(stylesheet.Rules))
	}
	rule := stylesheet.Rules[0]
	if len(rule.Selectors) != 3 {
		t.Fatalf("expected 3 selectors, got %d", len(rule.Selectors))
	}
	tags := []string{"h1", "h2", "h3"}
	for i, tag := range tags {
		if rule.Selectors[i].Simple.TagName != tag {
			t.Errorf("expected selector %d to be '%s', got '%s'", i, tag, rule.Selectors[i].Simple.TagName)
		}
	}
}

func TestParserImportant(t *testing.T) {
	cssText := `
		p {
			color: red !important;
			font-size: 16px;
		}
	`
	p := css.NewParser(cssText)
	stylesheet, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}
	if len(stylesheet.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(stylesheet.Rules))
	}
	rule := stylesheet.Rules[0]
	if len(rule.Declarations) != 2 {
		t.Fatalf("expected 2 declarations, got %d", len(rule.Declarations))
	}
	if !rule.Declarations[0].Important {
		t.Errorf("expected first declaration to be important")
	}
	if rule.Declarations[0].Value != "red" {
		t.Errorf("expected value 'red', got '%s'", rule.Declarations[0].Value)
	}
	if rule.Declarations[1].Important {
		t.Errorf("expected second declaration to not be important")
	}
}

func TestParserAtMedia(t *testing.T) {
	cssText := `
		@media screen and (max-width: 600px) {
			body {
				font-size: 14px;
			}
		}
	`
	p := css.NewParser(cssText)
	stylesheet, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}
	if len(stylesheet.AtRules) != 1 {
		t.Fatalf("expected 1 at-rule, got %d", len(stylesheet.AtRules))
	}
	atRule := stylesheet.AtRules[0]
	if atRule.Name != "media" {
		t.Errorf("expected at-rule name 'media', got '%s'", atRule.Name)
	}
	if !strings.Contains(atRule.Prelude, "screen") {
		t.Errorf("expected prelude to contain 'screen', got '%s'", atRule.Prelude)
	}
	if len(atRule.Rules) != 1 {
		t.Fatalf("expected 1 nested rule, got %d", len(atRule.Rules))
	}
}

func TestParserComplexSelector(t *testing.T) {
	cssText := `
		div.container > p#intro.highlight:first-child {
			color: blue;
		}
	`
	p := css.NewParser(cssText)
	stylesheet, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}
	if len(stylesheet.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(stylesheet.Rules))
	}
	rule := stylesheet.Rules[0]
	seq := rule.Selectors[0]

	// Check leftmost selector (div.container)
	if seq.Simple.TagName != "div" {
		t.Errorf("expected tag 'div', got '%s'", seq.Simple.TagName)
	}
	if len(seq.Simple.Classes) != 1 || seq.Simple.Classes[0] != "container" {
		t.Errorf("expected class 'container', got %v", seq.Simple.Classes)
	}

	// Check combinator
	if seq.Combinator != ">" {
		t.Errorf("expected child combinator, got '%s'", seq.Combinator)
	}

	// Check rightmost selector (p#intro.highlight:first-child)
	if seq.Next == nil {
		t.Fatal("expected next selector")
	}
	if seq.Next.Simple.TagName != "p" {
		t.Errorf("expected next tag 'p', got '%s'", seq.Next.Simple.TagName)
	}
	if seq.Next.Simple.ID != "intro" {
		t.Errorf("expected ID 'intro', got '%s'", seq.Next.Simple.ID)
	}
	if len(seq.Next.Simple.Classes) != 1 || seq.Next.Simple.Classes[0] != "highlight" {
		t.Errorf("expected class 'highlight', got %v", seq.Next.Simple.Classes)
	}
	if len(seq.Next.Simple.PseudoClasses) != 1 || seq.Next.Simple.PseudoClasses[0] != "first-child" {
		t.Errorf("expected pseudo-class 'first-child', got %v", seq.Next.Simple.PseudoClasses)
	}
}

func TestParserValueWithFunction(t *testing.T) {
	cssText := `
		div {
			background: url("image.png");
			width: calc(100% - 20px);
		}
	`
	p := css.NewParser(cssText)
	stylesheet, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}
	if len(stylesheet.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(stylesheet.Rules))
	}
	rule := stylesheet.Rules[0]
	if len(rule.Declarations) != 2 {
		t.Fatalf("expected 2 declarations, got %d", len(rule.Declarations))
	}

	// Check url() function
	if !strings.Contains(rule.Declarations[0].Value, "url(") {
		t.Errorf("expected value to contain 'url(', got '%s'", rule.Declarations[0].Value)
	}

	// Check calc() function
	if !strings.Contains(rule.Declarations[1].Value, "calc(") {
		t.Errorf("expected value to contain 'calc(', got '%s'", rule.Declarations[1].Value)
	}
}

func TestParserMalformedCSS(t *testing.T) {
	tests := []struct {
		name string
		css  string
	}{
		{
			name: "missing property value",
			css:  `p { color: }`,
		},
		{
			name: "missing colon",
			css:  `p { color red; }`,
		},
		{
			name: "unclosed comment at end",
			css:  `p { color: red; } /* unclosed comment`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := css.NewParser(tt.css)
			stylesheet, err := p.Parse()
			// Parser should not panic
			// Some malformed CSS may return errors, others may skip invalid parts
			if err != nil {
				t.Logf("Parse returned error (expected for some malformed CSS): %v", err)
			} else if stylesheet != nil {
				t.Logf("Parse succeeded, stylesheet has %d rules", len(stylesheet.Rules))
			}
		})
	}
}
