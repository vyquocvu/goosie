package js_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

type measuredDOMRect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
	Left   float64 `json:"left"`
}

type measuredIntersectionEntry struct {
	Name               string           `json:"name"`
	Ratio              float64          `json:"ratio"`
	IsIntersecting     bool             `json:"isIntersecting"`
	BoundingClientRect measuredDOMRect  `json:"boundingClientRect"`
	IntersectionRect   measuredDOMRect  `json:"intersectionRect"`
	RootBounds         *measuredDOMRect `json:"rootBounds"`
}

func readMeasuredIntersections(t *testing.T, r *js.Runtime) []measuredIntersectionEntry {
	t.Helper()
	value, err := r.Evaluate(`JSON.stringify(intersectionShapes)`)
	if err != nil {
		t.Fatal(err)
	}
	var entries []measuredIntersectionEntry
	if err := json.Unmarshal([]byte(value), &entries); err != nil {
		t.Fatalf("decode intersection entries: %v", err)
	}
	return entries
}

func assertMeasuredRect(t *testing.T, name string, got measuredDOMRect, want [8]float64) {
	t.Helper()
	values := []float64{got.X, got.Y, got.Width, got.Height, got.Top, got.Right, got.Bottom, got.Left}
	for i, value := range values {
		if math.Abs(value-want[i]) > 1e-6 {
			t.Errorf("%s rect field %d = %v, want %v", name, i, value, want[i])
		}
	}
}

