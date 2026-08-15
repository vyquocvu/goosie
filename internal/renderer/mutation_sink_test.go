package renderer

import (
	"context"
	"image/color"
	"reflect"
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
)

func TestMutationSinkRoutesAttributeMutationToRenderer(t *testing.T) {
	r := NewRenderer(800, 600)
	_, err := r.RenderHTML(context.Background(), `<html><body><div id="target" __goosie_id="100">before</div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	lookup := NewNodeIDLookup()
	lookup.Snapshot(r.GetRoot())
	presents := 0
	sink := NewMutationSink(r, lookup, func() { presents++ })
	batch := []js.DOMMutation{{
		Kind:      js.MutationSetAttribute,
		TargetID:  "100",
		Attribute: "data-state",
		NewValue:  "ready",
	}}
	sink.Handle(batch)
	if !r.IsDirty() {
		t.Fatal("renderer should be marked dirty after mutation")
	}
	if presents != 1 {
		t.Fatalf("present calls = %d, want 1", presents)
	}
}

func TestMutationSinkSyncsTextAndAttributeValues(t *testing.T) {
	r := NewRenderer(800, 600)
	_, err := r.RenderHTML(context.Background(), `<html><body><div id="target" __goosie_id="100" class="a">before</div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	lookup := NewNodeIDLookup()
	lookup.Snapshot(r.GetRoot())
	sink := NewMutationSink(r, lookup, nil)

	// set-text on an element target updates its first text child.
	sink.Handle([]js.DOMMutation{{
		Kind:     js.MutationSetText,
		TargetID: "100",
		NewValue: "after",
	}})
	target := findRenderNodeByIDRoot(r.GetRoot(), lookup.Lookup("100"))
	if target == nil {
		t.Fatal("target render node not found")
	}
	found := false
	for _, c := range target.Children {
		if c.Type == NodeTypeText {
			found = true
			if c.Text != "after" {
				t.Fatalf("text child = %q, want %q", c.Text, "after")
			}
		}
	}
	if !found {
		t.Fatal("expected a text child carrying the new value")
	}

	// set-attribute updates the Attrs map.
	sink.Handle([]js.DOMMutation{{
		Kind:      js.MutationSetAttribute,
		TargetID:  "100",
		Attribute: "class",
		NewValue:  "b",
	}})
	if target.Attrs["class"] != "b" {
		t.Fatalf("class attr = %q, want %q", target.Attrs["class"], "b")
	}
}

func TestMutationSinkAppendsTextChildOnEmptyElement(t *testing.T) {
	r := NewRenderer(800, 600)
	_, err := r.RenderHTML(context.Background(), `<html><body><div id="empty" __goosie_id="50"></div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	lookup := NewNodeIDLookup()
	lookup.Snapshot(r.GetRoot())
	sink := NewMutationSink(r, lookup, nil)

	sink.Handle([]js.DOMMutation{{
		Kind:     js.MutationSetText,
		TargetID: "50",
		NewValue: "filled",
	}})
	empty := findRenderNodeByIDRoot(r.GetRoot(), lookup.Lookup("50"))
	if empty == nil {
		t.Fatal("target render node not found")
	}
	if len(empty.Children) != 1 || empty.Children[0].Type != NodeTypeText || empty.Children[0].Text != "filled" {
		t.Fatalf("expected an appended text child with %q, got %+v", "filled", empty.Children)
	}
}

func TestMutationSinkRejectsStaleNodeIDs(t *testing.T) {
	r := NewRenderer(800, 600)
	_, err := r.RenderHTML(context.Background(), `<html><body><div>no ids</div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	sink := NewMutationSink(r, NewNodeIDLookup(), nil)
	// Unknown TargetID and ParentID: rejected without dirtying the renderer.
	sink.Handle([]js.DOMMutation{{
		Kind:     js.MutationSetText,
		TargetID: "99999",
		NewValue: "x",
	}})
	if r.IsDirty() {
		t.Fatal("stale NodeID must be rejected without marking the renderer dirty")
	}
}

func TestMutationBatchInvalidatesCanvasDisplayListCache(t *testing.T) {
	r := NewRenderer(800, 600)
	_, err := r.RenderHTML(context.Background(), `<html><body><div id="target" __goosie_id="100">before</div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	// Warm the canvas display-list cache (present builds it).
	r.UpdateViewport()
	r.canvasRenderer.mu.Lock()
	cached := r.canvasRenderer.cachedDisplayList
	r.canvasRenderer.mu.Unlock()
	if cached == nil {
		t.Fatal("expected a warm display list cache")
	}

	lookup := NewNodeIDLookup()
	lookup.Snapshot(r.GetRoot())
	sink := NewMutationSink(r, lookup, nil)
	sink.Handle([]js.DOMMutation{{
		Kind:      js.MutationSetAttribute,
		TargetID:  "100",
		Attribute: "data-x",
		NewValue:  "1",
	}})

	r.canvasRenderer.mu.Lock()
	cachedAfter := r.canvasRenderer.cachedDisplayList
	r.canvasRenderer.mu.Unlock()
	if cachedAfter != nil {
		t.Fatal("display list cache must be dropped after an in-place mutation")
	}
}

func TestMutationSinkIgnoresUnknownIDs(t *testing.T) {
	r := NewRenderer(800, 600)
	_, err := r.RenderHTML(context.Background(), `<html><body><div id="target">before</div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	sink := NewMutationSink(r, NewNodeIDLookup(), nil)
	sink.Handle([]js.DOMMutation{{Kind: js.MutationSetAttribute, TargetID: "unknown"}})
	if r.IsDirty() {
		t.Fatal("renderer should remain clean for unknown IDs")
	}
}

func BenchmarkMutationSinkAttributeMutation(b *testing.B) {
	r := NewRenderer(800, 600)
	_, err := r.RenderHTML(context.Background(), `<html><body><div id="target" __goosie_id="100">before</div></body></html>`)
	if err != nil {
		b.Fatal(err)
	}
	lookup := NewNodeIDLookup()
	lookup.Snapshot(r.GetRoot())
	sink := NewMutationSink(r, lookup, nil)
	batch := []js.DOMMutation{{
		Kind:      js.MutationSetAttribute,
		TargetID:  "100",
		Attribute: "data-state",
		NewValue:  "ready",
	}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink.Handle(batch)
		r.incremental.GetInvalidationTracker().ClearAll()
	}
}

func TestMutationSinkWithAdapterPresentsFrame(t *testing.T) {
	r := NewRenderer(200, 200)
	_, err := r.RenderHTML(context.Background(), `<html><body><div id="target" __goosie_id="100">before</div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	lookup := NewNodeIDLookup()
	lookup.Snapshot(r.GetRoot())
	r.chunkedDisplay.chunks.Add(PaintChunk{Owner: LayoutID(100), Start: 0, End: 1, Bounds: RectF{X: 0, Y: 0, W: 50, H: 50}})
	r.chunkedDisplay.commands.Add(DisplayCommand{Kind: CmdRect, Rect: RectCommand{Bounds: RectF{X: 0, Y: 0, W: 200, H: 200}}})
	adapter := NewFyneAdapter()
	sink := NewMutationSinkWithAdapter(r, lookup, adapter)
	sink.Handle([]js.DOMMutation{{
		Kind:      js.MutationSetAttribute,
		TargetID:  "100",
		Attribute: "data-state",
		NewValue:  "ready",
	}})
	if adapter.CurrentFrame() == nil {
		t.Fatal("expected adapter to receive a frame")
	}
}

// sameRGBAColor reports whether the computed color equals the parsed color
// of the given hex string (the concrete type parseColor returns for hex).
func sameRGBAColor(got color.Color, hex string) bool {
	want, err := parseColor(hex)
	if err != nil {
		return false
	}
	return reflect.DeepEqual(got, want)
}

// TestMutationRecomputesStyleOnClassChange exercises the PR10 typed-path
// style recompute: class attribute mutations must produce fresh computed
// styles (and revert cleanly) without a full reparse.
func TestMutationRecomputesStyleOnClassChange(t *testing.T) {
	r := NewRenderer(800, 600)
	_, err := r.RenderHTML(context.Background(), `<html><head><style>.a { color: #ff0000; } .b { color: #0000ff; }</style></head><body><div id="target" __goosie_id="100" class="a">text</div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	lookup := NewNodeIDLookup()
	lookup.Snapshot(r.GetRoot())
	sink := NewMutationSink(r, lookup, nil)

	target := findRenderNodeByIDRoot(r.GetRoot(), lookup.Lookup("100"))
	if target == nil {
		t.Fatal("target render node not found")
	}
	if got := target.ComputedStyle.Color; !sameRGBAColor(got, "#ff0000") {
		t.Fatalf("initial color = %v, want red", got)
	}

	// class="a" → class="b": computed style must flip to blue via the typed
	// path, with no reparse.
	sink.Handle([]js.DOMMutation{{
		Kind:      js.MutationSetAttribute,
		TargetID:  "100",
		Attribute: "class",
		NewValue:  "b",
	}})
	if got := target.ComputedStyle.Color; !sameRGBAColor(got, "#0000ff") {
		t.Fatalf("color after class change = %v, want blue", got)
	}

	// Removing the class must revert to the (colorless) default styling
	// rather than keeping the stale .b color.
	sink.Handle([]js.DOMMutation{{
		Kind:      js.MutationSetAttribute,
		TargetID:  "100",
		Attribute: "class",
		NewValue:  "",
	}})
	if got := target.ComputedStyle.Color; got != nil {
		t.Fatalf("color after class removal = %v, want nil (default)", got)
	}
}

// TestMutationRelayoutsSubtreeOnClassChange verifies that an attribute
// mutation that changes display also relayouts the subtree: the incremental
// engine must rebuild the target's layout box with the new display type.
func TestMutationRelayoutsSubtreeOnClassChange(t *testing.T) {
	r := NewRenderer(800, 600)
	_, err := r.RenderHTML(context.Background(), `<html><head><style>.foo { display: block; } .bar { display: flex; }</style></head><body><div id="target" __goosie_id="100" class="foo">text</div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	lookup := NewNodeIDLookup()
	lookup.Snapshot(r.GetRoot())
	sink := NewMutationSink(r, lookup, nil)

	target := findRenderNodeByIDRoot(r.GetRoot(), lookup.Lookup("100"))
	if target == nil {
		t.Fatal("target render node not found")
	}
	if got := target.ComputedStyle.Display; got != "block" {
		t.Fatalf("initial display = %q, want block", got)
	}

	// buildFrame lays out on a separate engine, so the incremental engine
	// has no box for the target before the mutation.
	le := r.incremental.LayoutEngine
	le.nodeMapMu.RLock()
	_, pre := le.nodeMap[target.ID]
	le.nodeMapMu.RUnlock()
	if pre {
		t.Fatal("incremental engine unexpectedly has a box before the mutation")
	}

	sink.Handle([]js.DOMMutation{{
		Kind:      js.MutationSetAttribute,
		TargetID:  "100",
		Attribute: "class",
		NewValue:  "bar",
	}})

	if got := target.ComputedStyle.Display; got != "flex" {
		t.Fatalf("display after class change = %q, want flex", got)
	}
	le.nodeMapMu.RLock()
	box, ok := le.nodeMap[target.ID]
	le.nodeMapMu.RUnlock()
	if !ok {
		t.Fatal("expected a rebuilt layout box after the mutation")
	}
	if box.Display != DisplayFlex {
		t.Fatalf("rebuilt box display = %v, want DisplayFlex", box.Display)
	}
}

// TestMutationRecomputesInlineStyleAttribute verifies the style= attribute
// is re-parsed on the typed path: changing it recomputes the computed style
// and clearing it reverts to the default styling.
func TestMutationRecomputesInlineStyleAttribute(t *testing.T) {
	r := NewRenderer(800, 600)
	_, err := r.RenderHTML(context.Background(), `<html><body><div id="target" __goosie_id="100" style="color: #00ff00">text</div></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	lookup := NewNodeIDLookup()
	lookup.Snapshot(r.GetRoot())
	sink := NewMutationSink(r, lookup, nil)

	target := findRenderNodeByIDRoot(r.GetRoot(), lookup.Lookup("100"))
	if target == nil {
		t.Fatal("target render node not found")
	}
	if got := target.ComputedStyle.Color; !sameRGBAColor(got, "#00ff00") {
		t.Fatalf("initial color = %v, want green", got)
	}

	sink.Handle([]js.DOMMutation{{
		Kind:      js.MutationSetAttribute,
		TargetID:  "100",
		Attribute: "style",
		NewValue:  "color: #ff0000",
	}})
	if got := target.ComputedStyle.Color; !sameRGBAColor(got, "#ff0000") {
		t.Fatalf("color after style change = %v, want red", got)
	}

	sink.Handle([]js.DOMMutation{{
		Kind:      js.MutationSetAttribute,
		TargetID:  "100",
		Attribute: "style",
		NewValue:  "",
	}})
	if got := target.ComputedStyle.Color; got != nil {
		t.Fatalf("color after style clear = %v, want nil (default)", got)
	}
}

func TestNodeIDLookupBidirectional(t *testing.T) {
	lookup := NewNodeIDLookup()
	lookup.Bind("42", 1001)
	lookup.Bind("99", 2002)
	if lookup.Lookup("42") != 1001 {
		t.Fatalf("forward lookup failed")
	}
	if lookup.Reverse(2002) != "99" {
		t.Fatalf("reverse lookup failed")
	}
	lookup.ForgetRenderID(1001)
	if lookup.Lookup("42") != 0 {
		t.Fatalf("forget did not clear entry")
	}
}
