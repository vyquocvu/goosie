package js_test

import (
	"fmt"
	"strings"
	"testing"
	"github.com/vyquocvu/goosie/internal/js"
)

// TestAdversarial_EventTarget_Suite rigorously exercises EventTarget lifecycle,
// listener registration/removal, dispatch order, event context integrity, bubbling,
// defaultPrevented, and exception containment.
func TestAdversarial_EventTarget_Suite(t *testing.T) {
	t.Run("MultipleListenersAndRegistrationOrder", func(t *testing.T) {
		rt := js.NewRuntime()
		rt.LoadHTML(`<html><body><button id="btn">Click me</button></body></html>`)

		script := `
			const btn = document.getElementById("btn");
			const order = [];
			for (let i = 0; i < 50; i++) {
				const idx = i;
				btn.addEventListener("click", function() {
					order.push(idx);
				});
			}
			btn.dispatchEvent(new Event("click"));
			JSON.stringify(order);
		`
		val, err := rt.RunScript(script)
		if err != nil {
			t.Fatalf("unexpected script failure: %v", err)
		}
		expectedOrder := make([]string, 50)
		for i := 0; i < 50; i++ {
			expectedOrder[i] = fmt.Sprintf("%d", i)
		}
		expectedJSON := "[" + strings.Join(expectedOrder, ",") + "]"
		if val.String() != expectedJSON {
			t.Errorf("listener execution order mismatch: got %s, want %s", val.String(), expectedJSON)
		}
	})

	t.Run("ListenerRemovalEdgeCases", func(t *testing.T) {
		rt := js.NewRuntime()
		rt.LoadHTML(`<html><body><div id="target"></div></body></html>`)

		script := `
			const el = document.getElementById("target");
			const log = [];
			
			const fnA = () => log.push("A");
			const fnB = () => log.push("B");
			const fnC = () => log.push("C");
			
			el.addEventListener("custom", fnA);
			el.addEventListener("custom", fnB);
			el.addEventListener("custom", fnC);
			
			// Non-existent removals should be graceful no-ops
			el.removeEventListener("custom", () => {});
			el.removeEventListener("nonexistent", fnA);
			el.removeEventListener(null, null);
			el.removeEventListener(undefined, undefined);
			
			// Remove fnB
			el.removeEventListener("custom", fnB);
			
			el.dispatchEvent(new Event("custom"));
			
			// Remove fnA, fnC and dispatch again
			el.removeEventListener("custom", fnA);
			el.removeEventListener("custom", fnC);
			el.dispatchEvent(new Event("custom"));
			
			JSON.stringify(log);
		`
		val, err := rt.RunScript(script)
		if err != nil {
			t.Fatalf("unexpected script failure: %v", err)
		}
		if val.String() != `["A","C"]` {
			t.Errorf("expected [\"A\",\"C\"], got %s", val.String())
		}
	})

	t.Run("OnceListenerLifecycleAndRemoval", func(t *testing.T) {
		rt := js.NewRuntime()
		rt.LoadHTML(`<html><body><div id="target"></div></body></html>`)

		script := `
			const el = document.getElementById("target");
			const log = [];
			
			const fnOnce1 = () => log.push("once1");
			const fnOnce2 = () => log.push("once2");
			const fnRegular = () => log.push("regular");
			
			el.addEventListener("test", fnOnce1, { once: true });
			el.addEventListener("test", fnOnce2, { once: true });
			el.addEventListener("test", fnRegular);
			
			// Unregister fnOnce2 before it gets dispatched
			el.removeEventListener("test", fnOnce2);
			
			el.dispatchEvent(new Event("test")); // fires fnOnce1, fnRegular
			el.dispatchEvent(new Event("test")); // fires fnRegular only
			
			JSON.stringify(log);
		`
		val, err := rt.RunScript(script)
		if err != nil {
			t.Fatalf("unexpected script failure: %v", err)
		}
		if val.String() != `["once1","regular","regular"]` {
			t.Errorf("expected [\"once1\",\"regular\",\"regular\"], got %s", val.String())
		}
	})

	t.Run("BubblingTargetAndCurrentTargetPreservation", func(t *testing.T) {
		rt := js.NewRuntime()
		rt.LoadHTML(`<html><body><div id="grandparent"><div id="parent"><button id="child">Action</button></div></div></body></html>`)

		script := `
			const grandparent = document.getElementById("grandparent");
			const parent = document.getElementById("parent");
			const child = document.getElementById("child");
			
			const traces = [];
			
			function makeListener(name) {
				return function(e) {
					traces.push({
						stage: name,
						targetId: e.target ? e.target.id : null,
						currentTargetId: e.currentTarget ? e.currentTarget.id : null,
						detail: e.detail || null
					});
				};
			}
			
			child.addEventListener("act", makeListener("child"));
			parent.addEventListener("act", makeListener("parent"));
			grandparent.addEventListener("act", makeListener("grandparent"));
			
			const customEvt = new CustomEvent("act", { bubbles: true, detail: { actionId: 42 } });
			child.dispatchEvent(customEvt);
			
			JSON.stringify(traces);
		`
		val, err := rt.RunScript(script)
		if err != nil {
			t.Fatalf("unexpected script failure: %v", err)
		}

		expected := `[{"stage":"child","targetId":"child","currentTargetId":"child","detail":{"actionId":42}},{"stage":"parent","targetId":"child","currentTargetId":"parent","detail":{"actionId":42}},{"stage":"grandparent","targetId":"child","currentTargetId":"grandparent","detail":{"actionId":42}}]`
		if val.String() != expected {
			t.Errorf("bubbling trace mismatch:\ngot:  %s\nwant: %s", val.String(), expected)
		}
	})

	t.Run("NonBubblingEventsDoNotTraverseParent", func(t *testing.T) {
		rt := js.NewRuntime()
		rt.LoadHTML(`<html><body><div id="parent"><button id="child"></button></div></body></html>`)

		script := `
			const parent = document.getElementById("parent");
			const child = document.getElementById("child");
			const log = [];
			
			parent.addEventListener("nobubble", () => log.push("parent_heard"));
			child.addEventListener("nobubble", () => log.push("child_heard"));
			
			// bubbles is false by default
			child.dispatchEvent(new Event("nobubble", { bubbles: false }));
			
			JSON.stringify(log);
		`
		val, err := rt.RunScript(script)
		if err != nil {
			t.Fatalf("failed: %v", err)
		}
		if val.String() != `["child_heard"]` {
			t.Errorf("expected [\"child_heard\"], got %s", val.String())
		}
	})

	t.Run("PreventDefaultAndDefaultPrevented", func(t *testing.T) {
		rt := js.NewRuntime()
		rt.LoadHTML(`<html><body><button id="btn"></button></body></html>`)

		script := `
			const btn = document.getElementById("btn");
			let defaultPreventedInside = false;
			
			btn.addEventListener("submit", (e) => {
				e.preventDefault();
				defaultPreventedInside = e.defaultPrevented;
			});
			
			const evt = new Event("submit", { cancelable: true });
			const dispatchResult = btn.dispatchEvent(evt);
			
			JSON.stringify({
				defaultPreventedInside: defaultPreventedInside,
				defaultPreventedAfter: evt.defaultPrevented,
				dispatchResult: dispatchResult
			});
		`
		val, err := rt.RunScript(script)
		if err != nil {
			t.Fatalf("failed: %v", err)
		}
		expected := `{"defaultPreventedInside":true,"defaultPreventedAfter":true,"dispatchResult":false}`
		if val.String() != expected {
			t.Errorf("expected %s, got %s", expected, val.String())
		}
	})

	t.Run("StopPropagationAndStopImmediatePropagation", func(t *testing.T) {
		rt := js.NewRuntime()
		rt.LoadHTML(`<html><body><div id="parent"><button id="child"></button></div></body></html>`)

		script := `
			const parent = document.getElementById("parent");
			const child = document.getElementById("child");
			const log = [];
			
			// Test stopPropagation
			parent.addEventListener("click", () => log.push("parent_click"));
			child.addEventListener("click", (e) => {
				log.push("child_click_1");
				e.stopPropagation();
			});
			child.addEventListener("click", () => log.push("child_click_2"));
			
			child.dispatchEvent(new Event("click", { bubbles: true }));
			
			// Test stopImmediatePropagation
			child.addEventListener("immediate", (e) => {
				log.push("immediate_1");
				e.stopImmediatePropagation();
			});
			child.addEventListener("immediate", () => log.push("immediate_2"));
			child.dispatchEvent(new Event("immediate", { bubbles: true }));
			
			JSON.stringify(log);
		`
		val, err := rt.RunScript(script)
		if err != nil {
			t.Fatalf("unexpected script failure: %v", err)
		}
		if val.String() != `["child_click_1","child_click_2","immediate_1"]` {
			t.Errorf("expected [\"child_click_1\",\"child_click_2\",\"immediate_1\"], got %s", val.String())
		}
	})

	t.Run("ExceptionIsolationInListeners", func(t *testing.T) {
		rt := js.NewRuntime()
		rt.LoadHTML(`<html><body><div id="parent"><button id="child"></button></div></body></html>`)

		script := `
			const parent = document.getElementById("parent");
			const child = document.getElementById("child");
			const log = [];
			
			child.addEventListener("bomb", () => {
				log.push("child_1_throwing");
				throw new Error("listener 1 exploded");
			});
			child.addEventListener("bomb", () => {
				log.push("child_2_survived");
			});
			
			const objHandler = {
				handleEvent: function() {
					log.push("child_3_obj_throwing");
					throw new Error("object listener exploded");
				}
			};
			child.addEventListener("bomb", objHandler);
			
			child.addEventListener("bomb", () => {
				log.push("child_4_survived");
			});
			
			parent.addEventListener("bomb", () => {
				log.push("parent_survived_bubble");
			});
			
			child.dispatchEvent(new Event("bomb", { bubbles: true }));
			
			JSON.stringify(log);
		`
		val, err := rt.RunScript(script)
		if err != nil {
			t.Fatalf("unexpected script failure: %v", err)
		}
		expected := `["child_1_throwing","child_2_survived","child_3_obj_throwing","child_4_survived","parent_survived_bubble"]`
		if val.String() != expected {
			t.Errorf("expected %s, got %s", expected, val.String())
		}
	})

	t.Run("WindowEventTargetAndGlobalThisInterchangeability", func(t *testing.T) {
		rt := js.NewRuntime()
		script := `
			const log = [];
			window.addEventListener("win_event", (e) => log.push("window_" + e.detail));
			globalThis.addEventListener("win_event", (e) => log.push("globalThis_" + e.detail));
			
			globalThis.dispatchEvent(new CustomEvent("win_event", { detail: "alpha" }));
			window.dispatchEvent(new CustomEvent("win_event", { detail: "beta" }));
			
			JSON.stringify(log);
		`
		val, err := rt.RunScript(script)
		if err != nil {
			t.Fatalf("unexpected script failure: %v", err)
		}
		expected := `["window_alpha","globalThis_alpha","window_beta","globalThis_beta"]`
		if val.String() != expected {
			t.Errorf("expected %s, got %s", expected, val.String())
		}
	})
}

