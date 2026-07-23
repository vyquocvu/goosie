package css

import (
	"testing"
)

// TestM7_ExtractResources_Import — M7 acceptance: @import urls are
// discovered and reported as ResourceStylesheet.
func TestM7_ExtractResources_Import(t *testing.T) {
	css := `@import "theme.css"; @import url('fonts.css');`
	sheet, err := NewParser(css).Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := ExtractResources(sheet)
	if len(got) != 2 {
		t.Fatalf("got %d resources, want 2: %#v", len(got), got)
	}
	if got[0].Kind != ResourceStylesheet || got[0].URL != "theme.css" {
		t.Errorf("got[0] = %+v, want {stylesheet, theme.css}", got[0])
	}
	if got[1].Kind != ResourceStylesheet || got[1].URL != "fonts.css" {
		t.Errorf("got[1] = %+v, want {stylesheet, fonts.css}", got[1])
	}
}

// TestM7_ExtractResources_FontFace — @font-face src: url(...) is
// reported as ResourceFont.
func TestM7_ExtractResources_FontFace(t *testing.T) {
	css := `
@font-face {
  font-family: 'MyFont';
  src: url('myfont.woff2') format('woff2'),
       url('myfont.woff') format('woff');
}`
	sheet, err := NewParser(css).Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := ExtractResources(sheet)
	if len(got) != 2 {
		t.Fatalf("got %d resources, want 2: %#v", len(got), got)
	}
	for _, g := range got {
		if g.Kind != ResourceFont {
			t.Errorf("got %+v, want font kind", g)
		}
	}
	urls := map[string]bool{}
	for _, g := range got {
		urls[g.URL] = true
	}
	if !urls["myfont.woff2"] || !urls["myfont.woff"] {
		t.Errorf("missing fonts in %v", urls)
	}
}

// TestM7_ExtractResources_ImageInDeclaration — url() in any
// declaration value is reported as ResourceImage with Property.
func TestM7_ExtractResources_ImageInDeclaration(t *testing.T) {
	css := `
.x { background-image: url('bg.png'); }
.y { list-style-image: url("bullet.svg"); }`
	sheet, err := NewParser(css).Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := ExtractResources(sheet)
	if len(got) != 2 {
		t.Fatalf("got %d resources, want 2: %#v", len(got), got)
	}
	want := map[string]string{
		"bg.png":     "background-image",
		"bullet.svg": "list-style-image",
	}
	for _, g := range got {
		if g.Kind != ResourceImage {
			t.Errorf("got %+v, want image kind", g)
		}
		expected, ok := want[g.URL]
		if !ok {
			t.Errorf("unexpected url %q", g.URL)
		}
		if g.Property != expected {
			t.Errorf("property for %q = %q, want %q", g.URL, g.Property, expected)
		}
	}
}

// TestM7_ExtractResources_NestedAtRule — @media containing rules
// with url() is discovered.
func TestM7_ExtractResources_NestedAtRule(t *testing.T) {
	css := `
@media screen {
  .x { background: url('nested.png'); }
}`
	sheet, err := NewParser(css).Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := ExtractResources(sheet)
	if len(got) != 1 {
		t.Fatalf("got %d resources, want 1: %#v", len(got), got)
	}
	if got[0].URL != "nested.png" || got[0].Kind != ResourceImage {
		t.Errorf("got %+v", got[0])
	}
}

// TestM7_ExtractResources_QuotedAndUnquoted — url() handles double,
// single, and unquoted forms.
func TestM7_ExtractResources_QuotedAndUnquoted(t *testing.T) {
	css := `
.a { background: url(double.png); }
.b { background: url('single.png'); }
.c { background: url("quoted.png"); }`
	sheet, err := NewParser(css).Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := ExtractResources(sheet)
	if len(got) != 3 {
		t.Fatalf("got %d, want 3", len(got))
	}
	want := map[string]bool{"double.png": true, "single.png": true, "quoted.png": true}
	for _, g := range got {
		if !want[g.URL] {
			t.Errorf("unexpected url %q", g.URL)
		}
	}
}

// TestM7_ExtractResources_NoURLs — declarations without url() yield
// no resources.
func TestM7_ExtractResources_NoURLs(t *testing.T) {
	css := `.x { color: red; width: 100px; }`
	sheet, err := NewParser(css).Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := ExtractResources(sheet)
	if len(got) != 0 {
		t.Errorf("got %d, want 0: %#v", len(got), got)
	}
}

// TestM7_ExtractResources_NilSheet — nil sheet returns nil.
func TestM7_ExtractResources_NilSheet(t *testing.T) {
	if got := ExtractResources(nil); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

// TestM7_ExtractResources_FontFacePropertyCarries — ResourceImage
// preserves the source property name for the renderer to map back.
func TestM7_ExtractResources_FontFacePropertyCarries(t *testing.T) {
	css := `
@font-face {
  font-family: 'X';
  src: url('x.woff2');
}
.a { background-image: url('bg.png'); }
`
	sheet, err := NewParser(css).Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := ExtractResources(sheet)
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	for _, g := range got {
		if g.Kind == ResourceFont && g.Property != "" {
			t.Errorf("font resource carried property %q (should be empty)", g.Property)
		}
		if g.Kind == ResourceImage && g.Property == "" {
			t.Errorf("image resource missing property name")
		}
	}
}

// TestM7_ResourceKind_String — string round-trip for log lines.
func TestM7_ResourceKind_String(t *testing.T) {
	cases := map[ResourceKind]string{
		ResourceStylesheet: "stylesheet",
		ResourceFont:       "font",
		ResourceImage:      "image",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", k, got, want)
		}
	}
	if got := ResourceKind(99).String(); got != "unknown" {
		t.Errorf("unknown kind = %q, want 'unknown'", got)
	}
}
