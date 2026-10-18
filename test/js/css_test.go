package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
)

func TestCSS_Escape(t *testing.T) {
	doc := &dom.Document{}
	rt, err := js.New(js.Options{DOM: doc})
	if err != nil {
		t.Fatalf("js.New failed: %v", err)
	}
	defer rt.Close()

	tests := []struct {
		input string
		want  string
	}{
		{".foo#bar", "\\.foo\\#bar"},
		{"hello world", "hello\\ world"},
		{"123", "\\31 23"},
		{"-a", "-a"},
	}

	for _, tt := range tests {
		got, err := rt.Evaluate("CSS.escape(" + `"` + tt.input + `"` + ")")
		if err != nil {
			t.Errorf("CSS.escape(%q) failed: %v", tt.input, err)
		} else if got != tt.want {
			t.Errorf("CSS.escape(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCSS_Supports(t *testing.T) {
	doc := &dom.Document{}
	rt, err := js.New(js.Options{DOM: doc})
	if err != nil {
		t.Fatalf("js.New failed: %v", err)
	}
	defer rt.Close()

	tests := []string{
		`CSS.supports("display", "flex")`,
		`CSS.supports("color", "red")`,
		`CSS.supports("(display: grid)")`,
		`CSS.supports("--custom-prop", "10px")`,
	}

	for _, expr := range tests {
		got, err := rt.Evaluate(expr)
		if err != nil || got != "true" {
			t.Errorf("%s = %q, want 'true', err: %v", expr, got, err)
		}
	}
}

func TestCSS_RegisterProperty_Success(t *testing.T) {
	doc := &dom.Document{}
	mutated := false
	rt, err := js.New(js.Options{
		DOM: doc,
		OnMutation: func() {
			mutated = true
		},
	})
	if err != nil {
		t.Fatalf("js.New failed: %v", err)
	}
	defer rt.Close()

	err = rt.Run(`
		CSS.registerProperty({
			name: '--brand-color',
			syntax: '<color>',
			inherits: false,
			initialValue: '#ff0000'
		});

		CSS.registerProperty({
			name: '--spacing',
			syntax: '<length>',
			inherits: true,
			initialValue: '16px'
		});

		CSS.registerProperty({
			name: '--any-val',
			syntax: '*',
			inherits: false
		});
	`, "test.js")
	if err != nil {
		t.Fatalf("CSS.registerProperty failed: %v", err)
	}

	if !mutated {
		t.Errorf("OnMutation was not called after CSS.registerProperty")
	}

	props := doc.RegisteredProperties()
	if len(props) != 3 {
		t.Fatalf("RegisteredProperties len = %d, want 3", len(props))
	}

	p1, ok := props["--brand-color"]
	if !ok || p1.Syntax != "<color>" || p1.Inherits != false || p1.InitialValue != "#ff0000" {
		t.Errorf("--brand-color = %+v, want <color> false #ff0000", p1)
	}

	p2, ok := props["--spacing"]
	if !ok || p2.Syntax != "<length>" || p2.Inherits != true || p2.InitialValue != "16px" {
		t.Errorf("--spacing = %+v, want <length> true 16px", p2)
	}

	p3, ok := props["--any-val"]
	if !ok || p3.Syntax != "*" || p3.Inherits != false {
		t.Errorf("--any-val = %+v, want * false", p3)
	}
}

func TestCSS_RegisterProperty_Errors(t *testing.T) {
	doc := &dom.Document{}
	rt, err := js.New(js.Options{DOM: doc})
	if err != nil {
		t.Fatalf("js.New failed: %v", err)
	}
	defer rt.Close()

	// 1. Missing '--' prefix
	err = rt.Run(`
		CSS.registerProperty({
			name: 'brand-color',
			syntax: '<color>',
			inherits: false,
			initialValue: '#ff0000'
		});
	`, "test.js")
	if err == nil {
		t.Errorf("CSS.registerProperty without '--' should throw SyntaxError, got nil")
	}

	// 2. Invalid syntax
	err = rt.Run(`
		CSS.registerProperty({
			name: '--bad-syntax',
			syntax: '<unknown-syntax-type>',
			inherits: false,
			initialValue: '10px'
		});
	`, "test.js")
	if err == nil {
		t.Errorf("CSS.registerProperty with invalid syntax should throw SyntaxError, got nil")
	}

	// 3. Missing inherits
	err = rt.Run(`
		CSS.registerProperty({
			name: '--no-inherits',
			syntax: '<length>',
			initialValue: '10px'
		});
	`, "test.js")
	if err == nil {
		t.Errorf("CSS.registerProperty without inherits should throw TypeError, got nil")
	}

	// 4. Initial value mismatch
	err = rt.Run(`
		CSS.registerProperty({
			name: '--mismatch',
			syntax: '<color>',
			inherits: false,
			initialValue: '100px'
		});
	`, "test.js")
	if err == nil {
		t.Errorf("CSS.registerProperty with mismatched initialValue should throw SyntaxError, got nil")
	}

	// 5. Duplicate registration
	err = rt.Run(`
		CSS.registerProperty({
			name: '--dup',
			syntax: '<color>',
			inherits: false,
			initialValue: 'red'
		});
		CSS.registerProperty({
			name: '--dup',
			syntax: '<color>',
			inherits: false,
			initialValue: 'blue'
		});
	`, "test.js")
	if err == nil {
		t.Errorf("CSS.registerProperty duplicate should throw InvalidModificationError, got nil")
	}
}
