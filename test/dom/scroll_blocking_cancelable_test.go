package dom_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
)

// WPT dom/events/non-cancelable-when-passive, measured against Chromium: a
// generated scroll-blocking event reports cancelable only while the dispatch
// can reach a listener that may prevent the scroll. The answer is one value per
// dispatch, shared by every listener, not a value per listener context.
func TestGeneratedScrollBlockingEventCancelabilityFollowsItsListeners(t *testing.T) {
	e := dom.NewTrustedEvent("wheel", true, true)
	e.SetScrollCancelableByListener(false)
	if e.Cancelable() {
		t.Errorf("generated wheel with no non-passive listener: Cancelable() = true, want false")
	}
	e.SetScrollCancelableByListener(true)
	if !e.Cancelable() {
		t.Errorf("generated wheel with a non-passive listener: Cancelable() = false, want true")
	}

	// A type that blocks no scroll keeps its flag whichever way the listeners go,
	// which is what generic-events-stay-cancelable.html asserts.
	for _, passive := range []bool{false, true} {
		generic := dom.NewTrustedEvent("test", true, true)
		generic.SetScrollCancelableByListener(passive)
		if !generic.Cancelable() {
			t.Errorf("non-scroll-blocking type with a non-passive listener present = %t: Cancelable() = false, want true", passive)
		}
	}

	// An event the page constructed keeps its own flag; the listener state is
	// only consulted for generated events.
	for _, typ := range []string{"wheel", "mousewheel", "touchstart", "touchmove"} {
		synthetic := dom.NewEvent(typ, true, true)
		synthetic.SetScrollCancelableByListener(false)
		if !synthetic.Cancelable() {
			t.Errorf("page-constructed %s: Cancelable() = false, want true", typ)
		}
	}

	// A flag set at construction is never restored by listener state.
	created := dom.NewTrustedEvent("wheel", true, false)
	created.SetScrollCancelableByListener(true)
	if created.Cancelable() {
		t.Errorf("generated wheel constructed non-cancelable: Cancelable() = true, want false")
	}
}

// A passive listener still cannot cancel, whatever it reports about cancelable.
func TestPassiveListenerCannotPreventDefaultRegardlessOfListenerState(t *testing.T) {
	e := dom.NewTrustedEvent("wheel", true, true)
	e.SetScrollCancelableByListener(true)
	e.SetInPassiveListener(true)
	e.PreventDefault()
	if e.DefaultPrevented() {
		t.Errorf("preventDefault inside a passive listener took effect")
	}
	e.SetInPassiveListener(false)
	e.PreventDefault()
	if !e.DefaultPrevented() {
		t.Errorf("preventDefault inside a non-passive listener was ignored")
	}
}
