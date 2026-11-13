package style_test

import (
	"reflect"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

// GOOSIE-TEXTSHADOW-001 asks for the inheritance half of `text-shadow`: the
// property is inherited (css-text-decor-3 §3), but Goosie's
// style.inheritFromParent never carried TextShadow, so a declaration on an
// ancestor reached only the element that declared it.
//
// Every expectation below is read off one Chromium document, probe
// /tmp/is/probe9.html against /tmp/is/probe9base.html (the same tree with the
// two author declarations deleted), Chromium 131.0.6778.33 through Playwright
// at deviceScaleFactor 1, measured as the pixels the declaration changes inside
// each row's box plus a 6px margin. "Effect N" is that count; it is 0 exactly
// where Chromium ignores the ancestor's declaration.
//
//	getComputedStyle (declared on #t: `text-shadow: 2px 3px 2px rgba(0,0,255,.7)`):
//	  #s1 span child        rgba(0, 0, 255, 0.7) 2px 3px 2px   effect 1674
//	  #s2 span child        rgba(0, 0, 255, 0.7) 2px 3px 2px   effect 1141
//	  #s3 em grandchild     rgba(0, 0, 255, 0.7) 2px 3px 2px   effect 1454
//	  #s4 `text-shadow:none`                                     effect    0
//	  #s5 own 1px 1px        rgba(255, 0, 0, 0.7) 1px 1px 0px   effect    0
//	  #s6 `inherit`          rgba(0, 0, 255, 0.7) 2px 3px 2px   effect  927
//	  #s7 `initial`          none                                 effect    0
//	  #s8 `unset`            rgba(0, 0, 255, 0.7) 2px 3px 2px   effect  843
//	  #s9 <td>               rgba(0, 0, 255, 0.7) 2px 3px 2px   effect  838
//	  #s10 table-cell span   rgba(0, 0, 255, 0.7) 2px 3px 2px   effect  813
//	  #s11/#s12 anon blocks  rgba(0, 0, 255, 0.7) 2px 3px 2px   effect 789/992
//
// The currentcolor half was measured the same way on #cc
// (`color:rgb(0,150,0); text-shadow:4px 4px`), where the omitted colour is
// `currentcolor`: #cg/#cs1 compute `rgb(0, 150, 0) 4px 4px 0px` and the child
// span that restyles `color` to rgb(200,0,0) computes `rgb(200, 0, 0) 4px 4px
// 0px` — the layer travels unchanged, the colour settles per using element.
//
// #s1 and #s2 are structurally the same case (a span in a div in the declaring
// element); their counts differ only because their text lengths differ.

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

// sameAsSource asserts the computed layer list is exactly what the declaring
// element computes, which is what "it inherits" means. It reports whether the
// lists matched so callers can skip their component checks on a red tree
// instead of indexing a list the engine never built.
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

// TestTextShadowInheritsToDescendantSpans is the defect's headline: Chromium
// changed 1,674 px on #s1 and 1,141 on #s2 by the ancestor's declaration, while
// the child element computed `none` in Goosie (0 px of shadow ink on a child's
// text — TASK-053 §9.2 limit 6 measured the same shape as 0 px against
// Chromium's 460 in a smaller probe).
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

// TestTextShadowInheritsThroughSeveralLevels covers #s3, the em two levels
// below the declaring element: 1,454 px of effect in Chromium.
// qc:BR-278
func TestTextShadowInheritsThroughSeveralLevels(t *testing.T) {
	doc, st := tsCascade(t, tsProbe, tsSheet)
	src := styleForClass(t, doc, st, "tsrc")
	cs := styleForClass(t, doc, st, "s3")
	sameAsSource(t, "s3", cs.TextShadow, src.TextShadow)
}

// TestTextShadowOwnDeclarationAndNoneWinOverInheritance pins the two rows where
// inheritance must NOT reach: #s4 `text-shadow: none` (effect 0, computed
// `none`) and #s5's own `1px 1px rgba(255,0,0,.7)` (effect 0, computed
// `rgba(255, 0, 0, 0.7) 1px 1px 0px`). An over-eager fix that re-applies the
// parent after the cascade would light both of these up.
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

// TestTextShadowCSSWideKeywordsOnAnInheritedProperty measures the three
// keywords against the same probe: `inherit` (effect 927) and `unset` (effect
// 843) both yield the parent's layer list, because text-shadow is inherited;
// `initial` (effect 0) yields none. Goosie's parser already answers `none` for
// an unparseable value, which is right for `initial` and wrong for `unset`.
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

// TestTextShadowInheritsIntoTableCells covers #s9, a real <td> (effect 838),
// and #s10, a span inside a `display: table-cell` box (effect 813). These
// elements take the ordinary cascade, so the case is here to keep the fix
// honest about which table boxes are affected: real cells, not only generated
// ones — the generated wrappers are AnonymousTableStyle's half of the defect.
// qc:BR-278
func TestTextShadowInheritsIntoTableCells(t *testing.T) {
	doc, st := tsCascade(t, tsProbe, tsSheet)
	src := styleForClass(t, doc, st, "tsrc")

	sameAsSource(t, "s9", styleForClass(t, doc, st, "s9").TextShadow, src.TextShadow)
	sameAsSource(t, "s10", styleForClass(t, doc, st, "s10").TextShadow, src.TextShadow)
}

// TestTextShadowInheritanceKeepsCurrentColorUnresolved is the #cc subtree:
// `color:rgb(0,150,0); text-shadow:4px 4px` on the ancestor, `color:rgb(200,0,0)`
// on one child span. Chromium computes `rgb(0, 150, 0) 4px 4px 0px` on the
// rows that keep the parent colour (#cg 1,390 px, #cs1 972 px) and
// `rgb(200, 0, 0) 4px 4px 0px` on the restyled child (#cs2 542 px), so the
// inherited layer must travel as an unresolved `currentcolor` and settle at
// paint against each element's own colour.
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

// TestAnonymousTableStyleKeepsInheritedTextShadow is the defect's second site.
// style.AnonymousTableStyle zeroes TextShadow while its sibling
// AnonymousBlockStyle deliberately keeps it (BR-230), and the two builders have
// the same contract: a whole-struct copy of the generating element's inherited
// text properties, minus the box decoration nobody declared on the wrapper.
// text-shadow is not box decoration, so the table builder must not clear it.
// qc:BR-278
func TestAnonymousTableStyleKeepsInheritedTextShadow(t *testing.T) {
	doc, st := tsCascade(t, tsProbe, tsSheet)
	src := styleForClass(t, doc, st, "tsrc")

	for _, d := range []style.Display{style.DisplayTableRow, style.DisplayTableCell, style.DisplayTableRowGroup} {
		anon := style.AnonymousTableStyle(src, d)
		if !reflect.DeepEqual(anon.TextShadow, src.TextShadow) {
			t.Errorf("anonymous %v TextShadow = %+v, want the element's %+v: text-shadow inherits, so the generated table box holding the text carries it", d, anon.TextShadow, src.TextShadow)
		}
		// Decoration the wrapper still must not invent.
		if len(anon.BoxShadow) != 0 || anon.BackgroundColor.A != 0 || anon.BackgroundImage != "" {
			t.Errorf("anonymous %v grew box decoration: shadow=%d bg=%+v image=%q", d, len(anon.BoxShadow), anon.BackgroundColor, anon.BackgroundImage)
		}
	}
	if len(src.TextShadow) == 0 {
		t.Fatal("the source has no layers, so the loop above proves nothing")
	}
}
