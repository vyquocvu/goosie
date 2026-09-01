package css_test

import (
	"testing"
	"github.com/vyquocvu/goosie/internal/css"
)

// TestExtractResources_Import — M7 acceptance: @import urls are
// discovered and reported as ResourceStylesheet.
func TestExtractResources_Import(t *testing.T) {
	rawCSS := `@import "theme.css"; @import url('fonts.css');`
	sheet, err := css.NewParser(rawCSS).Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := css.ExtractResources(sheet)
	if len(got) != 2 {
		t.Fatalf("got %d resources, want 2: %#v", len(got), got)
	}
	if got[0].Kind != css.ResourceStylesheet || got[0].URL != "theme.css" {
		t.Errorf("got[0] = %+v, want {stylesheet, theme.css}", got[0])
	}
	if got[1].Kind != css.ResourceStylesheet || got[1].URL != "fonts.css" {
		t.Errorf("got[1] = %+v, want {stylesheet, fonts.css}", got[1])
	}
}

// TestExtractResources_FontFace — @font-face src: url(...) is
// reported as ResourceFont.
func TestExtractResources_FontFace(t *testing.T) {
	rawCSS := `
@font-face {
  font-family: 'MyFont';
  src: url('myfont.woff2') format('woff2'),
       url('myfont.woff') format('woff');
}`
	sheet, err := css.NewParser(rawCSS).Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := css.ExtractResources(sheet)
	if len(got) != 2 {
		t.Fatalf("got %d resources, want 2: %#v", len(got), got)
	}
	for _, g := range got {
		if g.Kind != css.ResourceFont {
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

// TestExtractResources_ImageInDeclaration — url() in any
// declaration value is reported as ResourceImage with Property.
func TestExtractResources_ImageInDeclaration(t *testing.T) {
	rawCSS := `
.x { background-image: url('bg.png'); }
.y { list-style-image: url("bullet.svg"); }`
	sheet, err := css.NewParser(rawCSS).Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := css.ExtractResources(sheet)
	if len(got) != 2 {
		t.Fatalf("got %d resources, want 2: %#v", len(got), got)
	}
	want := map[string]string{
		"bg.png":     "background-image",
		"bullet.svg": "list-style-image",
	}
	for _, g := range got {
		if g.Kind != css.ResourceImage {
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

// TestExtractResources_NestedAtRule — @media containing rules
// with url() is discovered.
func TestExtractResources_NestedAtRule(t *testing.T) {
	rawCSS := `
@media screen {
  .x { background: url('nested.png'); }
}`
	sheet, err := css.NewParser(rawCSS).Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := css.ExtractResources(sheet)
	if len(got) != 1 {
		t.Fatalf("got %d resources, want 1: %#v", len(got), got)
	}
	if got[0].URL != "nested.png" || got[0].Kind != css.ResourceImage {
		t.Errorf("got %+v", got[0])
	}
}

// TestExtractResources_QuotedAndUnquoted — url() handles double,
// single, and unquoted forms.
func TestExtractResources_QuotedAndUnquoted(t *testing.T) {
	rawCSS := `
.a { background: url(double.png); }
.b { background: url('single.png'); }
.c { background: url("quoted.png"); }`
	sheet, err := css.NewParser(rawCSS).Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := css.ExtractResources(sheet)
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

// TestExtractResources_NoURLs — declarations without url() yield
// no resources.
func TestExtractResources_NoURLs(t *testing.T) {
	rawCSS := `.x { color: red; width: 100px; }`
	sheet, err := css.NewParser(rawCSS).Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := css.ExtractResources(sheet)
	if len(got) != 0 {
		t.Errorf("got %d, want 0: %#v", len(got), got)
	}
}

// TestExtractResources_NilSheet — nil sheet returns nil.
func TestExtractResources_NilSheet(t *testing.T) {
	if got := css.ExtractResources(nil); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

// TestExtractResources_FontFacePropertyCarries — ResourceImage
// preserves the source property name for the renderer to map back.
func TestExtractResources_FontFacePropertyCarries(t *testing.T) {
	rawCSS := `
@font-face {
  font-family: 'X';
  src: url('x.woff2');
}
.a { background-image: url('bg.png'); }
`
	sheet, err := css.NewParser(rawCSS).Parse()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := css.ExtractResources(sheet)
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	for _, g := range got {
		if g.Kind == css.ResourceFont && g.Property != "" {
			t.Errorf("font resource carried property %q (should be empty)", g.Property)
		}
		if g.Kind == css.ResourceImage && g.Property == "" {
			t.Errorf("image resource missing property name")
		}
	}
}

// TestResourceKind_String — string round-trip for log lines.
func TestResourceKind_String(t *testing.T) {
	cases := map[css.ResourceKind]string{
		css.ResourceStylesheet: "stylesheet",
		css.ResourceFont:       "font",
		css.ResourceImage:      "image",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", k, got, want)
		}
	}
	if got := css.ResourceKind(99).String(); got != "unknown" {
		t.Errorf("unknown kind = %q, want 'unknown'", got)
	}
}
