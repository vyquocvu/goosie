package css_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
)

func TestModernNotSelectorMultipleArguments(t *testing.T) {
	sel := css.ParseSelector("div:not(.first, .second, [hidden])")

	elem1 := &dom.Node{Type: dom.NodeElement, Data: "div"}
	elem1.SetAttribute("class", "normal")
	if !sel.Matches(elem1) {
		t.Errorf("elem1 (.normal) should match div:not(.first, .second, [hidden])")
	}

	elem2 := &dom.Node{Type: dom.NodeElement, Data: "div"}
	elem2.SetAttribute("class", "first")
	if sel.Matches(elem2) {
		t.Errorf("elem2 (.first) should NOT match")
	}

	elem3 := &dom.Node{Type: dom.NodeElement, Data: "div"}
	elem3.SetAttribute("class", "second")
	if sel.Matches(elem3) {
		t.Errorf("elem3 (.second) should NOT match")
	}

	elem4 := &dom.Node{Type: dom.NodeElement, Data: "div"}
	elem4.SetAttribute("hidden", "")
	if sel.Matches(elem4) {
		t.Errorf("elem4 ([hidden]) should NOT match")
	}
}

func TestModernSelectorSpecificity(t *testing.T) {
	// :not(#id) should have specificity (1, 0, 0)
	notID := css.ParseSelector(":not(#id)")
	a, b, c := notID.Specificity()
	if a != 1 || b != 0 || c != 0 {
		t.Errorf(":not(#id) specificity = (%d, %d, %d), want (1, 0, 0)", a, b, c)
	}

	// :not(.cls, #id) should take the maximum argument specificity -> (1, 0, 0)
	notMulti := css.ParseSelector(":not(.cls, #id)")
	a, b, c = notMulti.Specificity()
	if a != 1 || b != 0 || c != 0 {
		t.Errorf(":not(.cls, #id) specificity = (%d, %d, %d), want (1, 0, 0)", a, b, c)
	}

	// :has(#id) should take argument specificity -> (1, 0, 0)
	hasID := css.ParseSelector(":has(#id)")
	a, b, c = hasID.Specificity()
	if a != 1 || b != 0 || c != 0 {
		t.Errorf(":has(#id) specificity = (%d, %d, %d), want (1, 0, 0)", a, b, c)
	}

	// :has(> .item, #main) should take max specificity -> (1, 0, 0)
	hasMulti := css.ParseSelector(":has(> .item, #main)")
	a, b, c = hasMulti.Specificity()
	if a != 1 || b != 0 || c != 0 {
		t.Errorf(":has(> .item, #main) specificity = (%d, %d, %d), want (1, 0, 0)", a, b, c)
	}

	// :where(#id, .class) must contribute (0, 0, 0)
	whereSel := css.ParseSelector(":where(#id, .class)")
	a, b, c = whereSel.Specificity()
	if a != 0 || b != 0 || c != 0 {
		t.Errorf(":where(#id, .class) specificity = (%d, %d, %d), want (0, 0, 0)", a, b, c)
	}

	// :is(.a, #b) takes max specificity -> (1, 0, 0)
	isSel := css.ParseSelector(":is(.a, #b)")
	a, b, c = isSel.Specificity()
	if a != 1 || b != 0 || c != 0 {
		t.Errorf(":is(.a, #b) specificity = (%d, %d, %d), want (1, 0, 0)", a, b, c)
	}
}

func TestIsAndWhereUseForgivingSelectorLists(t *testing.T) {
	selectors, err := css.ParseSelectorList(`div:is(:not-a-real-pseudo, .good)`)
	if err != nil {
		t.Fatalf("ParseSelectorList with one invalid :is() member: %v", err)
	}
	good := &dom.Node{Type: dom.NodeElement, Data: "div"}
	good.SetAttribute("class", "good")
	if !selectors[0].Matches(good) {
		t.Fatal("valid :is() member did not match after invalid member was discarded")
	}
	if !css.ParseSelector(`div:where(:not-a-real-pseudo, .good)`).Matches(good) {
		t.Fatal("single-selector parsing did not preserve the valid :where() member")
	}
	other := &dom.Node{Type: dom.NodeElement, Data: "div"}
	other.SetAttribute("class", "other")
	if selectors[0].Matches(other) {
		t.Fatal(":is() matched when none of its valid members matched")
	}

	// Invalid alternatives do not contribute specificity. The remaining ID
	// selector still supplies :is()'s maximum specificity.
	isSelector := css.ParseSelector(`div:is(#winner, :not-a-real-pseudo, .good)`)
	a, b, c := isSelector.Specificity()
	if a != 1 || b != 0 || c != 1 {
		t.Fatalf(":is() specificity = (%d, %d, %d), want (1, 0, 1)", a, b, c)
	}

	whereSelector := css.ParseSelector(`div:where(#winner, :not-a-real-pseudo, .good)`)
	a, b, c = whereSelector.Specificity()
	if a != 0 || b != 0 || c != 1 {
		t.Fatalf(":where() specificity = (%d, %d, %d), want (0, 0, 1)", a, b, c)
	}

	for _, empty := range []string{
		`div:is(:not-a-real-pseudo, ::before)`,
		`div:where()`,
	} {
		parsed, err := css.ParseSelectorList(empty)
		if err != nil {
			t.Errorf("ParseSelectorList(%q): %v; empty forgiving list is valid", empty, err)
			continue
		}
		if parsed[0].Matches(good) {
			t.Errorf("%q matched despite its forgiving list having no valid members", empty)
		}
	}
}

func TestAttributeCaseInsensitiveModifier(t *testing.T) {
	selInsensitive := css.ParseSelector(`[data-target="HelloWorld" i]`)
	selSensitive := css.ParseSelector(`[data-target="HelloWorld" s]`)
	selDefault := css.ParseSelector(`[data-target="HelloWorld"]`)

	nodeUpper := &dom.Node{Type: dom.NodeElement, Data: "div"}
	nodeUpper.SetAttribute("data-target", "HELLOWORLD")

	nodeExact := &dom.Node{Type: dom.NodeElement, Data: "div"}
	nodeExact.SetAttribute("data-target", "HelloWorld")

	if !selInsensitive.Matches(nodeUpper) {
		t.Errorf("selInsensitive should match uppercase value")
	}
	if !selInsensitive.Matches(nodeExact) {
		t.Errorf("selInsensitive should match exact value")
	}

	if selSensitive.Matches(nodeUpper) {
		t.Errorf("selSensitive should NOT match uppercase value")
	}
	if !selSensitive.Matches(nodeExact) {
		t.Errorf("selSensitive should match exact value")
	}

	if selDefault.Matches(nodeUpper) {
		t.Errorf("selDefault should NOT match uppercase value")
	}
	if !selDefault.Matches(nodeExact) {
		t.Errorf("selDefault should match exact value")
	}
}
