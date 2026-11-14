package dom_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
)

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

	for _, passive := range []bool{false, true} {
		generic := dom.NewTrustedEvent("test", true, true)
		generic.SetScrollCancelableByListener(passive)
		if !generic.Cancelable() {
			t.Errorf("non-scroll-blocking type with a non-passive listener present = %t: Cancelable() = false, want true", passive)
		}
	}

	for _, typ := range []string{"wheel", "mousewheel", "touchstart", "touchmove"} {
		synthetic := dom.NewEvent(typ, true, true)
		synthetic.SetScrollCancelableByListener(false)
		if !synthetic.Cancelable() {
			t.Errorf("page-constructed %s: Cancelable() = false, want true", typ)
		}
	}

	created := dom.NewTrustedEvent("wheel", true, false)
	created.SetScrollCancelableByListener(true)
	if created.Cancelable() {
		t.Errorf("generated wheel constructed non-cancelable: Cancelable() = true, want false")
	}
}

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
