package renderer

import (
	"context"
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
