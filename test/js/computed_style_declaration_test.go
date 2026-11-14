package js_test

import "testing"

// qc:BR-205
func TestComputedStyleDeclarationExposesItsPropertyList(t *testing.T) {
	rt := styleRuntime(t, computedDoc, computedCascade)
	if err := rt.Run(`
		var cs = getComputedStyle(document.getElementById("art"));
		window.listShape = [
			typeof cs.length,
			typeof cs.item,
			String(cs.parentRule),
			String(cs.length),
			cs.item(0),
			cs.item(cs.length),
			cs.item(-1),
			String(cs[0] === cs.item(0))
		].join("|");
		window.listSlice = Array.prototype.slice.call(cs).join(",");
		window.listNames = (function () {
			var names = [];
			for (var i = 0; i < cs.length; i++) { names.push(cs.item(i)); }
			return names.join(",");
		})();
		var everyNameIsReadable = true;
		for (var i = 0; i < cs.length; i++) {
			var name = cs.item(i);
			if (cs[name] !== cs.getPropertyValue(name)) { everyNameIsReadable = false; }
		}
		window.listReadable = String(everyNameIsReadable);`, "inline"); err != nil {
		t.Fatal(err)
	}
	wantShape := "number|function|null|7|border-width|||true"
	if got := evalString(t, rt, `window.listShape`); got != wantShape {
		t.Fatalf("declaration list shape = %q, want %q", got, wantShape)
	}
	wantNames := "border-width,color,display,font-size,margin-left,text-align,width"
	if got := evalString(t, rt, `window.listSlice`); got != wantNames {
		t.Fatalf("slice of the declaration = %q, want %q", got, wantNames)
	}
	if got := evalString(t, rt, `window.listNames`); got != wantNames {
		t.Fatalf("item() names = %q, want %q", got, wantNames)
	}
	if got := evalString(t, rt, `window.listReadable`); got != "true" {
		t.Fatal("a listed property did not answer through its own getter")
	}
}

// qc:BR-205
func TestComputedStyleDeclarationListsOnlyResolvedNames(t *testing.T) {
	rt := styleRuntime(t, computedDoc, map[string]map[string]string{
		"plain": {"display": "block", "font-size": ""},
	})
	if got := evalString(t, rt, `
		var cs = getComputedStyle(document.getElementById("plain"));
		[String(cs.length), Array.prototype.slice.call(cs).join(","), cs.item(1)].join("|")`); got != "1|display|" {
		t.Fatalf("declaration of a partly resolved node = %q, want %q", got, "1|display|")
	}
	rt2 := styleRuntime(t, computedDoc, nil)
	if got := evalString(t, rt2, `
		var cs = getComputedStyle(document.getElementById("art"));
		[String(cs.length), Array.prototype.slice.call(cs).join(","), cs.getPropertyValue(cs.item(0))].join("|")`); got != "1|color|rgb(1, 2, 3)" {
		t.Fatalf("inline-only declaration = %q, want %q", got, "1|color|rgb(1, 2, 3)")
	}
}