// TestAdversarial_GlobalScopeAndWindowSync verifies that global variables and window properties
// are bidirectional mirrors across eval, closures, IIFEs, and property definitions.
func TestAdversarial_GlobalScopeAndWindowSync(t *testing.T) {
	rt := js.NewRuntime()

	t.Run("WindowPropertyAccessibleAsGlobal", func(t *testing.T) {
		script := `
			window.injectedConfig = { apiKey: "secret_123", timeout: 5000 };
			(function() {
				return injectedConfig.apiKey + "_" + injectedConfig.timeout;
			})();
		`
		val, err := rt.RunScript(script)
		if err != nil {
			t.Fatalf("failed: %v", err)
		}
		if val.String() != "secret_123_5000" {
			t.Errorf("expected 'secret_123_5000', got %s", val.String())
		}
	})

	t.Run("GlobalDeclarationAccessibleOnWindow", func(t *testing.T) {
		script := `
			var topLevelVar = 98765;
			function topLevelFunction() { return 42; }
			(window.topLevelVar === 98765) && (window.topLevelFunction() === 42);
		`
		val, err := rt.RunScript(script)
		if err != nil {
			t.Fatalf("failed: %v", err)
		}
		if val.ToBoolean() != true {
			t.Errorf("expected true, got %v", val)
		}
	})

	t.Run("EvalAndClosureMutualSync", func(t *testing.T) {
		script := `
			eval("window.evalCreated = 'eval_success';");
			eval("var evalVarCreated = 'eval_var_success';");
			
			const res1 = evalCreated;
			const res2 = window.evalVarCreated;
			
			(function() {
				window.closureCreated = "closure_success";
			})();
			const res3 = closureCreated;
			
			[res1, res2, res3].join("|");
		`
		val, err := rt.RunScript(script)
		if err != nil {
			t.Fatalf("failed: %v", err)
		}
		if val.String() != "eval_success|eval_var_success|closure_success" {
			t.Errorf("expected 'eval_success|eval_var_success|closure_success', got %s", val.String())
		}
	})

	t.Run("GlobalSelfReferencesIdentity", func(t *testing.T) {
		script := `
			(window === globalThis) &&
			(window.window === window) &&
			(globalThis.window === window) &&
			(window.globalThis === window) &&
			(typeof window.addEventListener === 'function');
		`
		val, err := rt.RunScript(script)
		if err != nil {
			t.Fatalf("failed: %v", err)
		}
		if val.ToBoolean() != true {
			t.Errorf("identity check failed: %v", val)
		}
	})
}

