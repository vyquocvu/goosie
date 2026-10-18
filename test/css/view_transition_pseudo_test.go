package css_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

func TestParameterizedPseudoElement(t *testing.T) {
	sheet := css.Parse(`::view-transition-group(header) { animation-duration: 0.5s; }`)
	if len(sheet.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(sheet.Rules))
	}
	rule := sheet.Rules[0]
	if len(rule.Selectors) != 1 {
		t.Fatalf("expected 1 selector, got %d", len(rule.Selectors))
	}
	sel := rule.Selectors[0]
	if len(sel.Parts) == 0 {
		t.Fatal("expected selector parts")
	}
	part := sel.Parts[len(sel.Parts)-1]
	found := false
	for _, c := range part.Conditions {
		if c.Type == css.CondPseudoElement {
			found = true
			if c.Value != "view-transition-group" {
				t.Errorf("pseudo name = %q, want view-transition-group", c.Value)
			}
			if c.Pseudo != "header" {
				t.Errorf("pseudo arg = %q, want header", c.Pseudo)
			}
		}
	}
	if !found {
		t.Error("CondPseudoElement not found in selector conditions")
	}
}
