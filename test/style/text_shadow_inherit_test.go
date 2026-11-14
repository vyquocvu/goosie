package style_test

import (
	"reflect"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

const tsProbe = `<html><head></head><body><div class="tsrc">` +
	`<div class="row"><span class="s1">declaring element own text</span></div>` +
	`<div class="row"><span class="s2">child span inherits</span></div>` +
	`<div class="row"><div><em class="s3">grandchild em inherits</em></div></div>` +
	`<div class="row"><span class="s4" style="text-shadow: none">author none wins</span></div>` +
	`<div class="row"><span class="s5" style="text-shadow: 1px 1px rgba(255,0,0,0.7)">own declaration wins</span></div>` +
	`<div class="row"><span class="s6" style="text-shadow: inherit">explicit inherit</span></div>` +
	`<div class="row"><span class="s7" style="text-shadow: initial">initial resets</span></div>` +
	`<div class="row"><span class="s8" style="text-shadow: unset">unset inherits</span></div>` +
	`<div class="row"><table><tbody><tr><td class="s9">table cell text</td></tr></tbody></table></div>` +
	`<div class="row"><div class="tbl"><div class="cell"><span class="s10">css table cell</span></div></div></div>` +
	`</div></body></html>`

const tsSheet = `.tsrc { text-shadow: 2px 3px 2px rgba(0, 0, 255, 0.7) }` +
	`.tbl { display: table } .cell { display: table-cell }`

func tsCascade(t *testing.T, html, sheet string) (*dom.Document, map[dom.NodeID]*style.ComputedStyle) {
	t.Helper()
	doc := domtest.Parse(html)
	st := style.Resolve(doc, []*css.Stylesheet{css.ParseForViewport(sheet, 800)}, nil)
	src := styleForClass(t, doc, st, "tsrc")
	if len(src.TextShadow) != 1 {
		t.Fatalf("declaring element layers = %d, want 1: the probe's author declaration must reach the element before its descendants are judged", len(src.TextShadow))
	}
	return doc, st
}

func sameAsSource(t *testing.T, cls string, got, want []css.TextShadow) bool {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf(".%s text-shadow = %+v, want the inherited %+v", cls, got, want)
		return false
	}
	if len(got) == 0 {
		t.Errorf(".%s text-shadow is empty and so is the source: the probe declares one layer", cls)
		return false
	}
	return true
}

// qc:BR-278
func TestTextShadowInheritsToDescendantSpans(t *testing.T) {
	doc, st := tsCascade(t, tsProbe, tsSheet)
	src := styleForClass(t, doc, st, "tsrc")

	for _, cls := range []string{"s1", "s2"} {
		cs := styleForClass(t, doc, st, cls)
		if !sameAsSource(t, cls, cs.TextShadow, src.TextShadow) {
			continue
		}
		sh := cs.TextShadow[0]
		if sh.OffsetX != 2 || sh.OffsetY != 3 || sh.Blur != 2 {
			t.Errorf(".%s offsets/blur = %v %v %v, want 2 3 2", cls, sh.OffsetX, sh.OffsetY, sh.Blur)
		}
		if sh.Color.R != 0 || sh.Color.G != 0 || sh.Color.B != 255 || sh.Color.A == 0 {
			t.Errorf(".%s colour = %+v, want blue at alpha .7 (rgba(0, 0, 255, 0.7))", cls, sh.Color)
		}
	}
}

// qc:BR-278
func TestTextShadowInheritsThroughSeveralLevels(t *testing.T) {
	doc, st := tsCascade(t, tsProbe, tsSheet)
	src := styleForClass(t, doc, st, "tsrc")
	cs := styleForClass(t, doc, st, "s3")
	sameAsSource(t, "s3", cs.TextShadow, src.TextShadow)
}

// qc:BR-278
func TestTextShadowOwnDeclarationAndNoneWinOverInheritance(t *testing.T) {
	doc, st := tsCascade(t, tsProbe, tsSheet)
	src := styleForClass(t, doc, st, "tsrc")

	none := styleForClass(t, doc, st, "s4")
	if len(none.TextShadow) != 0 {
		t.Errorf(".s4 text-shadow = %+v, want none: an author `text-shadow: none` beats the inherited value (Chromium changed 0 px there)", none.TextShadow)
	}

	own := styleForClass(t, doc, st, "s5")
	if len(own.TextShadow) != 1 {
		t.Fatalf(".s5 layers = %d, want exactly its own 1: %+v", len(own.TextShadow), own.TextShadow)
	}
	if reflect.DeepEqual(own.TextShadow, src.TextShadow) {
		t.Error(".s5 computed the ancestor's layer instead of its own declaration")
	}
	sh := own.TextShadow[0]
	if sh.OffsetX != 1 || sh.OffsetY != 1 || sh.Blur != 0 {
		t.Errorf(".s5 geometry = %v %v %v, want 1 1 0", sh.OffsetX, sh.OffsetY, sh.Blur)
	}
	if sh.Color.R != 255 || sh.Color.G != 0 || sh.Color.B != 0 || sh.Color.A == 0 {
		t.Errorf(".s5 colour = %+v, want red at alpha .7 (rgba(255, 0, 0, 0.7))", sh.Color)
	}
}