// TestAdversarial_NamedNodeMapAndAttributes tests zero attributes, multi-attribute stress,
// iterative modification, and NamedNodeMap/Attr proxy contract conformance.
func TestAdversarial_NamedNodeMapAndAttributes(t *testing.T) {
	t.Run("ZeroAttributesElement", func(t *testing.T) {
		rt := js.NewRuntime()
		rt.LoadHTML(`<html><body><div></div></body></html>`)

		script := `
			const el = document.querySelector("div");
			const checks = {
				hasAttributes: el.hasAttributes(),
				length: el.attributes.length,
				itemZero: el.attributes.item(0),
				indexZeroIsUndefined: (el.attributes[0] === undefined),
				getNamedItem: el.attributes.getNamedItem("id"),
				forOfCount: 0
			};
			for (const attr of el.attributes) {
				checks.forOfCount++;
			}
			JSON.stringify(checks);
		`
		val, err := rt.RunScript(script)
		if err != nil {
			t.Fatalf("failed: %v", err)
		}
		expected := `{"hasAttributes":false,"length":0,"itemZero":null,"indexZeroIsUndefined":true,"getNamedItem":null,"forOfCount":0}`
		if val.String() != expected {
			t.Errorf("expected %s, got %s", expected, val.String())
		}
	})

	t.Run("MultipleAttributesStressAndIteration", func(t *testing.T) {
		rt := js.NewRuntime()
		var attrsBuilder strings.Builder
		for i := 1; i <= 20; i++ {
			attrsBuilder.WriteString(fmt.Sprintf(` data-key-%02d="val-%02d"`, i, i))
		}
		htmlDoc := fmt.Sprintf(`<html><body><div id="stress-elem" class="main-card bold"%s></div></body></html>`, attrsBuilder.String())
		rt.LoadHTML(htmlDoc)

		script := `
			const el = document.getElementById("stress-elem");
			const map = el.attributes;
			
			const results = {
				hasAttributes: el.hasAttributes(),
				length: map.length,
				names: [],
				allHaveOwner: true,
				itemMatchIndex: true,
				namedAccessWorks: map["id"] && map["id"].value === "stress-elem"
			};
			
			let i = 0;
			for (const attr of map) {
				results.names.push(attr.name);
				if (attr.ownerElement !== el) results.allHaveOwner = false;
				if (map.item(i).name !== attr.name) results.itemMatchIndex = false;
				if (map[i].name !== attr.name) results.itemMatchIndex = false;
				i++;
			}
			
			// Test Attr nodeValue and setNamedItem mutation
			const classAttr = map.getNamedItem("class");
			classAttr.nodeValue = "mutated-class";
			results.mutatedClassValue = el.getAttribute("class");
			
			map.setNamedItem(new Attr("data-custom", "custom-val", el));
			results.customVal = el.getAttribute("data-custom");
			
			JSON.stringify(results);
		`
		val, err := rt.RunScript(script)
		if err != nil {
			t.Fatalf("failed: %v", err)
		}

		// Length is 1 (id) + 1 (class) + 20 (data-key-01..20) = 22
		if !strings.Contains(val.String(), `"length":22`) {
			t.Errorf("expected length 22, got %s", val.String())
		}
		if !strings.Contains(val.String(), `"allHaveOwner":true`) {
			t.Errorf("expected allHaveOwner: true, got %s", val.String())
		}
		if !strings.Contains(val.String(), `"itemMatchIndex":true`) {
			t.Errorf("expected itemMatchIndex: true, got %s", val.String())
		}
		if !strings.Contains(val.String(), `"namedAccessWorks":true`) {
			t.Errorf("expected namedAccessWorks: true, got %s", val.String())
		}
		if !strings.Contains(val.String(), `"mutatedClassValue":"mutated-class"`) {
			t.Errorf("expected mutatedClassValue: mutated-class, got %s", val.String())
		}
		if !strings.Contains(val.String(), `"customVal":"custom-val"`) {
			t.Errorf("expected customVal: custom-val, got %s", val.String())
		}
	})

	t.Run("ModifyingAttributesDuringIterationAndRemoval", func(t *testing.T) {
		rt := js.NewRuntime()
		rt.LoadHTML(`<html><body><div id="test" a="1" b="2" c="3"></div></body></html>`)

		script := `
			const el = document.getElementById("test");
			const map = el.attributes;
			
			// Remove during iteration
			const visited = [];
			for (const attr of map) {
				visited.push(attr.name);
				if (attr.name === "b") {
					el.removeAttribute("c");
					el.setAttribute("d", "4");
				}
			}
			
			// removeNamedItem
			const removedA = map.removeNamedItem("a");
			let notFoundThrown = false;
			try {
				map.removeNamedItem("nonexistent");
			} catch (e) {
				notFoundThrown = true;
			}
			
			JSON.stringify({
				visitedCount: visited.length,
				removedAName: removedA ? removedA.name : null,
				notFoundThrown: notFoundThrown,
				finalHasA: el.hasAttribute("a"),
				finalHasD: el.hasAttribute("d")
			});
		`
		val, err := rt.RunScript(script)
		if err != nil {
			t.Fatalf("failed: %v", err)
		}
		if !strings.Contains(val.String(), `"removedAName":"a"`) || !strings.Contains(val.String(), `"notFoundThrown":true`) || !strings.Contains(val.String(), `"finalHasA":false`) || !strings.Contains(val.String(), `"finalHasD":true`) {
			t.Errorf("unexpected NamedNodeMap modification results: %s", val.String())
		}
	})
}

