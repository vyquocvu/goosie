package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

func TestJourneyViewportResizeAndScrollReachMediaObserversAndClientRects(t *testing.T) {
	body := `<style>
		html, body { margin: 0; }
		body { height: 1200px; }
		#box { position: absolute; left: 10px; top: 20px; width: 50%; height: 50px; box-sizing: border-box; border: 10px solid; padding: 5px; }
		#partial { position: absolute; left: 0; top: 200px; width: 100px; height: 100px; }
	</style><div id="box"></div><div id="partial"></div><div id="viewport-state" role="status" aria-label=""></div>`
	script := `<script>
		globalThis.viewportLog = [];
		var state = document.getElementById('viewport-state');
		function record(value) {
			viewportLog.push(value);
			state.setAttribute('aria-label', viewportLog.join('|'));
			state.textContent = viewportLog.join('|');
		}
		matchMedia('(min-width: 350px)').addEventListener('change', function(event) {
			record('change:' + event.matches);
		});
		addEventListener('resize', function(event) {
			record('resize:' + innerWidth + 'x' + innerHeight + ':' + (event.target === window));
		});
		new ResizeObserver(function(entries) {
			for (var i = 0; i < entries.length; i++) {
				record('ro:' + entries[i].contentRect.width + 'x' + entries[i].contentRect.height);
			}
		}).observe(document.getElementById('box'));
		new IntersectionObserver(function(entries) {
			for (var i = 0; i < entries.length; i++) {
				var entry = entries[i];
				record('io:' + entry.intersectionRatio + ':' + entry.isIntersecting + ':' + entry.rootBounds.width + 'x' + entry.rootBounds.height);
			}
		}, { threshold: [0, 0.5, 1] }).observe(document.getElementById('partial'));
		addEventListener('scroll', function() {
			var box = document.getElementById('box');
			var rect = box.getBoundingClientRect();
			record('rect:' + JSON.stringify([rect.x, rect.y, box.offsetTop, scrollY, innerWidth, innerHeight]));
		});
	</script>`
	srvURL := findJourneyPage(t, body, script)
	b := newBrowserE2E(t)
	b.navigateViaAddressBar(t, srvURL)
	readLog := func() string {
		t.Helper()
		node, ok := findAXNode(b.window.AccessibilityTree(), "viewport-state")
		if !ok {
			return ""
		}
		return node.Label
	}
	b.waitFor(t, "initial observer entries", func() bool {
		want := fmt.Sprintf("io:1:true:640x%d", b.f.contentHeight())
		return strings.Contains(readLog(), "ro:290x20") && strings.Contains(readLog(), want)
	})
	cw := b.f.window.(*chromeWindow)
	resizeContent := func(width, height int32) {
		b.inject(t, surface.Event{Kind: surface.EvResize, Size: frame.Size{W: width, H: height + cw.chromeHeight()}, Scale: 1})
	}
	resizeContent(400, 300)
	b.waitFor(t, "resize reaches window", func() bool {
		return strings.Contains(readLog(), "resize:400x300:true")
	})
	b.waitFor(t, "resize reaches ResizeObserver", func() bool {
		return strings.Contains(readLog(), "ro:170x20")
	})
	resizeContent(320, 240)
	b.waitFor(t, "narrow resize reaches window and ResizeObserver", func() bool {
		log := readLog()
		return strings.Contains(log, "resize:320x240:true") && strings.Contains(log, "ro:130x20")
	})
	b.waitFor(t, "narrow resize crosses media and intersection thresholds", func() bool {
		log := readLog()
		return strings.Contains(log, "change:false") && strings.Contains(log, "io:0.4000000059604645:true:320x240")
	})
	b.pressKey(t, 0x1b)
	b.waitFor(t, "address bar blurred before scrolling", func() bool {
		node, ok := findAXNode(b.window.AccessibilityTree(), "goosie.address")
		return ok && !node.Focused
	})
	b.inject(t, surface.Event{
		Kind:  surface.EvScroll,
		Delta: frame.Point{Y: 20},
		Pos:   frame.Point{X: 300, Y: 200},
	})
	b.waitFor(t, "scroll updates intersection ratio and client rect", func() bool {
		log := readLog()
		tab := b.f.tabMgr.Active()
		return tab != nil && tab.ScrollY() == 20 && strings.Contains(log, "io:0.6000000238418579:true:320x240") &&
			strings.Contains(log, `rect:[10,0,20,20,320,240]`)
	})
}

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
