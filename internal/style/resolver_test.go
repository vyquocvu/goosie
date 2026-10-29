package style

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/test/domtest"
)

func TestResolverReusesCompiledIndexAcrossStyleAndPseudoPasses(t *testing.T) {
	doc := domtest.Parse(`<html><body><div class="target">hello</div></body></html>`)
	sheets := []*css.Stylesheet{css.Parse(`.target { color: red } .target::before { content: "x" }`)}
	resolver := NewResolver(sheets)
	compiledIndex := resolver.index

	styles := resolver.ResolveViewport(doc, Viewport{W: 800, H: 600}, nil)
	pseudos := resolver.ResolvePseudoElements(doc, styles, nil)
	if len(styles) == 0 || len(pseudos) == 0 {
		t.Fatalf("expected element and pseudo styles, got %d and %d", len(styles), len(pseudos))
	}

	resolver.ResolveViewport(doc, Viewport{W: 800, H: 600}, nil)
	if resolver.index != compiledIndex {
		t.Fatal("style resolution replaced the compiled selector index")
	}
}

func TestResolverAppliesDocumentPartRulesToShadowPartElements(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="host"></div></body></html>`)
	host := doc.ElementByID("host")
	host.ShadowRoot = &dom.ShadowRoot{Host: host, Mode: "open"}
	part := doc.NewElement("span")
	part.SetAttribute("part", "inner")
	part.CustomStates = map[string]bool{"innerFoo": true}
	host.ShadowRoot.AppendChild(part)

	sheet := css.Parse(`#host::part(inner) { opacity: 0 } #host::part(inner):state(innerFoo) { opacity: 0.5 } #host:state(outerFoo)::part(inner) { opacity: 0.25 }`)
	resolver := NewResolver([]*css.Stylesheet{sheet})
	styles := resolver.ResolveViewport(doc, Viewport{W: 800, H: 600}, nil)
	if got := styles[part.ID].Opacity; got != 0.5 {
		t.Fatalf("part opacity with part state = %v, want 0.5", got)
	}
	part.CustomStates = nil
	styles = resolver.ResolveViewport(doc, Viewport{W: 800, H: 600}, nil)
	if got := styles[part.ID].Opacity; got != 0 {
		t.Fatalf("part opacity without part state = %v, want 0", got)
	}
	host.CustomStates = map[string]bool{"outerFoo": true}
	styles = resolver.ResolveViewport(doc, Viewport{W: 800, H: 600}, nil)
	if got := styles[part.ID].Opacity; got != 0.25 {
		t.Fatalf("part opacity with host state = %v, want 0.25", got)
	}
}

func TestResolverIncrementallyResolvesLocalInteractionSubtree(t *testing.T) {
	css.ClearInteractiveState()
	t.Cleanup(css.ClearInteractiveState)
	doc := domtest.Parse(`<html><head><style>#target { color: black } #target:hover { color: red }</style></head><body><button id="target"><span>hover me</span></button><p>unrelated</p></body></html>`)
	resolver := NewResolver([]*css.Stylesheet{css.Parse(`#target { color: black } #target:hover { color: red }`)})
	viewport := Viewport{W: 800, H: 600}
	styles := resolver.ResolveViewport(doc, viewport, nil)
	pseudos := resolver.ResolvePseudoElements(doc, styles, nil)
	target := doc.ElementByID("target")
	if target == nil {
		t.Fatal("target is missing")
	}
	unrelated := doc.ElementByID("target").NextSibling
	if unrelated == nil {
		t.Fatal("unrelated sibling is missing")
	}
	oldUnrelatedStyle := styles[unrelated.ID]

	for current := target; current != nil && current.Element(); current = current.Parent {
		css.SetHover(current, true)
	}
	resolved, ok := resolver.ResolveInteractiveStates(doc, viewport, nil, styles, pseudos)
	if !ok {
		t.Fatal("simple local :hover selector should use incremental resolution")
	}
	if resolved.Visited >= len(styles) {
		t.Fatalf("incremental visits = %d, total styled nodes = %d", resolved.Visited, len(styles))
	}
	if got := styles[target.ID].Color; got.R != 255 || got.G != 0 || got.B != 0 {
		t.Fatalf("hover color = %+v, want red", got)
	}
	if styles[unrelated.ID] != oldUnrelatedStyle {
		t.Fatal("unrelated computed style was needlessly replaced")
	}

	for current := target; current != nil && current.Element(); current = current.Parent {
		css.SetHover(current, false)
	}
	resolved, ok = resolver.ResolveInteractiveStates(doc, viewport, nil, styles, pseudos)
	if !ok || styles[target.ID].Color.R != 0 {
		t.Fatalf("hover exit did not restore base style: incremental=%v style=%+v", ok, styles[target.ID])
	}
}

func TestResolverFallsBackForComplexInteractionSelectorsAndDOMChanges(t *testing.T) {
	css.ClearInteractiveState()
	t.Cleanup(css.ClearInteractiveState)
	viewport := Viewport{W: 800, H: 600}

	complexDoc := domtest.Parse(`<style>.parent:hover .child { color: red }</style><div class="parent"><span class="child">x</span></div>`)
	complexResolver := NewResolver([]*css.Stylesheet{css.Parse(`.parent:hover .child { color: red }`)})
	complexStyles := complexResolver.ResolveViewport(complexDoc, viewport, nil)
	complexPseudos := complexResolver.ResolvePseudoElements(complexDoc, complexStyles, nil)
	parent := complexDoc.ElementsByClassName("parent")[0]
	css.SetHover(parent, true)
	if _, ok := complexResolver.ResolveInteractiveStates(complexDoc, viewport, nil, complexStyles, complexPseudos); ok {
		t.Fatal("ancestor-dependent selector must use full-document fallback")
	}

	css.ClearInteractiveState()
	doc := domtest.Parse(`<style>#target:hover { color: red }</style><button id="target">x</button><p id="other">y</p>`)
	resolver := NewResolver([]*css.Stylesheet{css.Parse(`#target:hover { color: red }`)})
	styles := resolver.ResolveViewport(doc, viewport, nil)
	pseudos := resolver.ResolvePseudoElements(doc, styles, nil)
	target := doc.ElementByID("target")
	css.SetHover(target, true)
	doc.ElementByID("other").SetAttribute("class", "changed")
	if _, ok := resolver.ResolveInteractiveStates(doc, viewport, nil, styles, pseudos); ok {
		t.Fatal("DOM mutation must use full-document fallback even alongside hover change")
	}
}