// TestAdversarial_MediaElement_PlayPromiseChains stresses HTMLMediaElement methods,
// Promise resolution, async/await pipelines, and canPlayType matrix.
func TestAdversarial_MediaElement_PlayPromiseChains(t *testing.T) {
	t.Run("PlayPromiseThenChainResolution", func(t *testing.T) {
		rt := js.NewRuntime()
		rt.LoadHTML(`<html><body><video id="vid" src="movie.mp4"></video></body></html>`)

		_, err := rt.RunScript(`
			var vid = document.getElementById("vid");
			var status = "pending";
			
			vid.play()
				.then(function() {
					status = "resolved_1";
					return "next_step";
				})
				.then(function(val) {
					status = "resolved_2_" + val;
				});
		`)
		if err != nil {
			t.Fatalf("failed: %v", err)
		}

		// Read variable after microtasks flush at end of script
		val, err := rt.RunScript(`status`)
		if err != nil {
			t.Fatalf("failed: %v", err)
		}
		if val.String() != "resolved_2_next_step" {
			t.Errorf("expected 'resolved_2_next_step', got %s", val.String())
		}
	})

	t.Run("PlayPromiseAllConcurrentResolution", func(t *testing.T) {
		rt := js.NewRuntime()
		rt.LoadHTML(`<html><body><video id="v1"></video><video id="v2"></video><audio id="a1"></audio></body></html>`)

		_, err := rt.RunScript(`
			var v1 = document.getElementById("v1");
			var v2 = document.getElementById("v2");
			var a1 = document.getElementById("a1");
			var allResult = "pending";
			
			Promise.all([v1.play(), v2.play(), a1.play()]).then(function() {
				allResult = "all_resolved_paused_states_" + [v1.paused, v2.paused, a1.paused].join("_");
			});
		`)
		if err != nil {
			t.Fatalf("failed: %v", err)
		}

		val, err := rt.RunScript(`allResult`)
		if err != nil {
			t.Fatalf("failed: %v", err)
		}
		if val.String() != "all_resolved_paused_states_false_false_false" {
			t.Errorf("expected 'all_resolved_paused_states_false_false_false', got %s", val.String())
		}
	})

	t.Run("MediaStateTransitionsAndCanPlayType", func(t *testing.T) {
		rt := js.NewRuntime()
		rt.LoadHTML(`<html><body><video id="vid" src="movie.mp4"></video></body></html>`)

		script := `
			const vid = document.getElementById("vid");
			vid.pause();
			const pausedAfterPause = vid.paused;
			vid.load();
			const readyState = vid.readyState;
			
			const mimeTests = [
				vid.canPlayType("audio/mpeg"),
				vid.canPlayType("audio/mp3"),
				vid.canPlayType("video/mp4"),
				vid.canPlayType("video/webm"),
				vid.canPlayType("application/octet-stream"),
				vid.canPlayType("")
			];
			
			JSON.stringify({
				pausedAfterPause: pausedAfterPause,
				readyState: readyState,
				mimeTests: mimeTests
			});
		`
		val, err := rt.RunScript(script)
		if err != nil {
			t.Fatalf("failed: %v", err)
		}
		expected := `{"pausedAfterPause":true,"readyState":4,"mimeTests":["probably","probably","probably","probably","maybe",""]}`
		if val.String() != expected {
			t.Errorf("expected %s, got %s", expected, val.String())
		}
	})
}

