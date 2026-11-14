package css_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

func TestParseContainerRule_Basic(t *testing.T) {
	input := `
		.container {
			container-type: inline-size;
			container-name: sidebar;
		}
		@container (min-width: 400px) {
			.card {
				color: red;
			}
		}
		@container sidebar (max-width: 600px) {
			.item {
				color: blue;
			}
		}
	`
	sheet := css.Parse(input)
	if len(sheet.Rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(sheet.Rules))
	}

	if sheet.Rules[0].ContainerQuery != "" {
		t.Errorf("rule 0 should have no container query, got %q", sheet.Rules[0].ContainerQuery)
	}

	if sheet.Rules[1].ContainerQuery != "(min-width: 400px)" {
		t.Errorf("rule 1 expected container query '(min-width: 400px)', got %q", sheet.Rules[1].ContainerQuery)
	}

	if sheet.Rules[2].ContainerQuery != "sidebar (max-width: 600px)" {
		t.Errorf("rule 2 expected container query 'sidebar (max-width: 600px)', got %q", sheet.Rules[2].ContainerQuery)
	}
}

func TestParseContainerRule_Nested(t *testing.T) {
	input := `
		@container outer (min-width: 500px) {
			@container inner (max-width: 300px) {
				.nested {
					display: flex;
				}
			}
		}
	`
	sheet := css.Parse(input)
	if len(sheet.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(sheet.Rules))
	}
	expected := "outer (min-width: 500px) and inner (max-width: 300px)"
	if sheet.Rules[0].ContainerQuery != expected {
		t.Errorf("expected combined container query %q, got %q", expected, sheet.Rules[0].ContainerQuery)
	}
}

func TestContainerRelativeUnits(t *testing.T) {
	cqW := float32(400)
	cqH := float32(200)

	tests := []struct {
		input    string
		expected float32
	}{
		{"10cqw", 40},
		{"10cqh", 20},
		{"50cqi", 200},
		{"50cqb", 100},
		{"10cqmin", 20},
		{"10cqmax", 40},
	}

	for _, tc := range tests {
		val := css.ParseValue(tc.input)
		got := val.ToLengthWithContainerContext(16, 16, 1440, 900, cqW, cqH)
		if got != tc.expected {
			t.Errorf("ToLengthWithContainerContext(%q) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}

func TestResolveContainerUnits(t *testing.T) {
	cqW := float32(500)
	cqH := float32(250)

	expr := "calc(10cqw + 20px)"
	resolved, ok := css.ResolveContainerUnits(expr, cqW, cqH)
	if !ok {
		t.Fatalf("expected ResolveContainerUnits to succeed")
	}
	expected := "calc(50px + 20px)"
	if resolved != expected {
		t.Errorf("ResolveContainerUnits(%q) = %q, want %q", expr, resolved, expected)
	}
}
