package css_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

func TestCSS_LayerParsingStatementsAndBlocks(t *testing.T) {
	cssText := `
@layer reset, base, framework.components;

@layer reset {
    * { margin: 0; box-sizing: border-box; }
}

@layer base {
    h1 { font-size: 2rem; }
}

@layer framework {
    @layer components {
        .card { padding: 1rem; }
    }
}

@layer {
    .utility { display: flex; }
}
`
	sheet := css.Parse(cssText)
	if len(sheet.Layers) != 3 {
		t.Fatalf("expected 3 declared layer names, got %d: %v", len(sheet.Layers), sheet.Layers)
	}
	expectedLayers := []string{"reset", "base", "framework.components"}
	for i, exp := range expectedLayers {
		if sheet.Layers[i] != exp {
			t.Errorf("expected layer %d to be '%s', got '%s'", i, exp, sheet.Layers[i])
		}
	}

	if len(sheet.Rules) != 4 {
		t.Fatalf("expected 4 rules, got %d", len(sheet.Rules))
	}
	if sheet.Rules[0].Layer != "reset" {
		t.Errorf("rule 0: expected layer 'reset', got '%s'", sheet.Rules[0].Layer)
	}
	if sheet.Rules[1].Layer != "base" {
		t.Errorf("rule 1: expected layer 'base', got '%s'", sheet.Rules[1].Layer)
	}
	if sheet.Rules[2].Layer != "framework.components" {
		t.Errorf("rule 2: expected layer 'framework.components', got '%s'", sheet.Rules[2].Layer)
	}
	if sheet.Rules[3].Layer != "anon#1" {
		t.Errorf("rule 3: expected layer 'anon#1', got '%s'", sheet.Rules[3].Layer)
	}
}
