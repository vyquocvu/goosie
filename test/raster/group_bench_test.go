package raster_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
)

// Group-opacity compositing cost. compositeGroup resets, fades and blits a
// tile-sized scratch for every group a tile touches, so the price of a small
// faded box is the tile rather than the box. These benchmarks are the before and
// after evidence for restricting those passes to the region a group can write;
// TestGroupOpacityOverlapComposite owns the pixel contract.
//
// The measured path is DefaultRaster with a Job carrying a pooled scratch, which
// is exactly what one worker does on a frame - no channel, no worker count in
// the number.
const benchTile = 256

func benchRaster(b *testing.B, dl *paint.LayerDL) {
	b.Helper()
	rasterTile := raster.DefaultRaster(nil, nil)
	tile := frame.Rect4(0, 0, benchTile, benchTile)
	out := frame.NewBitmap(benchTile, benchTile)
	pool := frame.NewBitmapPool(out.Size(), 4)
	pool.Prealloc(4)
	job := raster.Job{DL: dl, Bounds: tile, Out: out,
		Background: frame.RGB(255, 255, 255), Scratch: pool}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := rasterTile(job); err != nil {
			b.Fatal(err)
		}
	}
}

// fadeList builds three overlapping 40x40 blues with their top-left at origin,
// recorded as one faded group (own opacity 0.5) or left as plain commands.
func fadeList(origin frame.Point, grouped bool) *paint.LayerDL {
	l := paint.NewList(4)
	for i := 0; i < 3; i++ {
		x0 := origin.X + int32(i)
		l.Append(paint.DisplayCmd{Kind: paint.CmdFill,
			Rect:  frame.Rect4(x0, origin.Y, x0+40, origin.Y+40),
			Color: frame.RGBA(0, 0, 255, 128)})
	}
	if grouped {
		l.AddGroup(0.5, 3)
	}
	return l.Build(1).Publish()
}

// A small faded box in a 256x256 tile: the shape of a toolbar button, a badge or
// a dimmed thumbnail.
func BenchmarkGroupFadeSmallBox(b *testing.B) {
	benchRaster(b, fadeList(frame.Point{X: 10, Y: 10}, true))
}

// The same three fills, unfaded and ungrouped: what the frame path cost before
// offscreen compositing, so the gap to the case above is what a region
// restriction can hand back.
func BenchmarkGroupFadeNoGroup(b *testing.B) {
	benchRaster(b, fadeList(frame.Point{X: 10, Y: 10}, false))
}

// Three fills spread over the whole tile, so the group's region is the tile: no
// restriction helps here, and this is the floor the per-tile pass stays at.
func BenchmarkGroupFadeWholeTile(b *testing.B) {
	l := paint.NewList(4)
	for i := 0; i < 3; i++ {
		x0, y0 := int32(i*90), int32(i*70)
		l.Append(paint.DisplayCmd{Kind: paint.CmdFill,
			Rect: frame.Rect4(x0, y0, x0+220, y0+180), Color: frame.RGBA(0, 0, 255, 128)})
	}
	l.AddGroup(0.5, 3)
	benchRaster(b, l.Build(1).Publish())
}

// 25 separate 24x24 faded boxes: a grid of dimmed thumbnails, where the fixed
// per-group pass is paid 25 times inside one tile.
func BenchmarkGroupFadeManySmallBoxes(b *testing.B) {
	l := paint.NewList(25)
	for y := 0; y < 5; y++ {
		for x := 0; x < 5; x++ {
			r := frame.Rect4(int32(x*48+8), int32(y*48+8), int32(x*48+32), int32(y*48+32))
			l.Append(paint.DisplayCmd{Kind: paint.CmdFill, Rect: r, Color: frame.RGBA(0, 0, 255, 128)})
			l.AddGroup(0.5, 1)
		}
	}
	benchRaster(b, l.Build(1).Publish())
}
