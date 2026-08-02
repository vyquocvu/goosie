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
