package css_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

func TestNesting_AmpersandExpansion(t *testing.T) {
	sheet := css.Parse(`
		.card {
			padding: 20px;
			& .title {
				font-size: 18px;
			}
			&:hover {
				background: #eee;
			}
			&.active {
				border: 1px solid blue;
			}
			& + & {
				margin-top: 10px;
			}
			body.dark & {
				background: #333;
			}
		}
	`)

	if len(sheet.Rules) != 6 {
		t.Fatalf("expected 6 rules, got %d", len(sheet.Rules))
	}

	// 1. .card
	if sheet.Rules[0].SelectorStrs[0] != ".card" {
		t.Errorf("rule 0 = %q, want .card", sheet.Rules[0].SelectorStrs[0])
	}
	if len(sheet.Rules[0].Declarations) != 1 || sheet.Rules[0].Declarations[0].Property != "padding" {
		t.Errorf("rule 0 decls = %v", sheet.Rules[0].Declarations)
	}

	// 2. .card .title
	if sheet.Rules[1].SelectorStrs[0] != ".card .title" {
		t.Errorf("rule 1 = %q, want '.card .title'", sheet.Rules[1].SelectorStrs[0])
	}
	if len(sheet.Rules[1].Declarations) != 1 || sheet.Rules[1].Declarations[0].Property != "font-size" {
		t.Errorf("rule 1 decls = %v", sheet.Rules[1].Declarations)
	}

	// 3. .card:hover
	if sheet.Rules[2].SelectorStrs[0] != ".card:hover" {
		t.Errorf("rule 2 = %q, want '.card:hover'", sheet.Rules[2].SelectorStrs[0])
	}

	// 4. .card.active
	if sheet.Rules[3].SelectorStrs[0] != ".card.active" {
		t.Errorf("rule 3 = %q, want '.card.active'", sheet.Rules[3].SelectorStrs[0])
	}

	// 5. .card + .card
	if sheet.Rules[4].SelectorStrs[0] != ".card + .card" {
		t.Errorf("rule 4 = %q, want '.card + .card'", sheet.Rules[4].SelectorStrs[0])
	}

	// 6. body.dark .card
	if sheet.Rules[5].SelectorStrs[0] != "body.dark .card" {
		t.Errorf("rule 5 = %q, want 'body.dark .card'", sheet.Rules[5].SelectorStrs[0])
	}
}

func TestNesting_MultipleParentSelectors(t *testing.T) {
	sheet := css.Parse(`
		h1, h2 {
			color: black;
			& .sub, & span {
				color: gray;
			}
		}
	`)

	if len(sheet.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(sheet.Rules))
	}

	// Rule 0: h1, h2
	if len(sheet.Rules[0].SelectorStrs) != 2 {
		t.Fatalf("rule 0 selectors count = %d, want 2", len(sheet.Rules[0].SelectorStrs))
	}

	// Rule 1: h1 .sub, h2 .sub, h1 span, h2 span
	sels := sheet.Rules[1].SelectorStrs
	want := []string{"h1 .sub", "h2 .sub", "h1 span", "h2 span"}
	if len(sels) != len(want) {
		t.Fatalf("rule 1 selectors = %v, want %v", sels, want)
	}
	for i := range want {
		if sels[i] != want[i] {
			t.Errorf("sel[%d] = %q, want %q", i, sels[i], want[i])
		}
	}
}

func TestNesting_RelaxedSyntaxWithoutAmpersand(t *testing.T) {
	sheet := css.Parse(`
		.card {
			color: black;
			.title {
				font-weight: bold;
			}
			> p {
				line-height: 1.5;
			}
			:hover {
				color: red;
			}
		}
	`)

	if len(sheet.Rules) != 4 {
		t.Fatalf("expected 4 rules, got %d", len(sheet.Rules))
	}

	if sheet.Rules[1].SelectorStrs[0] != ".card .title" {
		t.Errorf("rule 1 = %q, want '.card .title'", sheet.Rules[1].SelectorStrs[0])
	}
	if sheet.Rules[2].SelectorStrs[0] != ".card > p" {
		t.Errorf("rule 2 = %q, want '.card > p'", sheet.Rules[2].SelectorStrs[0])
	}
	if sheet.Rules[3].SelectorStrs[0] != ".card:hover" {
		t.Errorf("rule 3 = %q, want '.card:hover'", sheet.Rules[3].SelectorStrs[0])
	}
}

func TestNesting_NestedAtRules(t *testing.T) {
	sheet := css.ParseWithOptions(`
		.card {
			width: 100%;
			@media (min-width: 600px) {
				width: 50%;
				& .inner {
					padding: 10px;
				}
			}
			@container (min-width: 400px) {
				padding: 20px;
			}
		}
	`, css.MediaOptions{ViewportWidth: 800})

	// Rule 0: .card (width: 100%)
	// Rule 1: .card (width: 50%) with media width >= 600
	// Rule 2: .card .inner (padding: 10px) with media width >= 600
	// Rule 3: .card (padding: 20px) with container query
	if len(sheet.Rules) != 4 {
		t.Fatalf("expected 4 rules, got %d", len(sheet.Rules))
	}

	if sheet.Rules[1].SelectorStrs[0] != ".card" {
		t.Errorf("rule 1 selector = %q, want .card", sheet.Rules[1].SelectorStrs[0])
	}
	if sheet.Rules[2].SelectorStrs[0] != ".card .inner" {
		t.Errorf("rule 2 selector = %q, want '.card .inner'", sheet.Rules[2].SelectorStrs[0])
	}
	if sheet.Rules[3].ContainerQuery != "(min-width: 400px)" {
		t.Errorf("rule 3 container query = %q, want '(min-width: 400px)'", sheet.Rules[3].ContainerQuery)
	}
}

func TestScopeRule_Parsing(t *testing.T) {
	sheet := css.Parse(`
		@scope (.card) {
			p { color: blue; }
			:scope { border: 1px solid black; }
		}
		@scope (.article) to (.comments) {
			p { color: gray; }
		}
	`)

	if len(sheet.Rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(sheet.Rules))
	}

	// Rule 0: p within .card
	if sheet.Rules[0].ScopeRoot != ".card" || sheet.Rules[0].ScopeLimit != "" {
		t.Errorf("rule 0 scope = root:%q limit:%q", sheet.Rules[0].ScopeRoot, sheet.Rules[0].ScopeLimit)
	}

	// Rule 1: :scope within .card
	if sheet.Rules[1].ScopeRoot != ".card" {
		t.Errorf("rule 1 scopeRoot = %q", sheet.Rules[1].ScopeRoot)
	}

	// Rule 2: p within .article to .comments
	if sheet.Rules[2].ScopeRoot != ".article" || sheet.Rules[2].ScopeLimit != ".comments" {
		t.Errorf("rule 2 scope = root:%q limit:%q", sheet.Rules[2].ScopeRoot, sheet.Rules[2].ScopeLimit)
	}
}
