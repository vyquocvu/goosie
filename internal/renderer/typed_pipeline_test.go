package renderer

import (
	"context"
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
)

func TestTypedMutationEndToEndNoFullReparse(t *testing.T) {
	page := `<html><body><div id="target" __goosie_id="100">before</div></body></html>`
	r := NewRenderer(200, 200)
	_, err := r.RenderHTML(context.Background(), page)
	if err != nil {
		t.Fatal(err)
	}
	lookup := NewNodeIDLookup()
	lookup.Snapshot(r.GetRoot())
	adapter := NewFyneAdapter()
	sink := NewMutationSinkWithAdapter(r, lookup, adapter)

	// The chunk pipeline is producer-owned: seed it with the document's
	// commands and one dirty chunk so the present path has real work.
	// Without this, PresentFromMutationBatch correctly presents nothing.
	r.chunkedDisplay.commands.Add(DisplayCommand{
		Kind: CmdRect,
		Rect: RectCommand{Bounds: RectF{X: 0, Y: 0, W: 200, H: 200}},
	})
	r.chunkedDisplay.chunks.Add(PaintChunk{
		Owner:  LayoutID(100),
		Start:  0,
		End:    1,
		Bounds: RectF{X: 0, Y: 0, W: 200, H: 200},
	})
	r.chunkedDisplay.InvalidateByLayoutID(LayoutID(100))

	rt := js.NewRuntime()
	rt.SetDOMMutationBatchCallback(sink.Handle)
	rt.SetHTMLContent(page)
	if _, err := rt.RunScript(`document.body.firstChild.setAttribute("data-state","ready"); document.body.firstChild.textContent = "after";`); err != nil {
		t.Fatal(err)
	}
	if adapter.CurrentFrame() == nil {
		t.Fatal("expected adapter to receive a frame after JS mutation")
	}
	if r.chunkedDisplay.DirtyChunkCount() != 0 {
		t.Fatalf("expected chunks to be clean after present, got %d", r.chunkedDisplay.DirtyChunkCount())
	}
}
