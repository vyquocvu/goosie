package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

func abCascade(t *testing.T, cssText string) (*dom.Document, map[dom.NodeID]*style.ComputedStyle) {
	t.Helper()
	html := `<!doctype html><html><head><style>` + cssText + `</style></head><body>` +
		`<div id="gen">one two<div style="height:20px"></div>three four</div></body></html>`
	doc := domtest.Parse(html)
	var sheets []*css.Stylesheet
	for _, node := range doc.ElementsByTagName("style") {
		sheets = append(sheets, css.Parse(node.TextContent()))
	}
	return doc, style.ResolveViewport(doc, sheets, style.Viewport{}, nil)
}

func abStyle(t *testing.T, doc *dom.Document, styles map[dom.NodeID]*style.ComputedStyle) *style.ComputedStyle {
	t.Helper()
	el := doc.ElementByID("gen")
	if el == nil {
		t.Fatal("missing element gen")
	}
	cs := styles[el.ID]
	if cs == nil {
		t.Fatal("no computed style for gen")
	}
	return cs
}

// qc:BR-200
func TestAnonymousBlockStyleCarriesNoBackgroundLayers(t *testing.T) {
	for _, tc := range []struct {
		name         string
		css          string
		wantColor    bool
		wantLinear   bool
		wantRadial   bool
		wantImageURL bool
	}{
		{
			name:       "translucent linear gradient in the shorthand",
			css:        `div{background:linear-gradient(rgba(200,0,0,0.5),rgba(200,0,0,0.5))}`,
			wantLinear: true,
		},
		{
			name:       "radial gradient in the shorthand",
			css:        `div{background:radial-gradient(rgba(200,0,0,0.5),rgba(0,0,200,0.5))}`,
			wantRadial: true,
		},
		{
			name:       "gradient over a colour",
			css:        `div{background:rgba(0,120,0,0.5) linear-gradient(rgb(200,0,0),rgb(0,0,200))}`,
			wantColor:  true,
			wantLinear: true,
		},
		{
			name:         "background-image url",
			css:          `div{background-image:url(/tile.png)}`,
			wantImageURL: true,
		},
		{
			name:      "plain translucent colour",
			css:       `div{background:rgba(200,0,0,0.5)}`,
			wantColor: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc, styles := abCascade(t, tc.css)
			parent := abStyle(t, doc, styles)

			if got := parent.BackgroundColor.A > 0; got != tc.wantColor {
				t.Errorf("element BackgroundColor.A = %d, present=%v, want present=%v",
					parent.BackgroundColor.A, got, tc.wantColor)
			}
			if got := !parent.BackgroundGradient.Empty(); got != tc.wantLinear {
				t.Errorf("element linear gradient stops=%d, present=%v, want present=%v",
					len(parent.BackgroundGradient.Stops), got, tc.wantLinear)
			}
			if got := !parent.BackgroundRadialGradient.Empty(); got != tc.wantRadial {
				t.Errorf("element radial gradient stops=%d, present=%v, want present=%v",
					len(parent.BackgroundRadialGradient.Stops), got, tc.wantRadial)
			}
			if got := parent.BackgroundImage != ""; got != tc.wantImageURL {
				t.Errorf("element BackgroundImage=%q, present=%v, want present=%v",
					parent.BackgroundImage, got, tc.wantImageURL)
			}

			anon := style.AnonymousBlockStyle(parent)
			if anon.BackgroundColor.A != 0 {
				t.Errorf("anonymous block BackgroundColor.A = %d (rgba %d,%d,%d), want 0: the generating element already painted that colour",
					anon.BackgroundColor.A,
					anon.BackgroundColor.R, anon.BackgroundColor.G, anon.BackgroundColor.B)
			}
			if anon.BackgroundImage != "" {
				t.Errorf("anonymous block BackgroundImage = %q, want %q: the element already owns that layer",
					anon.BackgroundImage, "")
			}
			if !anon.BackgroundGradient.Empty() {
				t.Errorf("anonymous block linear gradient carries %d stops %v, want none: a copied ramp composites over the element's own, which measured 9693 px of the twice-composited (213,63,63) where Chromium has 0",
					len(anon.BackgroundGradient.Stops), anon.BackgroundGradient.Stops)
			}
			if !anon.BackgroundRadialGradient.Empty() {
				t.Errorf("anonymous block radial gradient carries %d stops %v, want none: same double composite as the linear layer",
					len(anon.BackgroundRadialGradient.Stops), anon.BackgroundRadialGradient.Stops)
			}

			if anon.Display != style.DisplayBlock {
				t.Errorf("anonymous block Display = %v, want DisplayBlock", anon.Display)
			}
			if anon.FontSize != parent.FontSize || anon.Color != parent.Color || anon.FontFamily != parent.FontFamily {
				t.Errorf("anonymous block typography lost: font-size %v vs %v, color %v vs %v, family %q vs %q",
					anon.FontSize, parent.FontSize, anon.Color, parent.Color, anon.FontFamily, parent.FontFamily)
			}
		})
	}
}

// qc:BR-200
func TestAnonymousBlockAndTableStyleCollapseBackgroundsAlike(t *testing.T) {
	doc, styles := abCascade(t, `div{background:rgba(200,0,0,0.5) linear-gradient(rgba(0,0,200,0.5),rgba(0,200,0,0.5))}`)
	parent := abStyle(t, doc, styles)
	block := style.AnonymousBlockStyle(parent)
	row := style.AnonymousTableStyle(parent, style.DisplayTableRow)

	if block.BackgroundColor.A != row.BackgroundColor.A ||
		block.BackgroundImage != row.BackgroundImage ||
		block.BackgroundGradient.Empty() != row.BackgroundGradient.Empty() ||
		block.BackgroundRadialGradient.Empty() != row.BackgroundRadialGradient.Empty() {
		t.Errorf("anonymous builders disagree on backgrounds: block colour.A=%d image=%q linear=%d stops radial=%d stops, table colour.A=%d image=%q linear=%d stops radial=%d stops",
			block.BackgroundColor.A, block.BackgroundImage, len(block.BackgroundGradient.Stops), len(block.BackgroundRadialGradient.Stops),
			row.BackgroundColor.A, row.BackgroundImage, len(row.BackgroundGradient.Stops), len(row.BackgroundRadialGradient.Stops))
	}
	if parent.BackgroundColor.A == 0 || parent.BackgroundGradient.Empty() {
		t.Errorf("the declaring element lost its own background (colour.A=%d, linear stops=%d): only the anonymous copy may be collapsed",
			parent.BackgroundColor.A, len(parent.BackgroundGradient.Stops))
	}
}
