package style_test

import (
	"reflect"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/style"
)

// qc:BR-229
func TestAnonymousBlockStyleCarriesNoElementDecoration(t *testing.T) {
	for _, tc := range []struct {
		name     string
		css      string
		declared func(t *testing.T, el *style.ComputedStyle)
		cleared  func(t *testing.T, anon *style.ComputedStyle)
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
	if len(anon.TextShadow) == 0 {
		t.Error("anonymous block TextShadow is empty")
	}
}

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
