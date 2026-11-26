package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
)

func resizeObserverRuntime(t *testing.T) (*js.Runtime, *dom.Node) {
	t.Helper()
	doc := observerTestDocument()
	target := doc.NewElement("div")
	doc.Body.AppendChild(target)

	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	t.Cleanup(func() { r.Close() })

	if err := r.Run(`
		var target = document.querySelector("div");
		globalThis.deliveries = [];
		var ro = new ResizeObserver(function(entries) {
			var out = [];
			for (var i = 0; i < entries.length; i++) {
				var rect = entries[i].contentRect;
				out.push(rect.width + "x" + rect.height);
			}
			globalThis.deliveries.push(out.join(","));
		});
		ro.observe(target);
	`, "resize-observer.js"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	return r, target
}

func resizeDeliveries(t *testing.T, r *js.Runtime) string {
	t.Helper()
	r.Tick()
	got, err := r.Evaluate(`JSON.stringify(globalThis.deliveries)`)
	if err != nil {
		t.Fatalf("read deliveries: %v", err)
	}
	return got
}

// qc:BR-117
func TestResizeObserverDeliversEachChangeOnce(t *testing.T) {
	r, target := resizeObserverRuntime(t)

	r.SetElementSize(target, 120, 40)
	r.DrainResizeObservers()
	if got, want := resizeDeliveries(t, r), `["120x40"]`; got != want {
		t.Fatalf("initial resize delivery = %s, want %s", got, want)
	}

	r.SetElementSize(target, 120, 40)
	r.DrainResizeObservers()
	if got, want := resizeDeliveries(t, r), `["120x40"]`; got != want {
		t.Fatalf("unchanged size was delivered again: %s, want %s", got, want)
	}

	r.SetElementSize(target, 200, 60)
	r.DrainResizeObservers()
	if got, want := resizeDeliveries(t, r), `["120x40","200x60"]`; got != want {
		t.Fatalf("changed size delivery = %s, want %s", got, want)
	}
}

// qc:BR-117
func TestResizeObserverUnobserveAndDisconnectStopDelivery(t *testing.T) {
	r, target := resizeObserverRuntime(t)

	r.SetElementSize(target, 120, 40)
	r.DrainResizeObservers()
	if got, want := resizeDeliveries(t, r), `["120x40"]`; got != want {
		t.Fatalf("initial resize delivery = %s, want %s", got, want)
	}

	if err := r.Run(`ro.unobserve(target);`, "unobserve.js"); err != nil {
		t.Fatalf("unobserve: %v", err)
	}
	r.SetElementSize(target, 200, 60)
	r.DrainResizeObservers()
	if got, want := resizeDeliveries(t, r), `["120x40"]`; got != want {
		t.Fatalf("unobserved target was delivered: %s, want %s", got, want)
	}

	if err := r.Run(`ro.observe(target);`, "reobserve.js"); err != nil {
		t.Fatalf("reobserve: %v", err)
	}
	r.SetElementSize(target, 200, 60)
	r.DrainResizeObservers()
	if got, want := resizeDeliveries(t, r), `["120x40","200x60"]`; got != want {
		t.Fatalf("re-observed target initial delivery = %s, want %s", got, want)
	}

	if err := r.Run(`ro.disconnect();`, "disconnect.js"); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	r.SetElementSize(target, 300, 80)
	r.DrainResizeObservers()
	if got, want := resizeDeliveries(t, r), `["120x40","200x60"]`; got != want {
		t.Fatalf("disconnected observer was delivered: %s, want %s", got, want)
	}
}

func TestResizeObserverReportsContentAndBorderBoxesSeparately(t *testing.T) {
	doc := observerTestDocument()
	target := doc.NewElement("div")
	doc.Body.AppendChild(target)
	r, err := js.New(js.Options{URL: "https://example.test/resize", DOM: doc})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	info := js.LayoutBoxInfo{
		Width: 100, Height: 50,
		BorderTop: 10, BorderRight: 10, BorderBottom: 10, BorderLeft: 10,
		PaddingTop: 5, PaddingRight: 5, PaddingBottom: 5, PaddingLeft: 5,
	}
	r.SetLayoutGeometryProvider(func(node *dom.Node) (js.LayoutBoxInfo, bool) {
		if node == target {
			return info, true
		}
		return js.LayoutBoxInfo{}, false
	})
	if err := r.Run(`
		globalThis.resizeShapes = [];
		new ResizeObserver(function(entries) {
			for (var i = 0; i < entries.length; i++) {
				var entry = entries[i];
				var border = entry.borderBoxSize[0];
				var content = entry.contentBoxSize[0];
				resizeShapes.push(JSON.stringify({
					contentRect: [entry.contentRect.x, entry.contentRect.y, entry.contentRect.width, entry.contentRect.height],
					borderBoxSize: [Array.isArray(entry.borderBoxSize), entry.borderBoxSize.length, border.inlineSize, border.blockSize, border.width === undefined],
					contentBoxSize: [Array.isArray(entry.contentBoxSize), entry.contentBoxSize.length, content.inlineSize, content.blockSize, content.width === undefined],
					deprecated: [typeof entry.inlineSize, typeof entry.blockSize]
				}));
			}
		}).observe(document.querySelector('div'));
	`, "resize-shapes.js"); err != nil {
		t.Fatal(err)
	}
	r.DrainResizeObservers()
	r.Tick()
	info.Width, info.Height = 140, 70
	r.DrainResizeObservers()
	r.Tick()
	got, err := r.Evaluate(`resizeShapes.join('|')`)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"contentRect":[5,5,70,20],"borderBoxSize":[true,1,100,50,true],"contentBoxSize":[true,1,70,20,true],"deprecated":["undefined","undefined"]}|` +
		`{"contentRect":[5,5,110,40],"borderBoxSize":[true,1,140,70,true],"contentBoxSize":[true,1,110,40,true],"deprecated":["undefined","undefined"]}`
	if got != want {
		t.Fatalf("ResizeObserver box shapes = %q, want %q", got, want)
	}
}

// qc:BR-117
func TestMutationObserverBatchesEachTurnIntoOneCallback(t *testing.T) {
	doc := observerTestDocument()
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	t.Cleanup(func() { r.Close() })

	if err := r.Run(`
		var target = document.createElement("div");
		document.body.appendChild(target);
		globalThis.callbackCount = 0;
		globalThis.totalRecords = 0;
		var mo = new MutationObserver(function(records) {
			globalThis.callbackCount++;
			globalThis.totalRecords += records.length;
		});
		mo.observe(target, { attributes: true });
		target.setAttribute("a", "1");
		target.setAttribute("b", "2");
		target.setAttribute("c", "3");
	`, "mutation-batch.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	count, _ := r.Global("callbackCount")
	records, _ := r.Global("totalRecords")
	if count.Num != 1 || records.Num != 3 {
		t.Fatalf("after one turn callbackCount=%v totalRecords=%v, want 1 and 3", count.Num, records.Num)
	}

	r.DrainMutationObservers()
	count, _ = r.Global("callbackCount")
	records, _ = r.Global("totalRecords")
	if count.Num != 1 || records.Num != 3 {
		t.Fatalf("extra drain re-delivered mutations: callbackCount=%v totalRecords=%v, want 1 and 3", count.Num, records.Num)
	}
}
