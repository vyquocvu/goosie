package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

// CSSOM View layout measurement APIs: getBoundingClientRect, getClientRects,
// offset properties, scroll properties, scrollIntoView, and window scroll
// methods. These tests verify the JS binding layer; the engine supplies the
// geometry through a LayoutGeometryProvider callback.

// layoutProvider installs a stub geometry provider for a js.Runtime, keyed by
// the element id the test markup gives the node.
func layoutProvider(rt *js.Runtime, byID map[string]js.LayoutBoxInfo) {
	rt.SetLayoutGeometryProvider(func(node *dom.Node) (js.LayoutBoxInfo, bool) {
		if node == nil {
			return js.LayoutBoxInfo{}, false
		}
		if id := node.GetAttribute("id"); id != "" {
			if info, ok := byID[id]; ok {
				return info, true
			}
		}
		return js.LayoutBoxInfo{}, false
	})
}

func offsetParentProvider(rt *js.Runtime, byID map[string]string) {
	rt.SetOffsetParentProvider(func(node *dom.Node) *dom.Node {
		if node == nil {
			return nil
		}
		id := node.GetAttribute("id")
		if parentID, ok := byID[id]; ok {
			// Find the parent node by ID in the document
			if node.Doc != nil {
				return node.Doc.ElementByID(parentID)
			}
		}
		return nil
	})
}

func layoutRuntime(t *testing.T, html string, byID map[string]js.LayoutBoxInfo) *js.Runtime {
	t.Helper()
	doc := domtest.Parse(html)
	rt := newRuntime(t, js.Options{URL: "https://example.test/page", DOM: doc})
	if byID != nil {
		layoutProvider(rt, byID)
	}
	return rt
}

const layoutDoc = `<html><head></head><body>
<div id="box" style="width: 100px; height: 50px;">content</div>
<p id="para">text</p>
</body></html>`

var layoutGeometry = map[string]js.LayoutBoxInfo{
	"box": {
		X: 10, Y: 20, Width: 100, Height: 50,
		BorderTop: 1, BorderRight: 1, BorderBottom: 1, BorderLeft: 1,
		PaddingTop: 5, PaddingRight: 5, PaddingBottom: 5, PaddingLeft: 5,
		MarginTop: 10, MarginRight: 10, MarginBottom: 10, MarginLeft: 10,
		ScrollWidth: 200, ScrollHeight: 200, ScrollContainer: true,
	},
	"para": {
		X: 10, Y: 90, Width: 200, Height: 30,
		ScrollWidth: 200, ScrollHeight: 30,
	},
}

// ---------------------------------------------------------------------------
// getBoundingClientRect
// ---------------------------------------------------------------------------

