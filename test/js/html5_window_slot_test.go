package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

func TestWindowNamedPropertiesResolveLiveIDsAndAllowGlobalShadowing(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="named"></div></body></html>`)
	rt, err := js.New(js.Options{URL: "https://example.test/", DOM: doc})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	if err := rt.Run(`
		var initial = named === document.getElementById("named");
		var replacement = document.createElement("p");
		replacement.id = "replaceMe";
		document.body.append(replacement);
		var live = replaceMe === replacement;
		var overwritten = false;
		window.named = "script binding";
		overwritten = named === "script binding";
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]bool{"initial": true, "live": true, "overwritten": true} {
		value, ok := rt.Global(key)
		if !ok || value.Bool != want {
			t.Errorf("%s = %v (present %t), want %t", key, value, ok, want)
		}
	}
}

func TestShadowRootInnerHTMLSelectorsAndManualSlotAssignment(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="host"><span id="light">اختبر</span></div></body></html>`)
	rt, err := js.New(js.Options{URL: "https://example.test/", DOM: doc})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	if err := rt.Run(`
		var host = document.getElementById("host");
		var light = document.getElementById("light");
		var shadow = host.attachShadow({mode:"open", slotAssignment:"manual"});
		shadow.innerHTML = '<slot dir="auto"></slot>';
		var slot = shadow.querySelector("slot");
		var found = slot !== null && shadow.querySelectorAll("slot").length === 1;
		var before = slot.matches(":dir(ltr)");
		slot.assign(light);
		var assigned = slot.assignedNodes().length === 1 && slot.assignedNodes()[0] === light;
		var rtl = slot.matches(":dir(rtl)");
		slot.assign();
		var cleared = slot.assignedNodes().length === 0 && slot.matches(":dir(ltr)");
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]bool{"found": true, "before": true, "assigned": true, "rtl": true, "cleared": true} {
		value, ok := rt.Global(key)
		if !ok || value.Bool != want {
			t.Errorf("%s = %v (present %t), want %t", key, value, ok, want)
		}
	}
}

func TestTemplateContentIsAQueryableDocumentFragment(t *testing.T) {
	doc := domtest.Parse(`<html><body><template id="template"><section class="inside"><span id="from-template">text</span></section></template><span id="outside"></span></body></html>`)
	rt, err := js.New(js.Options{URL: "https://example.test/", DOM: doc})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	if err := rt.Run(`
		var fragment = document.getElementById("template").content;
		var isFragment = fragment instanceof DocumentFragment;
		var first = fragment.querySelector("section > span");
		var all = fragment.querySelectorAll("span");
		var scoped = first !== null && first.id === "from-template" && first.textContent === "text";
		var count = all.length;
		var isolated = fragment.querySelector("#outside") === null;
		var textNodeHasNoQuery = typeof first.firstChild.querySelector === "undefined";
		var created = document.createElement("template");
		created.innerHTML = "<head><title>title</title></head><body><p id='inside-html'>body</p></body><frameset><frame></frameset>";
		var titleNode = created.content.querySelector("title");
		var paragraphNode = created.content.querySelector("#inside-html");
		var fragmentParsing = created.children.length === 0 &&
			titleNode !== null && titleNode.textContent === "title" &&
			paragraphNode !== null && paragraphNode.textContent === "body" &&
			created.content.querySelector("head, body, frameset, frame") === null;
		var innerHTMLReadsContent = created.innerHTML.indexOf("<title>") >= 0;
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]bool{
		"isFragment":            true,
		"scoped":                true,
		"isolated":              true,
		"textNodeHasNoQuery":    true,
		"fragmentParsing":       true,
		"innerHTMLReadsContent": true,
	} {
		value, ok := rt.Global(key)
		if !ok || value.Bool != want {
			t.Errorf("%s = %v (present %t), want %t", key, value, ok, want)
		}
	}
	if value, ok := rt.Global("count"); !ok || value.Num != 1 {
		t.Errorf("querySelectorAll span count = %v (present %t), want 1", value, ok)
	}
}
