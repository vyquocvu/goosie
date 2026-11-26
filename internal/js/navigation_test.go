package js

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

// qc:BR-207
func TestLocationNavigationCloseClearsSink(t *testing.T) {
	doc, err := dom.ParseBounded(`<!doctype html><html><body></body></html>`, dom.ParseLimits{Nodes: 16, Depth: 8, Attributes: 8, AttributeBytes: 256})
	if err != nil {
		t.Fatal(err)
	}
	rt, err := New(Options{URL: "https://example.test/start", DOM: doc})
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	rt.SetNavigationSink(func(NavigationRequest) { requests++ })
	if err := rt.Run(`window.savedAssign = location.assign`, "inline"); err != nil {
		t.Fatal(err)
	}
	assign, ok := goja.AssertFunction(rt.vm.Get("savedAssign"))
	if !ok {
		t.Fatal("location.assign is not callable")
	}
	if err := rt.Close(); err != nil {
		t.Fatal(err)
	}
	_, _ = assign(goja.Undefined(), rt.vm.ToValue("/late"))
	if requests != 0 {
		t.Fatalf("closed runtime sent %d navigation requests", requests)
	}
}
