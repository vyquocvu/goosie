package main

import (
	"fmt"
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

// qc:BR-143
func TestJourneyResizeKeepsControlsReachable(t *testing.T) {
	srvURL := findJourneyPage(t, "<p>resize journey</p>", "")
	b := newBrowserE2E(t)
	b.navigateViaAddressBar(t, srvURL)
	b.waitFor(t, "page loaded", func() bool { return b.activeURL() == srvURL })

	for _, w := range []int32{120, 200, 320, 1280} {
		b.inject(t, surface.Event{Kind: surface.EvResize, Size: frame.Size{W: w, H: 600}, Scale: 1})
		b.waitFor(t, fmt.Sprintf("resize to %d applied", w), func() bool { return b.f.devSize().W == w })

		l := toolbar.ComputeLayout(w)
		if l.Address.X1 <= l.Address.X0 || l.Address.W() < toolbar.MinAddressWidth {
			t.Fatalf("width %d: address invalid %+v", w, l.Address)
		}
		if _, ok := l.Rects[toolbar.ButtonMenu]; !ok {
			t.Fatalf("width %d: menu missing from layout", w)
		}

		group, ok := findAXNode(b.window.AccessibilityTree(), "goosie.toolbar")
		if !ok {
			t.Fatalf("width %d: toolbar accessibility group missing", w)
		}
		var menu, address bool
		for i, a := range group.Children {
			if a.X0 < 0 || a.X1 > float32(w) {
				t.Fatalf("width %d: control %q out of bounds [%v,%v]", w, a.ID, a.X0, a.X1)
			}
			switch a.ID {
			case "goosie.menu":
				menu = true
			case "goosie.address":
				address = true
			}
			for _, c := range group.Children[i+1:] {
				if a.X0 < c.X1 && c.X0 < a.X1 && a.Y0 < c.Y1 && c.Y0 < a.Y1 {
					t.Fatalf("width %d: controls %q and %q overlap", w, a.ID, c.ID)
				}
			}
		}
		if !menu || !address {
			t.Fatalf("width %d: menu=%v address=%v, want both reachable", w, menu, address)
		}

		target := fmt.Sprintf("%s/width-%d", srvURL, w)
		b.navigateViaAddressBar(t, target)
		b.waitFor(t, fmt.Sprintf("navigated at width %d", w), func() bool { return b.activeURL() == target })
	}
}
