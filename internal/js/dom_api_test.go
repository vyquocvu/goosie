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

func TestRequestAnimationFrame(t *testing.T) {
	rt := NewRuntime()
	val, err := rt.RunScript(`
		var fired = false;
		requestAnimationFrame(function() { fired = true; });
		fired;
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val.String() != "true" {
		t.Errorf("expected 'true', got %q", val.String())
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
