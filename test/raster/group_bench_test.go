package raster_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
)

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

func BenchmarkGroupFadeSmallBox(b *testing.B) {
	benchRaster(b, fadeList(frame.Point{X: 10, Y: 10}, true))
}

func BenchmarkGroupFadeNoGroup(b *testing.B) {
	benchRaster(b, fadeList(frame.Point{X: 10, Y: 10}, false))
}

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
