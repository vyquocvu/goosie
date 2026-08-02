package js

import (
	"testing"
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
