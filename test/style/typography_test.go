package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
)

func TestTypographyProperties(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)

	d1 := doc.NewElement("div")
	d1.SetAttribute("id", "d1")
	body.AppendChild(d1)

	d2 := doc.NewElement("div")
	d2.SetAttribute("id", "d2")
	body.AppendChild(d2)

	d3 := doc.NewElement("div")
	d3.SetAttribute("id", "d3")
	body.AppendChild(d3)

	sheet := css.Parse(`
		#d1 {
			word-break: break-all;
			hyphens: auto;
			tab-size: 4;
		}
		#d2 {
			word-break: keep-all;
			hyphens: none;
			tab-size: 16px;
		}
		#d3 {
			-webkit-hyphens: manual;
		}
	`)

	styles := style.Resolve(doc, []*css.Stylesheet{sheet}, nil)

	s1 := styles[d1.ID]
	if s1.WordBreak != "break-all" {
		t.Errorf("d1 WordBreak = %q, want break-all", s1.WordBreak)
	}
	if s1.Hyphens != "auto" {
		t.Errorf("d1 Hyphens = %q, want auto", s1.Hyphens)
	}
	if s1.TabSize != 4 {
		t.Errorf("d1 TabSize = %v, want 4", s1.TabSize)
	}

	s2 := styles[d2.ID]
	if s2.WordBreak != "keep-all" {
		t.Errorf("d2 WordBreak = %q, want keep-all", s2.WordBreak)
	}
	if s2.Hyphens != "none" {
		t.Errorf("d2 Hyphens = %q, want none", s2.Hyphens)
	}
	if s2.TabSize != 16 {
		t.Errorf("d2 TabSize = %v, want 16", s2.TabSize)
	}

	s3 := styles[d3.ID]
	if s3.Hyphens != "manual" {
		t.Errorf("d3 Hyphens = %q, want manual", s3.Hyphens)
	}
}

func TestWritingModesAndFontVariantNumeric(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)

	parent := doc.NewElement("div")
	parent.SetAttribute("id", "parent")
	body.AppendChild(parent)

	child := doc.NewElement("span")
	child.SetAttribute("id", "child")
	parent.AppendChild(child)

	dCustom := doc.NewElement("div")
	dCustom.SetAttribute("id", "custom")
	body.AppendChild(dCustom)

	sheet := css.Parse(`
		#parent {
			writing-mode: vertical-rl;
			text-orientation: upright;
			font-variant-numeric: tabular-nums slashed-zero;
		}
		#custom {
			writing-mode: vertical-lr;
			text-orientation: sideways;
			font-variant-numeric: lining-nums;
		}
	`)

	styles := style.Resolve(doc, []*css.Stylesheet{sheet}, nil)

	sp := styles[parent.ID]
	if sp.WritingMode != "vertical-rl" {
		t.Errorf("parent WritingMode = %q, want vertical-rl", sp.WritingMode)
	}
	if sp.TextOrientation != "upright" {
		t.Errorf("parent TextOrientation = %q, want upright", sp.TextOrientation)
	}
	if sp.FontVariantNumeric != "tabular-nums slashed-zero" {
		t.Errorf("parent FontVariantNumeric = %q, want 'tabular-nums slashed-zero'", sp.FontVariantNumeric)
	}

	sc := styles[child.ID]
	if sc.WritingMode != "vertical-rl" {
		t.Errorf("child inherited WritingMode = %q, want vertical-rl", sc.WritingMode)
	}
	if sc.TextOrientation != "upright" {
		t.Errorf("child inherited TextOrientation = %q, want upright", sc.TextOrientation)
	}
	if sc.FontVariantNumeric != "tabular-nums slashed-zero" {
		t.Errorf("child inherited FontVariantNumeric = %q, want 'tabular-nums slashed-zero'", sc.FontVariantNumeric)
	}

	sCust := styles[dCustom.ID]
	if sCust.WritingMode != "vertical-lr" {
		t.Errorf("custom WritingMode = %q, want vertical-lr", sCust.WritingMode)
	}
	if sCust.TextOrientation != "sideways" {
		t.Errorf("custom TextOrientation = %q, want sideways", sCust.TextOrientation)
	}
	if sCust.FontVariantNumeric != "lining-nums" {
		t.Errorf("custom FontVariantNumeric = %q, want lining-nums", sCust.FontVariantNumeric)
	}
}

func TestMaskProperties(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)

	d1 := doc.NewElement("div")
	d1.SetAttribute("id", "m1")
	body.AppendChild(d1)

	d2 := doc.NewElement("div")
	d2.SetAttribute("id", "m2")
	body.AppendChild(d2)

	sheet := css.Parse(`
		#m1 {
			mask-image: url('mask.png');
			mask-repeat: no-repeat;
			mask-size: contain;
			mask-position: center;
		}
		#m2 {
			-webkit-mask-image: url('webkit-mask.png');
			-webkit-mask-repeat: repeat-x;
			-webkit-mask-size: cover;
			-webkit-mask-position: top left;
		}
	`)

	styles := style.Resolve(doc, []*css.Stylesheet{sheet}, nil)

	s1 := styles[d1.ID]
	if s1.MaskImage != "mask.png" {
		t.Errorf("m1 MaskImage = %q, want mask.png", s1.MaskImage)
	}
	if s1.MaskRepeat != "no-repeat" {
		t.Errorf("m1 MaskRepeat = %q, want no-repeat", s1.MaskRepeat)
	}
	if s1.MaskSize != "contain" {
		t.Errorf("m1 MaskSize = %q, want contain", s1.MaskSize)
	}
	if s1.MaskPosition != "center" {
		t.Errorf("m1 MaskPosition = %q, want center", s1.MaskPosition)
	}

	s2 := styles[d2.ID]
	if s2.MaskImage != "webkit-mask.png" {
		t.Errorf("m2 MaskImage = %q, want webkit-mask.png", s2.MaskImage)
	}
	if s2.MaskRepeat != "repeat-x" {
		t.Errorf("m2 MaskRepeat = %q, want repeat-x", s2.MaskRepeat)
	}
	if s2.MaskSize != "cover" {
		t.Errorf("m2 MaskSize = %q, want cover", s2.MaskSize)
	}
	if s2.MaskPosition != "top left" {
		t.Errorf("m2 MaskPosition = %q, want 'top left'", s2.MaskPosition)
	}
}
