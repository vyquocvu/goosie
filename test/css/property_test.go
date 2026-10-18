package css_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

func TestPropertyRule_SyntaxValidation(t *testing.T) {
	validSyntaxes := []string{
		"'<color>'",
		"\"<length>\"",
		"<percentage>",
		"<length-percentage>",
		"<number>",
		"<integer>",
		"<angle>",
		"<time>",
		"<resolution>",
		"<url>",
		"<image>",
		"<custom-ident>",
		"*",
		"<length> | <percentage>",
		"<color> | none",
		"small | medium | large",
		"<length>+",
		"<color>#",
	}

	for _, s := range validSyntaxes {
		if err := css.ValidatePropertySyntax(s); err != nil {
			t.Errorf("ValidatePropertySyntax(%q) failed: %v, want valid", s, err)
		}
	}

	invalidSyntaxes := []string{
		"",
		" ",
		"<unknown-type>",
		"<color> |",
		"| <length>",
		"inherit",
		"initial",
		"123bad",
	}

	for _, s := range invalidSyntaxes {
		if err := css.ValidatePropertySyntax(s); err == nil {
			t.Errorf("ValidatePropertySyntax(%q) succeeded, want error", s)
		}
	}
}

func TestPropertyRule_MatchesSyntax(t *testing.T) {
	tests := []struct {
		syntax string
		val    string
		want   bool
	}{
		{"<color>", "red", true},
		{"<color>", "#112233", true},
		{"<color>", "not-a-color", false},
		{"<length>", "10px", true},
		{"<length>", "2.5rem", true},
		{"<length>", "0", true},
		{"<length>", "calc(10px + 20px)", true},
		{"<length>", "10%", false},
		{"<percentage>", "50%", true},
		{"<percentage>", "10px", false},
		{"<number>", "42", true},
		{"<number>", "3.14", true},
		{"<number>", "abc", false},
		{"<integer>", "42", true},
		{"<integer>", "3.14", false},
		{"<angle>", "90deg", true},
		{"<angle>", "1.5rad", true},
		{"<angle>", "10px", false},
		{"*", "anything here", true},
		{"<length> | <percentage>", "20px", true},
		{"<length> | <percentage>", "20%", true},
		{"<length> | <percentage>", "red", false},
		{"small | large", "small", true},
		{"small | large", "LARGE", true},
		{"small | large", "medium", false},
		{"<length>+", "10px 20px 30px", true},
		{"<length>+", "10px red", false},
		{"<color>#", "red, green, blue", true},
		{"<color>#", "red, 10px", false},
	}

	for _, tt := range tests {
		got := css.MatchesPropertySyntax(tt.syntax, tt.val)
		if got != tt.want {
			t.Errorf("MatchesPropertySyntax(%q, %q) = %v, want %v", tt.syntax, tt.val, got, tt.want)
		}
	}
}

func TestPropertyRule_ParseValid(t *testing.T) {
	src := `
	@property --my-color {
		syntax: '<color>';
		inherits: false;
		initial-value: #c0ffee;
	}

	@property --item-size {
		syntax: '<length>';
		inherits: true;
		initial-value: 16px;
	}

	@property --wildcard {
		syntax: '*';
		inherits: false;
	}
	`

	sheet := css.Parse(src)
	if len(sheet.Properties) != 3 {
		t.Fatalf("sheet.Properties len = %d, want 3", len(sheet.Properties))
	}

	p0 := sheet.Properties[0]
	if p0.Name != "--my-color" || p0.Syntax != "<color>" || p0.Inherits != false || p0.InitialValue != "#c0ffee" {
		t.Errorf("Properties[0] = %+v, want --my-color <color> false #c0ffee", p0)
	}

	p1 := sheet.Properties[1]
	if p1.Name != "--item-size" || p1.Syntax != "<length>" || p1.Inherits != true || p1.InitialValue != "16px" {
		t.Errorf("Properties[1] = %+v, want --item-size <length> true 16px", p1)
	}

	p2 := sheet.Properties[2]
	if p2.Name != "--wildcard" || p2.Syntax != "*" || p2.Inherits != false {
		t.Errorf("Properties[2] = %+v, want --wildcard * false", p2)
	}
}

func TestPropertyRule_ParseInvalidDropped(t *testing.T) {
	src := `
	/* Invalid: not starting with -- */
	@property my-color {
		syntax: '<color>';
		inherits: false;
		initial-value: red;
	}

	/* Invalid: missing inherits */
	@property --no-inherits {
		syntax: '<color>';
		initial-value: red;
	}

	/* Invalid: missing syntax */
	@property --no-syntax {
		inherits: false;
		initial-value: red;
	}

	/* Invalid: initial-value doesn't match syntax */
	@property --mismatch {
		syntax: '<color>';
		inherits: false;
		initial-value: 100px;
	}
	`

	sheet := css.Parse(src)
	if len(sheet.Properties) != 0 {
		t.Errorf("sheet.Properties len = %d, want 0 (all invalid dropped)", len(sheet.Properties))
	}
}

func TestStartingStyle_TopLevelAndNested(t *testing.T) {
	src := `
	dialog {
		opacity: 1;
		transform: scale(1);
	}

	@starting-style {
		dialog {
			opacity: 0;
			transform: scale(0.8);
		}
	}

	.card {
		opacity: 1;
		@starting-style {
			opacity: 0;
		}
	}
	`

	sheet := css.Parse(src)
	if len(sheet.Rules) != 4 {
		t.Fatalf("sheet.Rules len = %d, want 4", len(sheet.Rules))
	}

	// Normal dialog rule
	if sheet.Rules[0].StartingStyle {
		t.Errorf("Rules[0].StartingStyle = true, want false")
	}

	// @starting-style dialog rule
	if !sheet.Rules[1].StartingStyle {
		t.Errorf("Rules[1].StartingStyle = false, want true")
	}
	if len(sheet.Rules[1].Declarations) != 2 {
		t.Errorf("Rules[1].Declarations len = %d, want 2", len(sheet.Rules[1].Declarations))
	}

	// Normal .card rule
	if sheet.Rules[2].StartingStyle {
		t.Errorf("Rules[2].StartingStyle = true, want false")
	}

	// Nested @starting-style .card rule
	if !sheet.Rules[3].StartingStyle {
		t.Errorf("Rules[3].StartingStyle = false, want true")
	}
	if len(sheet.Rules[3].Declarations) != 1 || sheet.Rules[3].Declarations[0].Property != "opacity" {
		t.Errorf("Rules[3].Declarations = %+v, want opacity: 0", sheet.Rules[3].Declarations)
	}
}