// TestAdversarial_QuerySelectors_StressAndRecovery tests deeply nested trees,
// complex combinator sequences, pseudo classes, attribute selectors, and malformed queries.
func TestAdversarial_QuerySelectors_StressAndRecovery(t *testing.T) {
	createDeepRuntime := func() *js.Runtime {
		rt := js.NewRuntime()
		var htmlDoc strings.Builder
		htmlDoc.WriteString(`<html><body><div id="root" class="container">`)
		for i := 1; i <= 15; i++ {
			htmlDoc.WriteString(fmt.Sprintf(`<div id="level-%d" class="level depth-%d" data-depth="%d">`, i, i, i))
		}
		htmlDoc.WriteString(`<span id="deep-leaf" class="target active" data-role="leaf" data-status="ready-prod" data-tags="red green blue">Deep Content</span>`)
		for i := 1; i <= 15; i++ {
			htmlDoc.WriteString(`</div>`)
		}
		htmlDoc.WriteString(`</div></body></html>`)
		rt.LoadHTML(htmlDoc.String())
		return rt
	}

	t.Run("DeeplyNestedSelectorEvaluation", func(t *testing.T) {
		rt := createDeepRuntime()
		script := `
			const sel = "div#root.container > div#level-1.level > div#level-2 div#level-5.depth-5 div#level-10 div#level-15 > span#deep-leaf.target.active";
			const found = document.querySelector(sel);
			found ? found.id : null;
		`
		val, err := rt.RunScript(script)
		if err != nil {
			t.Fatalf("failed: %v", err)
		}
		if val.String() != "deep-leaf" {
			t.Errorf("expected 'deep-leaf', got %s", val.String())
		}
	})

	t.Run("AttributeOperatorsMatrix", func(t *testing.T) {
		rt := createDeepRuntime()
		script := `
			const leaf = document.getElementById("deep-leaf");
			const checks = [
				leaf.matches('[data-role="leaf"]'),
				leaf.matches('[data-role^="lea"]'),
				leaf.matches('[data-role$="eaf"]'),
				leaf.matches('[data-role*="ea"]'),
				leaf.matches('[data-tags~="green"]'),
				leaf.matches('[data-status|="ready"]'),
				leaf.matches('[data-tags]'),
				leaf.matches('[nonexistent]')
			];
			JSON.stringify(checks);
		`
		val, err := rt.RunScript(script)
		if err != nil {
			t.Fatalf("failed: %v", err)
		}
		expected := `[true,true,true,true,true,true,true,false]`
		if val.String() != expected {
			t.Errorf("expected %s, got %s", expected, val.String())
		}
	})

	t.Run("ClosestTraversalFromDeepLeaf", func(t *testing.T) {
		rt := createDeepRuntime()
		script := `
			const leaf = document.getElementById("deep-leaf");
			const c1 = leaf.closest("span.target");
			const c2 = leaf.closest(".depth-10");
			const c3 = leaf.closest("#root");
			const c4 = leaf.closest("nonexistent");
			
			[c1 ? c1.id : "none", c2 ? c2.id : "none", c3 ? c3.id : "none", c4 ? c4.id : "none"].join(",");
		`
		val, err := rt.RunScript(script)
		if err != nil {
			t.Fatalf("failed: %v", err)
		}
		if val.String() != "deep-leaf,level-10,root,none" {
			t.Errorf("expected 'deep-leaf,level-10,root,none', got %s", val.String())
		}
	})

	t.Run("MalformedAndInvalidSelectorRecovery", func(t *testing.T) {
		rt := createDeepRuntime()
		script := `
			const malformedSelectors = [
				"",
				"   ",
				":::invalid",
				"[unclosed",
				"div > > > span",
				",,,",
				"#",
				".",
				"[attr=]"
			];
			
			const results = [];
			for (const sel of malformedSelectors) {
				try {
					const resQuery = document.querySelector(sel);
					const resAll = document.querySelectorAll(sel);
					const resMatches = document.body.matches(sel);
					results.push({ sel: sel, query: resQuery === null, allLen: resAll.length, matches: resMatches });
				} catch (e) {
					results.push({ sel: sel, error: e.message });
				}
			}
			JSON.stringify(results);
		`
		val, err := rt.RunScript(script)
		if err != nil {
			t.Fatalf("unexpected panic/unhandled error during malformed selector execution: %v", err)
		}
		if strings.Contains(val.String(), "error") {
			t.Logf("Malformed selector produced errors: %s", val.String())
		}
	})
}
