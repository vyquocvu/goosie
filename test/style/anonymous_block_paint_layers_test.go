package style_test

import (
	"reflect"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/style"
)

// These are the cascade-side oracles for GOOSIE-SHADOWRUN-001's anonymous-box
// multiplier: style.AnonymousBlockStyle starts from a whole-struct copy of the
// generating element's ComputedStyle (`s = *parent`) and TASK-049 cleared only the
// four background layers, so box-shadow, border-radius, mask-* and border-image
// were handed to every anonymous block layout splices around a run of inline
// content. TASK-049 §6.3 named the leftovers; this file pins which ones go and
// which one must stay.
//
// The rule is the cascade's, per property, and the split was measured in pixels
// against Chromium 131.0.6778.33 on a 300x76 block that generates two anonymous
// boxes ((0,0,300,18) and (0,38,300,18) per cmd/dump-layout) before any of it was
// encoded here. "Effect" below is the pixels a declaration changes versus the same
// document without it:
//
//	box-shadow     Chromium 7780 px plain, 7780 anon, 0 of them inside the bands;
//	               Goosie pre-fix 6272 plain, 17072 anon. Non-inherited
//	               (css-backgrounds-3 §7.1): the copy must not carry it.
//	border-radius  Chromium 412 both shapes, Goosie 385 both: already inert at
//	               paint because a radius only shapes the background and the border
//	               that paintBoxDecoration draws for the box, and the copy has
//	               neither, so this one is a cascade pin only. Non-inherited
//	               (css-backgrounds-3 §4): clear it.
//	mask-*         Chromium 110 px plain / 119 anon (identical), Goosie 22773 both.
//	               Inert at paint: engine.go attaches mask pixels only to boxes with
//	               a DOM node and an anonymous box has none. Non-inherited
//	               (css-masking-1 §5.3): clear it.
//	border-image   Chromium 7920 both shapes, 3260 of it inside the bands; Goosie
//	               7920 both. Inert at paint: the copy's border widths are already
//	               zero and border-image pixels are node-gated too. Non-inherited
//	               (css-backgrounds-3 §6.2): clear it.
//	text-shadow    Chromium 655 px plain / 648 anon, ALL inside the bands - the
//	               glyphs are exactly where the property belongs. Inherited
//	               (css-text-decor-3 §3): the copy must KEEP it.
//
// The text-shadow case is the one a blanket collapse gets wrong. It is also the one
// whose regression cannot be seen in Goosie's pixels, because a text run takes its
// style from the DOM element through style.inheritStyle, never from the anonymous
// block; only the cascade assertion below can catch a copy that lost the layer.

