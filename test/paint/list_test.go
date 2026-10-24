package paint_test

import (
	"image"
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

func TestContentDamage(t *testing.T) {
	build := func(cmds ...paint.DisplayCmd) *paint.LayerDL {
		l := paint.NewList(len(cmds))
		for _, cmd := range cmds {
			l.Append(cmd)
		}
		return l.Build(1).Publish()
	}
	background := fill(0, 0, 768, 768, 0)
	old := fill(20, 20, 60, 60, 0)
	moved := fill(20, 300, 60, 350, 0)
	far := fill(600, 600, 640, 640, 0)
	text := paint.DisplayCmd{Kind: paint.CmdText, Text: paint.TextRun{Color: frame.RGB(0, 0, 0), Glyphs: []paint.GlyphRun{{Rune: 'a', X: 20, Y: 40, Size: 16}}}}
	changedText := text
	changedText.Text.Glyphs = []paint.GlyphRun{{Rune: 'b', X: 20, Y: 40, Size: 16}}
	imageCmd := paint.DisplayCmd{Kind: paint.CmdImage, Rect: old.Rect, Image: paint.ImageSpec{Src: image.NewRGBA(image.Rect(0, 0, 4, 4))}}
	changedImage := imageCmd
	changedImage.Image.Src = image.NewRGBA(image.Rect(0, 0, 4, 4))
	for _, tc := range []struct {
		name          string
		before, after *paint.LayerDL
		want          frame.Rect
	}{
		{"unchanged", build(background, text, imageCmd), build(background, text, imageCmd), frame.Rect{}},
		{"moved", build(background, old, far), build(background, moved, far), old.Rect.Union(moved.Rect)},
		{"removed", build(background, old, far), build(background, far), old.Rect},
		{"inserted", build(background, far), build(background, old, far), old.Rect},
		{"reordered", build(background, old, far), build(background, far, old), old.Rect.Union(far.Rect)},
		{"text_and_caret", build(background, text, far, old), build(background, changedText, far, moved), old.Rect.Union(moved.Rect)},
		{"new_image_identity", build(background, imageCmd, far), build(background, changedImage, far), old.Rect},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := paint.ContentDamage(tc.before, tc.after); got != tc.want {
				t.Fatalf("damage = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestContentDamageCommandProperties(t *testing.T) {
	for name, change := range map[string]func(*paint.DisplayCmd){
		"color":          func(c *paint.DisplayCmd) { c.Color = frame.RGB(0, 255, 0) },
		"border":         func(c *paint.DisplayCmd) { c.Border.Top.Width = 2 },
		"opacity":        func(c *paint.DisplayCmd) { c.Opacity = 0.5 },
		"radius":         func(c *paint.DisplayCmd) { c.Radius.TL.X = 2 },
		"clip":           func(c *paint.DisplayCmd) { c.Clip = frame.Rect4(20, 20, 30, 30) },
		"order":          func(c *paint.DisplayCmd) { c.Z = 2 },
		"filter":         func(c *paint.DisplayCmd) { c.Filter = "blur(2px)" },
		"backdrop":       func(c *paint.DisplayCmd) { c.BackdropFilter = "blur(2px)" },
		"text_color":     func(c *paint.DisplayCmd) { c.Text.Color = frame.RGB(0, 255, 0) },
		"text_glyphs":    func(c *paint.DisplayCmd) { c.Text.Glyphs = []paint.GlyphRun{{Rune: 'a'}} },
		"image_source":   func(c *paint.DisplayCmd) { c.Image.Src = image.NewRGBA(image.Rect(0, 0, 4, 4)) },
		"image_crop":     func(c *paint.DisplayCmd) { c.Image.SrcBox = image.Rect(0, 0, 4, 4) },
		"mask":           func(c *paint.DisplayCmd) { c.Mask = image.NewRGBA(image.Rect(0, 0, 4, 4)) },
		"gradient_angle": func(c *paint.DisplayCmd) { c.Gradient.Angle = 90 },
		"gradient_stops": func(c *paint.DisplayCmd) { c.Gradient.Stops = []frame.GradientStop{{At: 0.5}} },
		"radial_shape":   func(c *paint.DisplayCmd) { c.RadialGradient.Shape = "circle" },
		"radial_x":       func(c *paint.DisplayCmd) { c.RadialGradient.Cx = 0.5 },
		"radial_y":       func(c *paint.DisplayCmd) { c.RadialGradient.Cy = 0.5 },
		"radial_stops":   func(c *paint.DisplayCmd) { c.RadialGradient.Stops = []frame.GradientStop{{At: 0.5}} },
	} {
		t.Run(name, func(t *testing.T) {
			cmd := fill(10, 10, 50, 50, 0)
			before := paint.NewList(1)
			before.Append(cmd)
			change(&cmd)
			after := paint.NewList(1)
			after.Append(cmd)
			if got := paint.ContentDamage(before.Build(1), after.Build(1)); got != cmd.Rect {
				t.Fatalf("damage = %v, want %v", got, cmd.Rect)
			}
		})
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
