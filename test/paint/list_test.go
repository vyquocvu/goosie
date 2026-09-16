package paint_test

import (
	"testing"
	"unsafe"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

// content is the compile-time proof that a frozen display list can be attached
// to a layer without paint and frame importing each other.
var _ frame.Content = (*paint.LayerDL)(nil)

func fill(x0, y0, x1, y1 int32, z uint32) paint.DisplayCmd {
	return paint.DisplayCmd{
		Kind:  paint.CmdFill,
		Rect:  frame.Rect4(x0, y0, x1, y1),
		Color: frame.RGB(255, 0, 0),
		Z:     z,
	}
}

func TestListReusesBackingAcrossReset(t *testing.T) {
	l := paint.NewList(8)
	l.Append(fill(0, 0, 10, 10, 0))
	first := unsafe.SliceData(l.All())

	l.Reset()
	if l.Len() != 0 {
		t.Fatalf("Len after Reset = %d, want 0", l.Len())
	}
	if !l.Bounds().Empty() {
		t.Fatalf("Bounds after Reset = %v, want empty", l.Bounds())
	}
	l.Append(fill(0, 0, 4, 4, 0))
	if got := unsafe.SliceData(l.All()); got != first {
		t.Fatalf("Reset dropped the backing array: %p then %p", first, got)
	}
}

func TestListResetDropsCommandBounds(t *testing.T) {
	l := paint.NewList(4)
	glyphs := []paint.GlyphRun{{Rune: 'a', X: 0, Y: 10, Size: 10}}
	l.Append(paint.DisplayCmd{Kind: paint.CmdText, Text: paint.TextRun{Glyphs: glyphs, Color: frame.RGB(0, 0, 0)}})
	if l.All()[0].Text.Glyphs == nil {
		t.Fatal("text command lost its glyphs")
	}
	l.Reset()
	if got := l.All(); len(got) != 0 {
		t.Fatalf("All after Reset = %d commands", len(got))
	}
	// The retained capacity is zeroed, so a Reset list cannot pin the glyphs
	// slice alive through a stale reference in the unused tail.
	l.Append(fill(0, 0, 2, 2, 0))
	if got := l.All()[0].Text.Glyphs; got != nil {
		t.Fatalf("Reset left a live glyph reference: %v", got)
	}
}

func TestAppendTracksUnionBounds(t *testing.T) {
	l := paint.NewList(0)
	if !l.Bounds().Empty() {
		t.Fatalf("empty list bounds = %v, want empty", l.Bounds())
	}
	l.Append(fill(10, 20, 30, 40, 0))
	l.Append(fill(-5, 100, 5, 110, 0))
	want := frame.Rect4(-5, 20, 30, 110)
	if got := l.Bounds(); got != want {
		t.Fatalf("union bounds = %v, want %v", got, want)
	}
	// A command that paints nothing must not widen the extent.
	l.Append(paint.DisplayCmd{Kind: paint.CmdFill, Rect: frame.Rect4(0, 0, 999, 999)})
	if got := l.Bounds(); got != want {
		t.Fatalf("transparent fill widened bounds to %v, want %v", got, want)
	}
}

func TestSortStableIsOrderedByZAndTiedAppendOrder(t *testing.T) {
	l := paint.NewList(0)
	// Three groups by Z, each with distinguishable append order via Y.
	l.Append(fill(0, 300, 1, 301, 2))
	l.Append(fill(0, 100, 1, 101, 1))
	l.Append(fill(0, 200, 1, 201, 2))
	l.Append(fill(0, 400, 1, 401, 0))
	l.Append(fill(0, 500, 1, 501, 2))
	l.SortStable()

	var gotY []int32
	for i := 0; i < l.Len(); i++ {
		gotY = append(gotY, l.At(i).Rect.Y0)
	}
	wantY := []int32{400, 100, 300, 200, 500}
	for i := range wantY {
		if gotY[i] != wantY[i] {
			t.Fatalf("paint order after SortStable = %v, want %v", gotY, wantY)
		}
	}
}

func TestFinishTransfersOwnershipAndFreezes(t *testing.T) {
	l := paint.NewList(4)
	l.Append(fill(0, 0, 16, 16, 0))
	backing := unsafe.SliceData(l.All())

	dl := l.Build(7)
	if dl.Frozen() {
		t.Fatal("Build returned a list already marked frozen; the freeze is Publish's job")
	}
	if l.Len() != 0 {
		t.Fatalf("builder kept %d commands after Build", l.Len())
	}
	dl.Publish()
	if !dl.Frozen() {
		t.Fatal("Publish did not freeze the list")
	}
	if unsafe.SliceData(dl.All()) != backing {
		t.Fatal("Build copied the slab instead of moving it; a per-version copy breaks the allocation budget")
	}
	if dl.Version() != 7 {
		t.Fatalf("Version = %d, want 7", dl.Version())
	}
	if want := frame.Rect4(0, 0, 16, 16); dl.Extent() != want {
		t.Fatalf("Extent = %v, want %v", dl.Extent(), want)
	}
}

func TestPublishTwicePanics(t *testing.T) {
	dl := published(1, fill(0, 0, 8, 8, 0), fill(0, 8, 8, 16, 0))
	defer func() {
		if recover() == nil {
			t.Fatal("second Publish did not panic; a display list must be immutable after publication")
		}
	}()
	dl.Publish()
}

func TestIntersectingReturnsSpanWithoutAllocating(t *testing.T) {
	// Commands at y 0, 100, 200, 300, 400 with one transparent command in the
	// middle of the queried range so the span must over-approximate rather than
	// filter.
	dl := published(3,
		fill(0, 0, 10, 10, 0),
		fill(0, 100, 10, 110, 0),
		paint.DisplayCmd{Kind: paint.CmdFill, Rect: frame.Rect4(0, 200, 10, 210)}, // transparent
		fill(0, 300, 10, 310, 0),
		fill(0, 400, 10, 410, 0),
	)

	span := dl.Intersecting(frame.Rect4(0, 90, 10, 310))
	if len(span) != 3 {
		t.Fatalf("span length = %d, want 3 (indices 1..3 inclusive)", len(span))
	}
	if span[0].Rect.Y0 != 100 || span[len(span)-1].Rect.Y0 != 300 {
		t.Fatalf("span edges = %v..%v, want y100..y300", span[0].Rect, span[len(span)-1].Rect)
	}

	if got := dl.Intersecting(frame.Rect4(0, 5000, 10, 5100)); got != nil {
		t.Fatalf("disjoint query returned %d commands, want nil", len(got))
	}

	// The span is a view into the same array, not a copy.
	head := unsafe.SliceData(dl.All())
	spanHead := unsafe.SliceData(span)
	if spanHead == head {
		t.Fatal("span starts at the list head; the first command does not intersect")
	}
	off := uintptr(unsafe.Pointer(spanHead)) - uintptr(unsafe.Pointer(head))
	if off%unsafe.Sizeof(paint.DisplayCmd{}) != 0 {
		t.Fatalf("span is not inside the list array: offset %d bytes", off)
	}

	if n := testing.AllocsPerRun(50, func() { _ = dl.Intersecting(frame.Rect4(0, 90, 10, 310)) }); n != 0 {
		t.Fatalf("Intersecting allocated %v times per call; a tile query may not allocate", n)
	}
}

func TestCommandIntersectsIsPerKind(t *testing.T) {
	text := paint.DisplayCmd{
		Kind: paint.CmdText,
		Text: paint.TextRun{
			Glyphs: []paint.GlyphRun{{Rune: 'H', X: 20, Y: 30, Size: 10}, {Rune: 'i', X: 30, Y: 30, Size: 10}},
			Color:  frame.RGB(0, 0, 0),
		},
	}
	if !text.Intersects(frame.Rect4(15, 20, 40, 32)) {
		t.Fatalf("text run did not intersect its own glyph box: %v", mustBounds(t, text))
	}
	if text.Intersects(frame.Rect4(0, 100, 10, 110)) {
		t.Fatal("text run intersected a rect far below its baseline")
	}

	border := paint.DisplayCmd{
		Kind: paint.CmdBorder,
		Rect: frame.Rect4(0, 0, 50, 50),
		Border: paint.BorderSpec{
			Top:    paint.SideSpec{Width: 2, Color: frame.RGB(0, 0, 0)},
			Right:  paint.SideSpec{Width: 2, Color: frame.RGB(0, 0, 0)},
			Bottom: paint.SideSpec{Width: 2, Color: frame.RGB(0, 0, 0)},
			Left:   paint.SideSpec{Width: 2, Color: frame.RGB(0, 0, 0)},
		},
	}
	if !border.Intersects(frame.Rect4(40, 0, 60, 10)) {
		t.Fatal("border command did not intersect its own right edge")
	}
	empty := border
	empty.Border = paint.BorderSpec{}
	if empty.Intersects(frame.Rect4(0, 0, 50, 50)) {
		t.Fatal("zero-width border still reported as intersecting")
	}
}

func BenchmarkListRebuild(b *testing.B) {
	l := paint.NewList(4096)
	cmds := make([]paint.DisplayCmd, 4096)
	for i := range cmds {
		cmds[i] = fill(int32(i), int32(i), int32(i)+8, int32(i)+8, uint32(i%7))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Reset()
		for _, c := range cmds {
			l.Append(c)
		}
		l.SortStable()
		_ = l.Build(uint64(i)).Publish()
	}
}

func published(v uint64, cmds ...paint.DisplayCmd) *paint.LayerDL {
	l := paint.NewList(len(cmds))
	for _, c := range cmds {
		l.Append(c)
	}
	return l.Build(v).Publish()
}

func mustBounds(t *testing.T, c paint.DisplayCmd) frame.Rect {
	t.Helper()
	b, _ := c.Bounds()
	return b
}