// TestAnonymousBlockStyleCarriesNoElementDecoration is the per-property collapse:
// the element that declared the decoration keeps all of it, the anonymous block box
// carries none of the four non-inherited groups.
// qc:BR-229
func TestAnonymousBlockStyleCarriesNoElementDecoration(t *testing.T) {
	for _, tc := range []struct {
		name string
		css  string
		// declared asserts the element still owns what it declared. It runs first so
		// a declaration that silently stops parsing cannot make the case pass by
		// vacuity: an element with nothing and a copy with nothing look like a fix.
		declared func(t *testing.T, el *style.ComputedStyle)
		// cleared asserts the anonymous copy carries none of it.
		cleared func(t *testing.T, anon *style.ComputedStyle)
	}{
		{
			name: "box-shadow longhand",
			css:  `div{box-shadow:0 0 10px 6px rgba(200,0,0,0.5)}`,
			declared: func(t *testing.T, el *style.ComputedStyle) {
				if len(el.BoxShadow) != 1 {
					t.Fatalf("element box-shadow layers = %d, want 1", len(el.BoxShadow))
				}
			},
			cleared: func(t *testing.T, anon *style.ComputedStyle) {
				if len(anon.BoxShadow) != 0 {
					t.Errorf("anonymous block carries %d box-shadow layer(s) %v, want 0: "+
						"Chromium paints one ring for the element and none for the generated boxes "+
						"(7780 px effect with and without them); Goosie pre-fix painted 17072 px with them against 6272 without",
						len(anon.BoxShadow), anon.BoxShadow)
				}
			},
		},
		{
			name: "box-shadow list of two layers",
			css:  `div{box-shadow:1px 1px 2px rgb(200,0,0),0 0 4px rgb(0,0,200)}`,
			declared: func(t *testing.T, el *style.ComputedStyle) {
				if len(el.BoxShadow) != 2 {
					t.Fatalf("element box-shadow layers = %d, want 2", len(el.BoxShadow))
				}
			},
			cleared: func(t *testing.T, anon *style.ComputedStyle) {
				if len(anon.BoxShadow) != 0 {
					t.Errorf("anonymous block carries %d box-shadow layer(s), want 0: the multiplier is per layer, so a copy would double both",
						len(anon.BoxShadow))
				}
			},
		},
		{
			name: "border-radius",
			css:  `div{border-radius:8px 4px}`,
			declared: func(t *testing.T, el *style.ComputedStyle) {
				if el.BorderRadius[0][0] == 0 || el.BorderRadius[1][0] == 0 {
					t.Fatalf("element border-radius = %v, want the declared corners", el.BorderRadius)
				}
			},
			cleared: func(t *testing.T, anon *style.ComputedStyle) {
				if anon.BorderRadius != ([4][2]float32{}) {
					t.Errorf("anonymous block BorderRadius = %v, want zero: a radius belongs to the element's border box and an anonymous box has no border or background of its own",
						anon.BorderRadius)
				}
			},
		},
		{
			name: "mask longhands",
			css:  `div{mask-image:url(/m.svg);-webkit-mask-image:url(/m.svg);mask-repeat:no-repeat;mask-size:50% 50%;mask-position:center}`,
			declared: func(t *testing.T, el *style.ComputedStyle) {
				if el.MaskImage == "" || el.MaskRepeat == "" || el.MaskSize == "" || el.MaskPosition == "" {
					t.Fatalf("element mask = {%q %q %q %q}, want all four longhands",
						el.MaskImage, el.MaskRepeat, el.MaskSize, el.MaskPosition)
				}
			},
			cleared: func(t *testing.T, anon *style.ComputedStyle) {
				if anon.MaskImage != "" || anon.MaskRepeat != "" || anon.MaskSize != "" || anon.MaskPosition != "" {
					t.Errorf("anonymous block mask = {%q %q %q %q}, want all four empty: "+
						"Chromium applies the mask once (110 px effect plain / 119 anon, the same pixels), and mask-image does not inherit",
						anon.MaskImage, anon.MaskRepeat, anon.MaskSize, anon.MaskPosition)
				}
			},
		},
		{
			name: "border-image",
			css:  `div{border-style:solid;border-width:5px;border-image-source:url(/b.svg);border-image-slice:10;border-image-width:5px}`,
			declared: func(t *testing.T, el *style.ComputedStyle) {
				if el.BorderImage.Source != "/b.svg" {
					t.Fatalf("element border-image source = %q, want /b.svg", el.BorderImage.Source)
				}
			},
			cleared: func(t *testing.T, anon *style.ComputedStyle) {
				if anon.BorderImage != (css.BorderImage{}) {
					t.Errorf("anonymous block BorderImage = %+v, want zero: the copy has no border widths left to slice an image into, and Chromium's 7920 px border-image effect is identical with and without anonymous boxes",
						anon.BorderImage)
				}
			},
		},
		{
			name: "all of them at once",
			css: `div{box-shadow:0 0 10px 6px rgba(200,0,0,0.5);border-radius:8px;` +
				`mask-image:url(/m.svg);border-style:solid;border-width:5px;border-image-source:url(/b.svg);border-image-slice:10}`,
			declared: func(t *testing.T, el *style.ComputedStyle) {
				if len(el.BoxShadow) == 0 || el.BorderRadius[0][0] == 0 || el.MaskImage == "" || el.BorderImage.Source == "" {
					t.Fatalf("element does not carry the combined declaration: shadow=%d radius=%v mask=%q border-image=%q",
						len(el.BoxShadow), el.BorderRadius, el.MaskImage, el.BorderImage.Source)
				}
			},
			cleared: func(t *testing.T, anon *style.ComputedStyle) {
				if len(anon.BoxShadow) != 0 || anon.BorderRadius != ([4][2]float32{}) ||
					anon.MaskImage != "" || anon.BorderImage != (css.BorderImage{}) {
					t.Errorf("combined case leaked into the anonymous copy: shadow=%d radius=%v mask=%q border-image=%q",
						len(anon.BoxShadow), anon.BorderRadius, anon.MaskImage, anon.BorderImage.Source)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc, styles := abCascade(t, tc.css)
			parent := abStyle(t, doc, styles)
			tc.declared(t, parent)

			anon := style.AnonymousBlockStyle(parent)
			tc.cleared(t, anon)

			// The copy is still the element's typography and still a block: an
			// over-collapse that returns DefaultStyle would clear the properties above
			// for the wrong reason.
			if anon.Display != style.DisplayBlock {
				t.Errorf("anonymous block Display = %v, want DisplayBlock", anon.Display)
			}
			if anon.FontSize != parent.FontSize || anon.Color != parent.Color {
				t.Errorf("anonymous block typography lost: font-size %v vs %v, color %v vs %v",
					anon.FontSize, parent.FontSize, anon.Color, parent.Color)
			}
		})
	}
}

// TestAnonymousBlockStyleKeepsInheritedTextShadow is the other direction, and the
// reason this task is per property: text-shadow IS inherited, so the copy is what
// the text inside the box would compute anyway, and Chromium paints it exactly
// there (655 px with the declaration, 648 with the anonymous boxes present, all of
// them inside the bands). A blanket "clear what the parent had" would drop it.
// qc:BR-230
func TestAnonymousBlockStyleKeepsInheritedTextShadow(t *testing.T) {
	doc, styles := abCascade(t, `div{text-shadow:2px 2px 2px rgb(0,0,255),0 0 3px rgb(0,0,0)}`)
	parent := abStyle(t, doc, styles)
	if len(parent.TextShadow) != 2 {
		t.Fatalf("element text-shadow layers = %d, want 2", len(parent.TextShadow))
	}
	anon := style.AnonymousBlockStyle(parent)
	if !reflect.DeepEqual(anon.TextShadow, parent.TextShadow) {
		t.Errorf("anonymous block TextShadow = %+v, want the element's %+v: text-shadow inherits, so the generated box that holds the text carries it too; zeroing it here is the regression TASK-049 §6.3 warned about",
			anon.TextShadow, parent.TextShadow)
	}
	// And the element keeps its own, so the assertion above is not "both are nil".
	if len(anon.TextShadow) == 0 {
		t.Error("anonymous block TextShadow is empty")
	}
}

// TestAnonymousBlockAndTableStyleCollapseShadowsAlike pins the shape of the fix
// against the sibling builder TASK-049 already brought in line: both anonymous
// builders hand out no shadow, no radius, no mask and no border-image. Before this
// change the table builder cleared six groups and the block builder none, so a
// declaration reached an anonymous block box that a table wrapper never got.
// qc:BR-229
func TestAnonymousBlockAndTableStyleCollapseShadowsAlike(t *testing.T) {
	doc, styles := abCascade(t, `div{box-shadow:0 0 10px 6px rgba(200,0,0,0.5);border-radius:8px;`+
		`mask-image:url(/m.svg);mask-repeat:no-repeat;mask-size:50% 50%;mask-position:center;`+
		`border-style:solid;border-width:5px;border-image-source:url(/b.svg);border-image-slice:10}`)
	parent := abStyle(t, doc, styles)
	if len(parent.BoxShadow) == 0 || parent.BorderRadius[0][0] == 0 || parent.MaskImage == "" || parent.BorderImage.Source == "" {
		t.Fatalf("declaration did not reach the element: shadow=%d radius=%v mask=%q border-image=%q",
			len(parent.BoxShadow), parent.BorderRadius, parent.MaskImage, parent.BorderImage.Source)
	}
	block := style.AnonymousBlockStyle(parent)
	row := style.AnonymousTableStyle(parent, style.DisplayTableRow)

	if len(block.BoxShadow) != len(row.BoxShadow) ||
		block.BorderRadius != row.BorderRadius ||
		block.MaskImage != row.MaskImage || block.MaskRepeat != row.MaskRepeat ||
		block.MaskSize != row.MaskSize || block.MaskPosition != row.MaskPosition ||
		block.BorderImage != row.BorderImage {
		t.Errorf("anonymous builders disagree on the copied decoration: block shadow=%d radius=%v mask={%q %q %q %q} border-image=%q, table shadow=%d radius=%v mask={%q %q %q %q} border-image=%q",
			len(block.BoxShadow), block.BorderRadius, block.MaskImage, block.MaskRepeat, block.MaskSize, block.MaskPosition, block.BorderImage.Source,
			len(row.BoxShadow), row.BorderRadius, row.MaskImage, row.MaskRepeat, row.MaskSize, row.MaskPosition, row.BorderImage.Source)
	}
}