// qc:BR-278
func TestTextShadowCSSWideKeywordsOnAnInheritedProperty(t *testing.T) {
	doc, st := tsCascade(t, tsProbe, tsSheet)
	src := styleForClass(t, doc, st, "tsrc")

	sameAsSource(t, "s6", styleForClass(t, doc, st, "s6").TextShadow, src.TextShadow)
	sameAsSource(t, "s8", styleForClass(t, doc, st, "s8").TextShadow, src.TextShadow)

	init := styleForClass(t, doc, st, "s7")
	if len(init.TextShadow) != 0 {
		t.Errorf(".s7 text-shadow = %+v, want none: `initial` is the property's initial value, not its parent's (Chromium changed 0 px there)", init.TextShadow)
	}
}

// qc:BR-278
func TestTextShadowInheritsIntoTableCells(t *testing.T) {
	doc, st := tsCascade(t, tsProbe, tsSheet)
	src := styleForClass(t, doc, st, "tsrc")

	sameAsSource(t, "s9", styleForClass(t, doc, st, "s9").TextShadow, src.TextShadow)
	sameAsSource(t, "s10", styleForClass(t, doc, st, "s10").TextShadow, src.TextShadow)
}

// qc:BR-278
func TestTextShadowInheritanceKeepsCurrentColorUnresolved(t *testing.T) {
	doc := domtest.Parse(`<html><head></head><body><div class="cc">` +
		`<div class="row cg">green row keeps parent color</div>` +
		`<div class="row"><span class="cs1">green declaring element</span></div>` +
		`<div class="row"><span class="cs2 red">red child span</span></div>` +
		`</div></body></html>`)
	st := style.Resolve(doc, []*css.Stylesheet{css.ParseForViewport(
		`.cc { color: rgb(0,150,0); text-shadow: 4px 4px } .cs2 { color: rgb(200,0,0) }`, 800)}, nil)

	src := styleForClass(t, doc, st, "cc")
	if len(src.TextShadow) != 1 || !src.TextShadow[0].ColorIsCurrent {
		t.Fatalf("declaring element = %+v, want one currentcolor layer", src.TextShadow)
	}
	for _, cls := range []string{"cg", "cs1", "cs2"} {
		cs := styleForClass(t, doc, st, cls)
		if !sameAsSource(t, cls, cs.TextShadow, src.TextShadow) {
			continue
		}
		if !cs.TextShadow[0].ColorIsCurrent {
			t.Errorf(".%s lost the currentcolor flag: the inherited layer must resolve against the using element's colour", cls)
		}
	}
	if got := styleForClass(t, doc, st, "cs2").Color; got.R != 200 || got.G != 0 || got.B != 0 {
		t.Errorf(".cs2 colour = %+v, want rgb(200,0,0) so its shadow resolves red as Chromium measured", got)
	}
	if got := styleForClass(t, doc, st, "cg").Color; got.R != 0 || got.G != 150 || got.B != 0 {
		t.Errorf(".cg colour = %+v, want the inherited rgb(0,150,0)", got)
	}
}

// qc:BR-278
func TestAnonymousTableStyleKeepsInheritedTextShadow(t *testing.T) {
	doc, st := tsCascade(t, tsProbe, tsSheet)
	src := styleForClass(t, doc, st, "tsrc")

	for _, d := range []style.Display{style.DisplayTableRow, style.DisplayTableCell, style.DisplayTableRowGroup} {
		anon := style.AnonymousTableStyle(src, d)
		if !reflect.DeepEqual(anon.TextShadow, src.TextShadow) {
			t.Errorf("anonymous %v TextShadow = %+v, want the element's %+v: text-shadow inherits, so the generated table box holding the text carries it", d, anon.TextShadow, src.TextShadow)
		}
		if len(anon.BoxShadow) != 0 || anon.BackgroundColor.A != 0 || anon.BackgroundImage != "" {
			t.Errorf("anonymous %v grew box decoration: shadow=%d bg=%+v image=%q", d, len(anon.BoxShadow), anon.BackgroundColor, anon.BackgroundImage)
		}
	}
	if len(src.TextShadow) == 0 {
		t.Fatal("the source has no layers, so the loop above proves nothing")
	}
}
