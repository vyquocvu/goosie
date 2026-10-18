package style_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
)

func TestAccentColorParsing(t *testing.T) {
	tests := []struct {
		name   string
		value  string
		expect bool // should parse successfully
	}{
		{"auto", "auto", true},
		{"blue keyword", "blue", true},
		{"rgb(255, 0, 0)", "rgb(255, 0, 0)", true},
		{"#ff5733 hex", "#ff5733", true},
		{"invalid color", "notacolor", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ok bool
			val := strings.ToLower(strings.TrimSpace(tt.value))
			if val == "auto" {
				ok = true
			} else {
				_, ok = css.ParseColorWithScheme(val, "normal")
			}

			if !tt.expect && ok {
				t.Errorf("Expected invalid color %q to fail", tt.value)
			}
		})
	}
}

func TestAccentColorDefault(t *testing.T) {
	cs := style.DefaultStyle()
	if cs.AccentColor.A != 0 {
		t.Errorf("Default accent-color should have zero alpha, got %v", cs.AccentColor)
	}
}

func TestAccentColorInheritance(t *testing.T) {
	parent := &style.ComputedStyle{
		AccentColor: css.Color{R: 255, G: 128, B: 0, A: 255},
		Color:       css.Color{R: 0, G: 0, B: 0, A: 255},
		FontSize:    16,
	}

	child := style.AnonymousBlockStyle(parent)

	if child.AccentColor != parent.AccentColor {
		t.Errorf("Expected inherited accent-color %v, got %v", parent.AccentColor, child.AccentColor)
	}

	parent2 := &style.ComputedStyle{}
	child2 := style.AnonymousBlockStyle(parent2)

	if child2.AccentColor.A != 0 {
		t.Errorf("Expected zero alpha when parent has no accent-color, got %v", child2.AccentColor)
	}
}

func TestAccentColorResolveAndInherit(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)

	form := doc.NewElement("form")
	form.SetAttribute("id", "f1")
	body.AppendChild(form)

	input := doc.NewElement("input")
	input.SetAttribute("type", "checkbox")
	input.SetAttribute("id", "cb1")
	form.AppendChild(input)

	sheet := css.Parse(`
		#f1 { accent-color: #ff0077; }
	`)

	styles := style.Resolve(doc, []*css.Stylesheet{sheet}, nil)
	sForm := styles[form.ID]
	if sForm.AccentColor != (css.Color{R: 255, G: 0, B: 119, A: 255}) {
		t.Fatalf("unexpected AccentColor on form: %v", sForm.AccentColor)
	}

	sInput := styles[input.ID]
	if sInput.AccentColor != sForm.AccentColor {
		t.Fatalf("expected input to inherit AccentColor %v, got %v", sForm.AccentColor, sInput.AccentColor)
	}
}
