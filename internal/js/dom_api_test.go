package js

import (
	"testing"
	"time"
)

func TestDatasetRead(t *testing.T) {
	rt := NewRuntime()
	rt.LoadHTML(`<html><body><div id="a" data-foo-bar="hello"></div></body></html>`)
	val, err := rt.RunScript(`document.getElementById("a").dataset.fooBar`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val.String() != "hello" {
		t.Errorf("expected 'hello', got %q", val.String())
	}
}

func TestStyleSetProperty(t *testing.T) {
	rt := NewRuntime()
	rt.LoadHTML(`<html><body><div id="a"></div></body></html>`)
	_, err := rt.RunScript(`
		var el = document.getElementById("a");
		el.style.setProperty("color", "red");
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClosest(t *testing.T) {
	rt := NewRuntime()
	rt.LoadHTML(`<html><body><div class="outer"><span id="inner">x</span></div></body></html>`)
	val, err := rt.RunScript(`document.getElementById("inner").closest(".outer") !== null`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val.String() != "true" {
		t.Errorf("expected 'true', got %q", val.String())
	}
}

func TestGetBoundingClientRect(t *testing.T) {
	rt := NewRuntime()
	rt.LoadHTML(`<html><body><div id="a"></div></body></html>`)
	val, err := rt.RunScript(`typeof document.getElementById("a").getBoundingClientRect().width !== "undefined"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val.String() != "true" {
		t.Errorf("expected 'true', got %q", val.String())
	}
}

func TestWindowInnerWidth(t *testing.T) {
	rt := NewRuntime()
	val, err := rt.RunScript(`typeof window.innerWidth === "number"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val.String() != "true" {
		t.Errorf("expected 'true', got %q", val.String())
	}
}

func TestMutationObserver(t *testing.T) {
	rt := NewRuntime()
	val, err := rt.RunScript(`typeof MutationObserver === "function"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val.String() != "true" {
		t.Errorf("expected 'true', got %q", val.String())
	}
}

func TestIntersectionObserverFires(t *testing.T) {
	rt := NewRuntime()
	rt.LoadHTML(`<html><body><div id="a"></div></body></html>`)
	val, err := rt.RunScript(`
		var fired = false;
		var io = new IntersectionObserver(function(entries) { fired = entries[0].isIntersecting; });
		io.observe(document.getElementById("a"));
		fired;
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val.String() != "true" {
		t.Errorf("expected 'true', got %q", val.String())
	}
}

// TestRequestAnimationFrame verifies that the runtime routes RAF
// callbacks through the FrameScheduler and that the callback only
// fires when the owner goroutine calls Tick. The previous polyfill
// fired the callback synchronously via queueMicrotask + immediate
// __flushMicrotasks, which collapsed animation loops into microtask
// recursion. The new contract: RAF is a deferred request; the test
// explicitly drives the scheduler.
func TestRequestAnimationFrame(t *testing.T) {
	rt := NewRuntime()
	if _, err := rt.RunScript(`
		var fired = false;
		requestAnimationFrame(function() { fired = true; });
	`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Without a Tick, the callback must not have fired yet. Verify
	// via the runtime's JS state.
	val, err := rt.RunScript(`fired`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val.String() != "false" {
		t.Fatalf("RAF callback should be deferred, got fired=%q", val.String())
	}

	// Drive the frame; the callback should now run.
	rt.FrameScheduler().Tick()

	val, err = rt.RunScript(`fired`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val.String() != "true" {
		t.Errorf("RAF callback should have fired after Tick, got fired=%q", val.String())
	}

	// Pending must be zero after a fired tick.
	if rt.FrameScheduler().Pending() != 0 {
		t.Errorf("Pending should be 0 after Tick, got %d", rt.FrameScheduler().Pending())
	}
}

func TestGetComputedStyle(t *testing.T) {
	rt := NewRuntime()
	rt.LoadHTML(`<html><body><div id="a" style="color: red;"></div></body></html>`)
	val, err := rt.RunScript(`window.getComputedStyle(document.getElementById("a")).getPropertyValue("color")`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val.String() != "red" {
		t.Errorf("expected 'red', got %q", val.String())
	}
}

func TestCloneNodeShallowAndDeep(t *testing.T) {
	rt := NewRuntime()
	rt.LoadHTML(`<html><body><div id="a" class="box" data-kind="source"><span>child</span></div></body></html>`)

	val, err := rt.RunScript(`
		var original = document.getElementById("a");
		var shallow = original.cloneNode(false);
		var deep = original.cloneNode(true);
		shallow.id === "a" &&
			shallow.className === "box" &&
			shallow.getAttribute("data-kind") === "source" &&
			shallow.childNodes.length === 0 &&
			deep.childNodes.length === 1 &&
			deep.childNodes[0].tagName.toLowerCase() === "span" &&
			deep.textContent === "child" &&
			deep.parentNode === null &&
			deep.childNodes[0].parentNode === deep;
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val.String() != "true" {
		t.Errorf("expected cloneNode checks to pass, got %q", val.String())
	}
}

func TestCloneNodeTextNode(t *testing.T) {
	rt := NewRuntime()
	val, err := rt.RunScript(`
		var text = document.createTextNode("hello");
		var clone = text.cloneNode();
		clone !== text && clone.nodeType === 3 && clone.textContent === "hello" && clone.parentNode === null;
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val.String() != "true" {
		t.Errorf("expected text cloneNode checks to pass, got %q", val.String())
	}
}

func TestCreateHTMLDocument(t *testing.T) {
	rt := NewRuntime()

	val, err := rt.RunScript(`
		var doc = document.implementation.createHTMLDocument("test title");
		doc.head.childNodes[0].tagName === "title" && doc.head.childNodes[0].textContent === "test title" && doc.nodeType === 9;
	`)

	if err != nil {
		t.Fatalf("Failed to run script: %v", err)
	}

	if !val.ToBoolean() {
		t.Errorf("createHTMLDocument did not create the expected structure")
	}
}

func TestWindowEventTarget(t *testing.T) {
	rt := NewRuntime()
	val, err := rt.RunScript(`
		var loadFired = false;
		var customDetail = null;
		var onceCount = 0;

		window.addEventListener("load", function(e) {
			loadFired = (e.type === "load" && e.target === window && e.currentTarget === window);
		});

		globalThis.addEventListener("custom", function(e) {
			customDetail = e.detail;
		});

		window.addEventListener("onceEvent", function() {
			onceCount++;
		}, { once: true });

		var ev = new Event("load");
		window.dispatchEvent(ev);

		globalThis.dispatchEvent(new CustomEvent("custom", { detail: { score: 99 } }));

		window.dispatchEvent(new Event("onceEvent"));
		window.dispatchEvent(new Event("onceEvent"));

		loadFired && customDetail && customDetail.score === 99 && onceCount === 1;
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !val.ToBoolean() {
		t.Errorf("expected Window EventTarget tests to pass")
	}
}

func TestGlobalScopeMirroring(t *testing.T) {
	rt := NewRuntime()
	// Step 1: Assign properties to window in an IIFE
	_, err := rt.RunScript(`
		(function(w) {
			w.ga = function(action, id) { return action + ":" + id; };
			w.$ = function(selector) { return "jQuery(" + selector + ")"; };
			w.jQuery = w.$;
			w.env = { mode: "production", version: "1.0.0" };
		})(window);
	`)
	if err != nil {
		t.Fatalf("script 1 failed: %v", err)
	}

	// Step 2: Access properties in subsequent script as bare global variables
	val, err := rt.RunScript(`
		var r1 = ga("create", "UA-12345");
		var r2 = $("#main");
		var r3 = jQuery.name || "jq";
		var r4 = env.mode;
		r1 === "create:UA-12345" && r2 === "jQuery(#main)" && r4 === "production";
	`)
	if err != nil {
		t.Fatalf("script 2 failed: %v", err)
	}
	if !val.ToBoolean() {
		t.Errorf("expected global scope mirroring to succeed")
	}
}

func TestEventDispatchTargetContext(t *testing.T) {
	rt := NewRuntime()
	rt.LoadHTML(`<html><body><button id="btn" class="active">Click</button></body></html>`)
	val, err := rt.RunScript(`
		var btn = document.getElementById("btn");
		var targetCorrect = false;
		var readyStateObserved = "";

		btn.addEventListener("click", function(event) {
			targetCorrect = (event.target === btn && event.currentTarget === btn);
		});
		btn.dispatchEvent(new Event("click", { bubbles: true }));

		document.addEventListener("readystatechange", function(event) {
			if (event.target && event.target.readyState) {
				readyStateObserved = event.target.readyState;
			}
		});
		document.dispatchEvent(new Event("readystatechange"));

		targetCorrect && readyStateObserved === "complete";
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !val.ToBoolean() {
		t.Errorf("expected Event dispatch target context test to pass")
	}
}

func TestElementHasAttributesAndAttributesIterator(t *testing.T) {
	rt := NewRuntime()
	rt.LoadHTML(`<html><body><div id="target" class="foo bar" data-name="test" title="hello"></div></body></html>`)
	val, err := rt.RunScript(`
		var target = document.getElementById("target");
		var empty = document.createElement("span");

		var hasAttrsTarget = target.hasAttributes();
		var hasAttrsEmpty = empty.hasAttributes();

		var collected = [];
		for (const attr of target.attributes) {
			collected.push(attr.name + "=" + attr.value);
		}

		var len = target.attributes.length;
		var item0 = target.attributes.item(0);
		var classAttr = target.attributes.getNamedItem("class");

		hasAttrsTarget === true &&
		hasAttrsEmpty === false &&
		len >= 4 &&
		collected.length === len &&
		item0 !== null && typeof item0.name === "string" &&
		classAttr !== null && classAttr.value === "foo bar";
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !val.ToBoolean() {
		t.Errorf("expected Element attributes and iterator test to pass")
	}
}

func TestMediaAndDocumentAPIStubs(t *testing.T) {
	rt := NewRuntime()
	rt.SetOrigin("https://example.com/sub/page.html?foo=bar#section1")
	val, err := rt.RunScript(`
		var audio = document.createElement("audio");
		var playPromise = audio.play();
		var isPromise = (playPromise && typeof playPromise.then === "function");
		audio.pause();
		audio.load();
		var canPlay = audio.canPlayType("audio/mpeg");

		var mediaPropsOk = (audio.paused === true && audio.volume === 1 && audio.muted === false && audio.readyState === 4);

		var docLocOk = (document.location === window.location &&
		                document.location.protocol === "https:" &&
		                document.location.hostname === "example.com" &&
		                document.location.pathname === "/sub/page.html");

		isPromise && canPlay === "probably" && mediaPropsOk && docLocOk;
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !val.ToBoolean() {
		t.Errorf("expected Media and Document API stubs test to pass")
	}
}

func TestTimerStringEvaluation(t *testing.T) {
	rt := NewRuntime()
	_, err := rt.RunScript(`
		window.__timerEvalRun = false;
		setTimeout("window.__timerEvalRun = true", 10);
	`)
	if err != nil {
		t.Fatalf("setTimeout failed: %v", err)
	}

	// Wait for timer to execute
	for i := 0; i < 20; i++ {
		time.Sleep(10 * time.Millisecond)
		val, _ := rt.RunScript(`window.__timerEvalRun`)
		if val != nil && val.ToBoolean() {
			return
		}
	}
	t.Errorf("expected timer string evaluation to set window.__timerEvalRun = true")
}

func TestJSExpandedQuerySelectors(t *testing.T) {
	rt := NewRuntime()
	rt.LoadHTML(`
		<html>
		<body>
			<div class="NavigationDrawer-header">
				<a href="/home" id="brand">Goosie</a>
			</div>
			<div class="article-toc">
				<a href="#intro" class="toc-link is-active">Intro</a>
				<a href="https://example.com" class="external">Ext</a>
			</div>
			<ul class="items">
				<li class="item primary active"><span class="label">One</span></li>
			</ul>
		</body>
		</html>
	`)

	val, err := rt.RunScript(`
		var brand = document.querySelector(".NavigationDrawer-header > a");
		var tocLink = document.querySelector(".article-toc a[href^='#']");
		var item = document.querySelector(".item.primary.active");

		var matchesChild = brand && brand.matches(".NavigationDrawer-header > a");
		var matchesDescendant = tocLink && tocLink.matches(".article-toc a[href^='#']");
		var closestHeader = brand && brand.closest(".NavigationDrawer-header") !== null;

		brand !== null && brand.id === "brand" &&
		tocLink !== null && tocLink.textContent === "Intro" &&
		item !== null &&
		matchesChild && matchesDescendant && closestHeader;
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !val.ToBoolean() {
		t.Errorf("expected expanded query selectors to match in JS DOM")
	}
}

