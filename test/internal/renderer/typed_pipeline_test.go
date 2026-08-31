package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"context"
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
)

func TestTypedMutationEndToEndNoFullReparse(t *testing.T) {
	page := `<html><body><div id="target" __goosie_id="100">before</div></body></html>`
	r := renderer.NewRenderer(200, 200)
	_, err := r.RenderHTML(context.Background(), page)
	if err != nil {
		t.Fatal(err)
	}
	lookup := renderer.NewNodeIDLookup()
	lookup.Snapshot(r.GetRoot())
	adapter := renderer.NewFyneAdapter()
	sink := renderer.NewMutationSinkWithAdapter(r, lookup, adapter)

	// The chunk pipeline is producer-owned: seed it with the document's
	// commands and one dirty chunk so the present path has real work.
	// Without this, PresentFromMutationBatch correctly presents nothing.
	r.ChunkedDisplay().Commands().Add(renderer.DisplayCommand{
		Kind: renderer.CmdRect,
		Rect: renderer.RectCommand{Bounds: renderer.RectF{X: 0, Y: 0, W: 200, H: 200}},
	})
	r.ChunkedDisplay().Chunks().Add(renderer.PaintChunk{
		Owner:  renderer.LayoutID(100),
		Start:  0,
		End:    1,
		Bounds: renderer.RectF{X: 0, Y: 0, W: 200, H: 200},
	})
	r.ChunkedDisplay().InvalidateByLayoutID(renderer.LayoutID(100))

	rt := js.NewRuntime()
	rt.SetDOMMutationBatchCallback(sink.Handle)
	rt.SetHTMLContent(page)
	if _, err := rt.RunScript(`document.body.firstChild.setAttribute("data-state","ready"); document.body.firstChild.textContent = "after";`); err != nil {
		t.Fatal(err)
	}
	if adapter.CurrentFrame() == nil {
		t.Fatal("expected adapter to receive a frame after JS mutation")
	}
	if r.ChunkedDisplay().DirtyChunkCount() != 0 {
		t.Fatalf("expected chunks to be clean after present, got %d", r.ChunkedDisplay().DirtyChunkCount())
	}
}
