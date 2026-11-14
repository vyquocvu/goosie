package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

// qc:BR-197
func TestOpacityClampsToUnitRange(t *testing.T) {
	html := `<!DOCTYPE html><html><head><style>
		.a{opacity:0} .b{opacity:0.5} .c{opacity:.5} .d{opacity:5}
		.e{opacity:0%} .f{opacity:50%} .g{opacity:1} .bad{opacity:oops}
	</style></head><body>
		<div id="a" class="a"></div><div id="b" class="b"></div><div id="c" class="c"></div>
		<div id="d" class="d"></div><div id="e" class="e"></div><div id="f" class="f"></div>
		<div id="g" class="g"></div><div id="bad" class="bad"></div>
	</body></html>`
	doc := domtest.Parse(html)
	var sheets []*css.Stylesheet
	for _, node := range doc.ElementsByTagName("style") {
		sheets = append(sheets, css.Parse(node.TextContent()))
	}
	styles := style.ResolveViewport(doc, sheets, style.Viewport{}, nil)

	cases := []struct {
		id   string
		want float32
	}{
		{"a", 0}, {"b", 0.5}, {"c", 0.5}, {"d", 1},
		{"e", 0}, {"f", 0.5},
		{"g", 1},
		{"bad", 1},
	}
	for _, tc := range cases {
		el := doc.ElementByID(tc.id)
		if el == nil {
			t.Fatalf("missing element %q", tc.id)
		}
		cs := styles[el.ID]
		if cs == nil {
			t.Fatalf("no computed style for %q", tc.id)
		}
		if cs.Opacity != tc.want {
			t.Errorf("computed opacity for #%s = %v, want %v", tc.id, cs.Opacity, tc.want)
		}
	}
}

// qc:BR-197
func TestVisibilityInheritsAndRestores(t *testing.T) {
	html := `<!DOCTYPE html><html><head><style>
		.parent{visibility:hidden}
		.shown{visibility:visible}
	</style></head><body>
		<div id="p" class="parent">
			<div id="silent"></div>
			<div id="shown" class="shown">
				<div id="grand"></div>
			</div>
		</div>
	</body></html>`
	doc := domtest.Parse(html)
	var sheets []*css.Stylesheet
	for _, node := range doc.ElementsByTagName("style") {
		sheets = append(sheets, css.Parse(node.TextContent()))
	}
	styles := style.ResolveViewport(doc, sheets, style.Viewport{}, nil)

	cases := []struct {
		id   string
		want string
	}{
		{"p", "hidden"},
		{"silent", "hidden"},
		{"shown", "visible"},
		{"grand", "visible"},
	}
	for _, tc := range cases {
		el := doc.ElementByID(tc.id)
		cs := styles[el.ID]
		if cs == nil {
			t.Fatalf("no computed style for %q", tc.id)
		}
		if cs.Visibility != tc.want {
			t.Errorf("computed visibility for #%s = %q, want %q", tc.id, cs.Visibility, tc.want)
		}
	}
}
