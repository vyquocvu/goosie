package dom_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
)

// WPT dom/events/non-cancelable-when-passive: a scroll-blocking event type
// reports cancelable as false only while a passive listener runs, and an event
// whose type is not scroll-blocking stays cancelable inside one.
func TestCancelableFalseOnlyInsidePassiveScrollBlockingListener(t *testing.T) {
	for _, typ := range []string{"wheel", "mousewheel", "touchstart", "touchmove"} {
		e := dom.NewTrustedEvent(typ, true, true)
		if !e.Cancelable() {
			t.Fatalf("%s: cancelable = false outside a listener, want true", typ)
		}
		e.SetInPassiveListener(true)
		if e.Cancelable() {
			t.Errorf("%s: cancelable = true inside a passive listener, want false", typ)
		}
		e.SetInPassiveListener(false)
		if !e.Cancelable() {
			t.Errorf("%s: cancelable = false after the passive listener returned, want true", typ)
		}
	}
	generic := dom.NewTrustedEvent("test", true, true)
	generic.SetInPassiveListener(true)
	if !generic.Cancelable() {
		t.Errorf("non-scroll-blocking type inside a passive listener: cancelable = false, want true")
	}
	createdUncancelable := dom.NewTrustedEvent("wheel", true, false)
	if createdUncancelable.Cancelable() {
		t.Errorf("wheel created with cancelable false: Cancelable() = true, want false")
	}
}