func TestIntersectionObserverMatchesChromiumGeometryAndThresholds(t *testing.T) {
	doc := domtest.Parse("<html><body><div id='partial'></div><div id='off'></div><div id='edge'></div><div id='root'><div id='child'></div></div></body></html>")
	boxes := map[string]js.LayoutBoxInfo{
		"partial": {Y: 200, Width: 100, Height: 100},
		"off":     {Y: 400, Width: 100, Height: 100},
		"edge":    {Y: 240, Width: 100, Height: 100},
		"root": {
			X: 20, Y: 20, Width: 200, Height: 100,
			BorderTop: 10, BorderRight: 10, BorderBottom: 10, BorderLeft: 10,
			PaddingTop: 5, PaddingRight: 5, PaddingBottom: 5, PaddingLeft: 5,
		},
		"child": {X: 30, Y: 100, Width: 100, Height: 100},
	}
	r, err := js.New(js.Options{URL: "https://example.test/intersection", DOM: doc})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	r.SetLayoutGeometryProvider(func(node *dom.Node) (js.LayoutBoxInfo, bool) {
		info, ok := boxes[node.GetAttribute("id")]
		return info, ok
	})
	r.SetViewportRect(0, 0, 320, 240)
	if err := r.Run(`
		globalThis.intersectionShapes = [];
		globalThis.capture = function(name, target, options) {
			var observer = new IntersectionObserver(function(entries) {
				for (var i = 0; i < entries.length; i++) {
					var entry = entries[i];
					var rect = function(value) {
						return value && { x: value.x, y: value.y, width: value.width, height: value.height,
							top: value.top, right: value.right, bottom: value.bottom, left: value.left };
					};
					intersectionShapes.push({ name: name, ratio: entry.intersectionRatio,
						isIntersecting: entry.isIntersecting, boundingClientRect: rect(entry.boundingClientRect),
						intersectionRect: rect(entry.intersectionRect), rootBounds: rect(entry.rootBounds) });
				}
			}, options);
			observer.observe(target);
		};
		capture('viewport', document.getElementById('partial'), { threshold: [0, 0.5, 1] });
		capture('margin', document.getElementById('partial'), { rootMargin: '20px' });
		capture('off', document.getElementById('off'));
		capture('edge', document.getElementById('edge'));
		capture('root', document.getElementById('child'), { root: document.getElementById('root') });
		capture('rootMargin', document.getElementById('child'), { root: document.getElementById('root'), rootMargin: '10px' });
	`, "intersection-shapes.js"); err != nil {
		t.Fatal(err)
	}
	r.CheckIntersections()
	r.Tick()
	entries := readMeasuredIntersections(t, r)
	if len(entries) != 6 {
		t.Fatalf("initial intersection entries = %d, want 6", len(entries))
	}
	byName := make(map[string]measuredIntersectionEntry)
	for _, entry := range entries {
		byName[entry.Name] = entry
	}
	viewport := byName["viewport"]
	if math.Abs(viewport.Ratio-0.4) > 1e-6 || !viewport.IsIntersecting {
		t.Errorf("initial viewport ratio/intersecting = %v/%v, want 0.4/true", viewport.Ratio, viewport.IsIntersecting)
	}
	assertMeasuredRect(t, "viewport bounding", viewport.BoundingClientRect, [8]float64{0, 200, 100, 100, 200, 100, 300, 0})
	assertMeasuredRect(t, "viewport intersection", viewport.IntersectionRect, [8]float64{0, 200, 100, 40, 200, 100, 240, 0})
	assertMeasuredRect(t, "viewport root", *viewport.RootBounds, [8]float64{0, 0, 320, 240, 0, 320, 240, 0})
	margin := byName["margin"]
	if math.Abs(margin.Ratio-0.6) > 1e-6 {
		t.Errorf("rootMargin ratio = %v, want 0.6", margin.Ratio)
	}
	assertMeasuredRect(t, "margin root", *margin.RootBounds, [8]float64{-20, -20, 360, 280, -20, 340, 260, -20})
	off := byName["off"]
	if off.Ratio != 0 || off.IsIntersecting {
		t.Errorf("initial off-screen entry = %v/%v, want 0/false", off.Ratio, off.IsIntersecting)
	}
	assertMeasuredRect(t, "off intersection", off.IntersectionRect, [8]float64{})
	edge := byName["edge"]
	if edge.Ratio != 0 || !edge.IsIntersecting {
		t.Errorf("edge-adjacent entry = %v/%v, want 0/true", edge.Ratio, edge.IsIntersecting)
	}
	assertMeasuredRect(t, "edge intersection", edge.IntersectionRect, [8]float64{0, 240, 100, 0, 240, 100, 240, 0})
	root := byName["root"]
	if math.Abs(root.Ratio-0.2) > 1e-6 {
		t.Errorf("element root ratio = %v, want 0.2", root.Ratio)
	}
	assertMeasuredRect(t, "element root", *root.RootBounds, [8]float64{20, 20, 200, 100, 20, 220, 120, 20})
	rootMargin := byName["rootMargin"]
	if math.Abs(rootMargin.Ratio-0.3) > 1e-6 {
		t.Errorf("element rootMargin ratio = %v, want 0.3", rootMargin.Ratio)
	}
	assertMeasuredRect(t, "element margin root", *rootMargin.RootBounds, [8]float64{10, 10, 220, 120, 10, 230, 130, 10})

	r.SetWindowScrollY(20)
	r.Tick()
	entries = readMeasuredIntersections(t, r)
	var viewportEntries []measuredIntersectionEntry
	for _, entry := range entries {
		if entry.Name == "viewport" {
			viewportEntries = append(viewportEntries, entry)
		}
	}
	if len(viewportEntries) != 2 {
		t.Fatalf("viewport threshold entries = %d, want 2", len(viewportEntries))
	}
	scrolled := viewportEntries[1]
	if math.Abs(scrolled.Ratio-0.6) > 1e-6 || !scrolled.IsIntersecting {
		t.Errorf("scrolled ratio/intersecting = %v/%v, want 0.6/true", scrolled.Ratio, scrolled.IsIntersecting)
	}
	assertMeasuredRect(t, "scrolled bounding", scrolled.BoundingClientRect, [8]float64{0, 180, 100, 100, 180, 100, 280, 0})
	assertMeasuredRect(t, "scrolled intersection", scrolled.IntersectionRect, [8]float64{0, 180, 100, 60, 180, 100, 240, 0})
}
