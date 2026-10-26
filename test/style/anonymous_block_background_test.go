package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

// These are the style-layer oracles for GOOSIE-ANONBG-001: an anonymous block box
// re-composited its generating element's gradient, because style.AnonymousBlockStyle
// copies that element's computed style verbatim and cleared only BackgroundColor.
// The two sibling layers that actually paint, BackgroundGradient and
// BackgroundRadialGradient, survived the copy, and internal/paint/builder_box.go
// emits those layers for every box whose display is not inline - which an anonymous
// block is, because AnonymousBlockStyle sets DisplayBlock.
//
// Measured on this host with cmd/goosie-headless at 400x120 DPR 1 over a 300x76 block
// declaring `background:linear-gradient(rgba(200,0,0,.5),rgba(200,0,0,.5))` and
// holding raw text, a 20px-tall div sibling and more raw text. cmd/dump-layout splices
// exactly two anonymous boxes there, (0,0,300,18) and (0,38,300,18); those two bands
// carried 9,693 px of (213,63,63), the twice-composited value, against Chromium's 0.
// Clearing all four layers puts the block at 21,693 px of (227,127,127) and no doubled
// pixel anywhere.
//
// The rule the fix implements is that a computed background belongs to the element that
// declared it: css-backgrounds-3 §4 paints a background on the element's own boxes and
// css-cascade-4 §6.4 lists background-color and background-image as non-inherited, so a
// box layout invented to hold content owns none of them. AnonymousTableStyle already
// collapsed all four; AnonymousBlockStyle was the inconsistent one, and the paint layer
// cannot tell the two box kinds apart (layout keeps flagAnonymous unexported and gives an
// anonymous box no BoxKey), which is why this belongs to the style stage.
//
// The controls matter as much as the collapse: the element keeps all four layers and the
// anonymous box keeps the element's typography. A mechanism that zeroes the declaring
// element's own background, or that drops everything the copy is for, goes red here.

// abCascade resolves a document's cascade the way the engine's style pass does, and
// returns the style table with the document so a case can name an element by id.
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

// abStyle returns the computed style of #gen, the box every case here generates
// anonymous children from.
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

func TestAnonymousBlockStyleCarriesNoBackgroundLayers(t *testing.T) {
	for _, tc := range []struct {
		name string
		css  string
		// Which of the four layers the declaration is expected to fill on the
		// element. Listed explicitly so a declaration that silently stops
		// parsing cannot make the case pass by vacuity: zero layers on the
		// element and zero on the anonymous box would look like a fix.
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

			// Control first: the element that declared the background must still
			// carry exactly the layers the declaration names.
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

			// The copy is load-bearing for everything else: an anonymous block
			// still draws its text in the element's font and colour.
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

// TestAnonymousBlockAndTableStyleCollapseBackgroundsAlike pins the invariant whose
// absence was the defect: both anonymous style builders hand out no background layer.
// Before this fix the table builder cleared four layers and the block builder cleared
// one, so a gradient reached an anonymous block box that a table wrapper never got.
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
