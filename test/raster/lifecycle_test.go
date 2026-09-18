package raster_test

import (
	"context"
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
)

func TestNavigationReturnsLateTileToItsOriginalGrid(t *testing.T) {
	bounds := frame.Rect4(0, 0, frame.TileSize, frame.TileSize)
	bp := frame.NewBitmapPool(bounds.Size(), 1)
	old := frame.NewLayer(1, bounds, frame.TileSizeBytes(), bp)
	list := (&paint.List{}).Build(1)
	old.SetContent(list)
	current := frame.NewLayer(1, bounds, frame.TileSizeBytes(), frame.NewBitmapPool(bounds.Size(), 1))
	current.SetContent(list)
	coord := frame.TileCoord{}
	pixels, ok := old.Grid.Acquire(coord)
	if !ok { t.Fatal("acquire") }
	p := raster.New(1, 2, func(raster.Job) error { return nil })
	p.Start(context.Background())
	defer p.Close()
	if err := p.Submit(raster.Job{Layer: old, Coord: coord, DL: list, Bounds: bounds, Out: pixels}); err != nil { t.Fatal(err) }
	done := p.Wait(1)
	s := raster.NewScheduler(current, p, frame.Viewport{Size: bounds.Size()}, 1, raster.Pref{})
	_ = s.HandleDone(done[0])
	if got := old.Grid.Stats().PaintingBytes; got != 0 {
		t.Fatalf("retired grid retained %d bytes of completed work", got)
	}
	if bp.Stats().Released != 1 {
		t.Fatal("late result was not returned to its original pool")
	}
}

func TestCloseReturnsQueuedRasterBuffers(t *testing.T) {
	p := raster.New(1, 2, func(raster.Job) error { return nil })
	for i := 0; i < 2; i++ {
		if err := p.Submit(raster.Job{Out: frame.NewBitmap(1, 1)}); err != nil { t.Fatal(err) }
	}
	if err := p.Close(); err != nil { t.Fatal(err) }
	count := 0
	for {
		d, ok := p.Poll()
		if !ok { break }
		if d.Out == nil || d.Err == nil { t.Fatal("queued result lost its buffer or cancellation error") }
		count++
	}
	if count != 2 || p.Stats().Outstanding != 0 {
		t.Fatalf("reclaimed %d queued buffers, outstanding=%d", count, p.Stats().Outstanding)
	}
}
