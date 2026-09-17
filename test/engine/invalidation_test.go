package engine_test

import (
 "testing"

 "github.com/vyquocvu/goosie/internal/engine"
 "github.com/vyquocvu/goosie/internal/frame"
)

func TestScrollInvalidationDoesNotScheduleContentWork(t *testing.T) {
 var inv engine.Invalidation
 vp := frame.Viewport{Offset: frame.Point{Y:100}, Size:frame.Size{W:512,H:512}}
 allocs := testing.AllocsPerRun(600, func() {
  inv.Add(engine.Scroll, frame.RectF4(0,0,512,512), 1)
  p := inv.Resolve(vp, nil)
  if p.FullDoc || len(p.Rects)!=0 || len(p.Subtree)!=0 || p.StyleObjects!=0 || p.LayoutObjects!=0 || p.Viewport!=vp {
   t.Fatalf("scroll scheduled content work: %+v", p)
  }
  inv.Reset()
 })
 if allocs!=0 { t.Fatalf("scroll allocated %g times",allocs) }
}

func TestInvalidationKeepsDisjointDamageAndReasons(t *testing.T) {
 pool := frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, 8)
 grid := frame.NewGrid(frame.Rect4(0,0,768,256),frame.TileSize, 8*frame.TileSizeBytes(),pool)
 for col:=int32(0); col<3; col++ {
  c:=frame.TileCoord{Col:col}
  out,ok:=grid.Acquire(c); if !ok {t.Fatal("acquire")}; grid.MarkValid(c,1,out)
 }
 var inv engine.Invalidation
 left,right:=frame.RectF4(1,1,10,10),frame.RectF4(520,1,530,10)
 inv.Add(engine.Hover,left,2)
 inv.Add(engine.StyleChange,left,2)
 inv.Add(engine.ImageLoad,right,3)
 p:=inv.Resolve(frame.Viewport{Size:frame.Size{W:768,H:256}},grid)
 if len(p.Subtree)!=1 || p.Subtree[0]!=2 {t.Fatalf("paint-only image requested layout: %v",p.Subtree)}
 if len(p.Rects)!=2 || p.TilesInvalidated!=2 {t.Fatalf("damage was broadened or duplicated: %+v",p)}
 if !grid.Needs(frame.TileCoord{Col:0},1) || grid.Needs(frame.TileCoord{Col:1},1) || !grid.Needs(frame.TileCoord{Col:2},1) {t.Fatal("only intersecting tiles must become stale")}
 if p.Reasons & engine.Hover==0 || p.Reasons & engine.StyleChange==0 || p.Reasons & engine.ImageLoad==0 {t.Fatal("reason lost")}
 inv.Reset()
 if p=inv.Resolve(frame.Viewport{},nil); p.Reasons!=0 || len(p.Rects)!=0 || len(p.Subtree)!=0 {t.Fatal("reset retained batch")}
}

func TestFullInvalidationIsExplicit(t *testing.T) {
 for _,reason:=range []engine.Reason{engine.Resize,engine.StylesheetChange} {
  var inv engine.Invalidation
  inv.Add(reason,frame.RectF{},0)
  if !inv.Resolve(frame.Viewport{},nil).FullDoc {t.Fatal("global change not propagated")}
 }
 var inv engine.Invalidation
 inv.Add(engine.DOMMutation,frame.RectF4(0,0,5,5),4)
 if inv.Resolve(frame.Viewport{},nil).FullDoc {t.Fatal("local mutation escalated to whole document")}
}