func TestGetBoundingClientRectReturnsDOMRect(t *testing.T) {
	rt := layoutRuntime(t, layoutDoc, layoutGeometry)
	result, err := rt.Evaluate(`
		var el = document.getElementById('box');
		var rect = el.getBoundingClientRect();
		rect.x + ',' + rect.y + ',' + rect.width + ',' + rect.height;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result != "10,20,100,50" {
		t.Errorf("getBoundingClientRect: got %q, want %q", result, "10,20,100,50")
	}
}

func TestGetBoundingClientRectEdges(t *testing.T) {
	rt := layoutRuntime(t, layoutDoc, layoutGeometry)
	result, err := rt.Evaluate(`
		var el = document.getElementById('box');
		var rect = el.getBoundingClientRect();
		rect.top + ',' + rect.right + ',' + rect.bottom + ',' + rect.left;
	`)
	if err != nil {
		t.Fatal(err)
	}
	// top=min(y,y+h)=20, right=max(x,x+w)=110, bottom=max(y,y+h)=70, left=min(x,x+w)=10
	if result != "20,110,70,10" {
		t.Errorf("getBoundingClientRect edges: got %q, want %q", result, "20,110,70,10")
	}
}

func TestGetBoundingClientRectNoLayout(t *testing.T) {
	// Without a layout provider, getBoundingClientRect returns zeros.
	doc := domtest.Parse(layoutDoc)
	rt := newRuntime(t, js.Options{URL: "https://example.test/page", DOM: doc})
	result, err := rt.Evaluate(`
		var el = document.getElementById('box');
		var rect = el.getBoundingClientRect();
		rect.x + ',' + rect.y + ',' + rect.width + ',' + rect.height;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result != "0,0,0,0" {
		t.Errorf("no-layout getBoundingClientRect: got %q, want %q", result, "0,0,0,0")
	}
}

func TestGetBoundingClientRectDetached(t *testing.T) {
	rt := layoutRuntime(t, layoutDoc, layoutGeometry)
	result, err := rt.Evaluate(`
		var el = document.createElement('div');
		var rect = el.getBoundingClientRect();
		rect.x + ',' + rect.y + ',' + rect.width + ',' + rect.height;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result != "0,0,0,0" {
		t.Errorf("detached getBoundingClientRect: got %q, want %q", result, "0,0,0,0")
	}
}

// ---------------------------------------------------------------------------
// getClientRects
// ---------------------------------------------------------------------------

func TestGetClientRectsReturnsList(t *testing.T) {
	rt := layoutRuntime(t, layoutDoc, layoutGeometry)
	result, err := rt.Evaluate(`
		var el = document.getElementById('box');
		var rects = el.getClientRects();
		rects.length + ',' + rects[0].width;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result != "1,100" {
		t.Errorf("getClientRects: got %q, want %q", result, "1,100")
	}
}

func TestGetClientRectsNoLayout(t *testing.T) {
	doc := domtest.Parse(layoutDoc)
	rt := newRuntime(t, js.Options{URL: "https://example.test/page", DOM: doc})
	result, err := rt.Evaluate(`
		var el = document.getElementById('box');
		var rects = el.getClientRects();
		rects.length;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result != "0" {
		t.Errorf("no-layout getClientRects: got %q, want %q", result, "0")
	}
}

// ---------------------------------------------------------------------------
// DOMRect constructor
// ---------------------------------------------------------------------------

func TestDOMRectConstructor(t *testing.T) {
	rt := layoutRuntime(t, layoutDoc, nil)
	result, err := rt.Evaluate(`
		var r = new DOMRect(1, 2, 3, 4);
		r.x + ',' + r.y + ',' + r.width + ',' + r.height + ',' +
		r.top + ',' + r.right + ',' + r.bottom + ',' + r.left;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result != "1,2,3,4,2,4,6,1" {
		t.Errorf("DOMRect constructor: got %q, want %q", result, "1,2,3,4,2,4,6,1")
	}
}

func TestDOMRectExists(t *testing.T) {
	rt := layoutRuntime(t, layoutDoc, nil)
	result, err := rt.Evaluate(`typeof DOMRect`)
	if err != nil {
		t.Fatal(err)
	}
	if result != "function" {
		t.Errorf("DOMRect type: got %q, want %q", result, "function")
	}
}

// ---------------------------------------------------------------------------
// offsetWidth / offsetHeight
// ---------------------------------------------------------------------------

func TestOffsetWidthHeight(t *testing.T) {
	rt := layoutRuntime(t, layoutDoc, layoutGeometry)
	result, err := rt.Evaluate(`
		var el = document.getElementById('box');
		el.offsetWidth + ',' + el.offsetHeight;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result != "100,50" {
		t.Errorf("offsetWidth/Height: got %q, want %q", result, "100,50")
	}
}

func TestOffsetWidthHeightNoLayout(t *testing.T) {
	doc := domtest.Parse(layoutDoc)
	rt := newRuntime(t, js.Options{URL: "https://example.test/page", DOM: doc})
	result, err := rt.Evaluate(`
		var el = document.getElementById('box');
		el.offsetWidth + ',' + el.offsetHeight;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result != "0,0" {
		t.Errorf("no-layout offsetWidth/Height: got %q, want %q", result, "0,0")
	}
}

// ---------------------------------------------------------------------------
// offsetTop / offsetLeft
// ---------------------------------------------------------------------------

func TestOffsetTopLeft(t *testing.T) {
	rt := layoutRuntime(t, layoutDoc, layoutGeometry)
	result, err := rt.Evaluate(`
		var el = document.getElementById('box');
		el.offsetTop + ',' + el.offsetLeft;
	`)
	if err != nil {
		t.Fatal(err)
	}
	// Without an offsetParent provider, these are the absolute position.
	if result != "20,10" {
		t.Errorf("offsetTop/Left: got %q, want %q", result, "20,10")
	}
}

// ---------------------------------------------------------------------------
// offsetParent
// ---------------------------------------------------------------------------

func TestOffsetParentNull(t *testing.T) {
	rt := layoutRuntime(t, layoutDoc, layoutGeometry)
	// Without an offset parent provider, offsetParent is null.
	result, err := rt.Evaluate(`
		var el = document.getElementById('box');
		el.offsetParent;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result != "null" {
		t.Errorf("offsetParent without provider: got %q, want %q", result, "null")
	}
}

func TestOffsetParentWithProvider(t *testing.T) {
	doc := domtest.Parse(`<html><head></head><body>
	<div id="parent" style="position: relative;">
		<div id="child">content</div>
	</div>
	</body></html>`)
	rt := newRuntime(t, js.Options{URL: "https://example.test/page", DOM: doc})
	layoutProvider(rt, map[string]js.LayoutBoxInfo{
		"parent": {X: 0, Y: 0, Width: 200, Height: 200},
		"child":  {X: 10, Y: 10, Width: 100, Height: 50},
	})
	offsetParentProvider(rt, map[string]string{
		"child": "parent",
	})
	result, err := rt.Evaluate(`
		var el = document.getElementById('child');
		el.offsetParent !== null && el.offsetParent.id === 'parent';
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result != "true" {
		t.Errorf("offsetParent: got %q, want %q", result, "true")
	}
}

// ---------------------------------------------------------------------------
// scrollTop / scrollLeft
// ---------------------------------------------------------------------------

func TestScrollTopDefault(t *testing.T) {
	rt := layoutRuntime(t, layoutDoc, layoutGeometry)
	result, err := rt.Evaluate(`
		var el = document.getElementById('box');
		el.scrollTop;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result != "0" {
		t.Errorf("scrollTop default: got %q, want %q", result, "0")
	}
}

func TestScrollTopSetGet(t *testing.T) {
	rt := layoutRuntime(t, layoutDoc, layoutGeometry)
	result, err := rt.Evaluate(`
		var el = document.getElementById('box');
		el.scrollTop = 100;
		el.scrollTop;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result != "100" {
		t.Errorf("scrollTop after set: got %q, want %q", result, "100")
	}
}

func TestScrollLeftSetGet(t *testing.T) {
	rt := layoutRuntime(t, layoutDoc, layoutGeometry)
	result, err := rt.Evaluate(`
		var el = document.getElementById('box');
		el.scrollLeft = 50;
		el.scrollLeft;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result != "50" {
		t.Errorf("scrollLeft after set: got %q, want %q", result, "50")
	}
}

func TestScrollTopNegativeClamped(t *testing.T) {
	rt := layoutRuntime(t, layoutDoc, layoutGeometry)
	result, err := rt.Evaluate(`
		var el = document.getElementById('box');
		el.scrollTop = -10;
		el.scrollTop;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result != "0" {
		t.Errorf("scrollTop negative clamp: got %q, want %q", result, "0")
	}
}

func TestDocumentScrollingElementSharesWindowScrollOffset(t *testing.T) {
	rt := layoutRuntime(t, `<!doctype html><html><body><div id="box">content</div></body></html>`, nil)
	result, err := rt.Evaluate(`
		var root = document.scrollingElement;
		var isRoot = root === document.documentElement;
		root.scrollTop = 123;
		var fromWindow = window.scrollY;
		window.scrollTo(0, 45);
		isRoot + ',' + fromWindow + ',' + root.scrollTop;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result != "true,123,45" {
		t.Errorf("document scrolling element state = %q, want %q", result, "true,123,45")
	}
}

// ---------------------------------------------------------------------------
// scrollWidth / scrollHeight
// ---------------------------------------------------------------------------

func TestScrollWidthHeight(t *testing.T) {
	rt := layoutRuntime(t, layoutDoc, layoutGeometry)
	result, err := rt.Evaluate(`
		var el = document.getElementById('box');
		el.scrollWidth + ',' + el.scrollHeight;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result != "200,200" {
		t.Errorf("scrollWidth/Height: got %q, want %q", result, "200,200")
	}
}

// The travel an element accepts is its scrollable area minus its client box, so
// the fixture's 200x200 extent over a 98x48 client box stops at 102 and 152.
func TestScrollTopClampsToScrollableArea(t *testing.T) {
	rt := layoutRuntime(t, layoutDoc, layoutGeometry)
	result, err := rt.Evaluate(`
		var el = document.getElementById('box');
		el.scrollTop = 1000;
		el.scrollLeft = 1000;
		el.scrollTop + ',' + el.scrollLeft;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result != "152,102" {
		t.Errorf("clamped offsets: got %q, want %q", result, "152,102")
	}
}

// A box whose content is not clipped or scrolled has nothing to scroll, so the
// write is ignored even where its extent is larger than its client box.
func TestScrollTopIgnoredOnNonScrollContainer(t *testing.T) {
	rt := layoutRuntime(t, layoutDoc, layoutGeometry)
	result, err := rt.Evaluate(`
		var el = document.getElementById('para');
		el.scrollTop = 50;
		el.scrollLeft = 50;
		el.scrollTop + ',' + el.scrollLeft + ',' + el.scrollHeight;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result != "0,0,30" {
		t.Errorf("offsets on a non-scroll container: got %q, want %q", result, "0,0,30")
	}
}

// ---------------------------------------------------------------------------
// clientWidth / clientHeight
// ---------------------------------------------------------------------------

func TestClientWidthHeight(t *testing.T) {
	rt := layoutRuntime(t, layoutDoc, layoutGeometry)
	result, err := rt.Evaluate(`
		var el = document.getElementById('box');
		el.clientWidth + ',' + el.clientHeight;
	`)
	if err != nil {
		t.Fatal(err)
	}
	// clientWidth = border-box width - border-left - border-right = 100 - 1 - 1 = 98
	// clientHeight = border-box height - border-top - border-bottom = 50 - 1 - 1 = 48
	if result != "98,48" {
		t.Errorf("clientWidth/Height: got %q, want %q", result, "98,48")
	}
}

// ---------------------------------------------------------------------------
// scrollIntoView
// ---------------------------------------------------------------------------

func TestScrollIntoViewDoesNotThrow(t *testing.T) {
	rt := layoutRuntime(t, layoutDoc, layoutGeometry)
	_, err := rt.Evaluate(`
		var el = document.getElementById('box');
		el.scrollIntoView();
		'ok';
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestScrollIntoViewWithArg(t *testing.T) {
	rt := layoutRuntime(t, layoutDoc, layoutGeometry)
	_, err := rt.Evaluate(`
		var el = document.getElementById('box');
		el.scrollIntoView(true);
		el.scrollIntoView({block: 'center'});
		'ok';
	`)
	if err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// window.scroll / scrollTo / scrollBy
// ---------------------------------------------------------------------------

func TestWindowScrollTo(t *testing.T) {
	rt := layoutRuntime(t, layoutDoc, nil)
	result, err := rt.Evaluate(`
		window.scrollTo(100, 200);
		window.scrollX + ',' + window.scrollY;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result != "100,200" {
		t.Errorf("window.scrollTo: got %q, want %q", result, "100,200")
	}
}

func TestWindowScrollBy(t *testing.T) {
	rt := layoutRuntime(t, layoutDoc, nil)
	result, err := rt.Evaluate(`
		window.scrollTo(50, 50);
		window.scrollBy(10, 20);
		window.scrollX + ',' + window.scrollY;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result != "60,70" {
		t.Errorf("window.scrollBy: got %q, want %q", result, "60,70")
	}
}

func TestWindowScrollWithOptionsObject(t *testing.T) {
	rt := layoutRuntime(t, layoutDoc, nil)
	result, err := rt.Evaluate(`
		window.scrollTo({top: 30, left: 40});
		window.scrollX + ',' + window.scrollY;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result != "40,30" {
		t.Errorf("window.scrollTo options: got %q, want %q", result, "40,30")
	}
}

func TestWindowScrollAliases(t *testing.T) {
	rt := layoutRuntime(t, layoutDoc, nil)
	result, err := rt.Evaluate(`
		window.scroll(10, 20);
		var a = window.pageXOffset + ',' + window.pageYOffset;
		window.scrollTo(0, 0);
		var b = window.pageXOffset + ',' + window.pageYOffset;
		a + ';' + b;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result != "10,20;0,0" {
		t.Errorf("window scroll aliases: got %q, want %q", result, "10,20;0,0")
	}
}

func TestWindowScrollNegativeClamped(t *testing.T) {
	rt := layoutRuntime(t, layoutDoc, nil)
	result, err := rt.Evaluate(`
		window.scrollTo(-10, -20);
		window.scrollX + ',' + window.scrollY;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result != "0,0" {
		t.Errorf("window scroll negative clamp: got %q, want %q", result, "0,0")
	}
}

// ---------------------------------------------------------------------------
// element.scroll / scrollTo / scrollBy
// ---------------------------------------------------------------------------

func TestElementScrollTo(t *testing.T) {
	rt := layoutRuntime(t, layoutDoc, layoutGeometry)
	result, err := rt.Evaluate(`
		var el = document.getElementById('box');
		el.scrollTo(30, 40);
		el.scrollLeft + ',' + el.scrollTop;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result != "30,40" {
		t.Errorf("element.scrollTo: got %q, want %q", result, "30,40")
	}
}

func TestElementScrollBy(t *testing.T) {
	rt := layoutRuntime(t, layoutDoc, layoutGeometry)
	result, err := rt.Evaluate(`
		var el = document.getElementById('box');
		el.scrollTo(10, 20);
		el.scrollBy(5, 10);
		el.scrollLeft + ',' + el.scrollTop;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result != "15,30" {
		t.Errorf("element.scrollBy: got %q, want %q", result, "15,30")
	}
}

// ---------------------------------------------------------------------------
// DOMRectList.item()
// ---------------------------------------------------------------------------

func TestDOMRectListItem(t *testing.T) {
	rt := layoutRuntime(t, layoutDoc, layoutGeometry)
	result, err := rt.Evaluate(`
		var el = document.getElementById('box');
		var rects = el.getClientRects();
		var r = rects.item(0);
		r !== null && r.width === 100;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result != "true" {
		t.Errorf("DOMRectList.item(0): got %q, want %q", result, "true")
	}
}

func TestDOMRectListItemOutOfRange(t *testing.T) {
	rt := layoutRuntime(t, layoutDoc, layoutGeometry)
	result, err := rt.Evaluate(`
		var el = document.getElementById('box');
		var rects = el.getClientRects();
		rects.item(5);
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result != "null" {
		t.Errorf("DOMRectList.item(5): got %q, want %q", result, "null")
	}
}
