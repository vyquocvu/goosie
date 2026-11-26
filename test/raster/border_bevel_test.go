package raster_test

import (
	"image/color"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
)

func bevelRasterize(t *testing.T, cmds []paint.DisplayCmd, width, height int, background frame.Color) *frame.Bitmap {
	t.Helper()
	list := paint.NewList(len(cmds))
	for _, cmd := range cmds {
		list.Append(cmd)
	}
	dl := list.Build(1).Publish()
	out := frame.NewBitmap(width, height)
	if err := raster.RasterizeTile(dl, frame.Rect4(0, 0, int32(width), int32(height)), out, nil, nil, background); err != nil {
		t.Fatalf("RasterizeTile: %v", err)
	}
	return out
}

func bevelPixelClose(got frame.Color, want color.RGBA, tolerance int) bool {
	diff := func(a, b uint8) int {
		d := int(a) - int(b)
		if d < 0 {
			return -d
		}
		return d
	}
	return diff(got.R(), want.R) <= tolerance && diff(got.G(), want.G) <= tolerance &&
		diff(got.B(), want.B) <= tolerance && diff(got.A(), want.A) <= tolerance
}

func TestRasterizeTileInsetOutsetUsesMeasuredShades(t *testing.T) {
	for _, tc := range []struct {
		name        string
		base        frame.Color
		dark, light color.RGBA
	}{
		{"uaGrey", frame.RGB(118, 118, 118), color.RGBA{R: 33, G: 33, B: 33, A: 255}, color.RGBA{R: 202, G: 202, B: 202, A: 255}},
		{"authoredRed", frame.RGB(200, 80, 40), color.RGBA{R: 116, G: 46, B: 23, A: 255}, color.RGBA{R: 255, G: 102, B: 51, A: 255}},
		{"black", frame.RGB(0, 0, 0), color.RGBA{R: 84, G: 84, B: 84, A: 255}, color.RGBA{R: 168, G: 168, B: 168, A: 255}},
		{"white", frame.RGB(255, 255, 255), color.RGBA{R: 171, G: 171, B: 171, A: 255}, color.RGBA{R: 255, G: 255, B: 255, A: 255}},
	} {
		for _, style := range []paint.BorderStyle{paint.BorderStyleInset, paint.BorderStyleOutset} {
			t.Run(tc.name+"/"+styleName(style), func(t *testing.T) {
				spec := paint.SideSpec{Width: 4, Color: tc.base, Style: style}
				out := bevelRasterize(t, []paint.DisplayCmd{{
					Kind: paint.CmdBorder, Rect: frame.Rect4(10, 10, 50, 50),
					Border: paint.BorderSpec{Top: spec, Right: spec, Bottom: spec, Left: spec},
				}}, 60, 60, frame.TransparentBlack)
				top, bottom := tc.light, tc.dark
				if style == paint.BorderStyleInset {
					top, bottom = tc.dark, tc.light
				}
				checks := []struct {
					x, y int
					want color.RGBA
					name string
				}{
					{30, 10, top, "top"},
					{49, 30, bottom, "right"},
					{30, 49, bottom, "bottom"},
					{10, 30, top, "left"},
				}
				for _, check := range checks {
					if got := out.At(check.x, check.y); !bevelPixelClose(got, check.want, 0) {
						t.Errorf("%s pixel = %v, want %v", check.name, got.RGBA(), check.want)
					}
				}
			})
		}
	}
}

func styleName(style paint.BorderStyle) string {
	switch style {
	case paint.BorderStyleInset:
		return "inset"
	case paint.BorderStyleOutset:
		return "outset"
	case paint.BorderStyleRidge:
		return "ridge"
	case paint.BorderStyleGroove:
		return "groove"
	default:
		return "solid"
	}
}

func TestRasterizeTileRidgeAndGrooveSplitTheMeasuredShades(t *testing.T) {
	light := frame.RGB(202, 202, 202)
	dark := frame.RGB(33, 33, 33)
	for _, style := range []paint.BorderStyle{paint.BorderStyleRidge, paint.BorderStyleGroove} {
		t.Run(styleName(style), func(t *testing.T) {
			spec := paint.SideSpec{Width: 4, Color: frame.RGB(118, 118, 118), Style: style}
			out := bevelRasterize(t, []paint.DisplayCmd{{
				Kind: paint.CmdBorder, Rect: frame.Rect4(10, 10, 50, 50),
				Border: paint.BorderSpec{Top: spec, Right: spec, Bottom: spec, Left: spec},
			}}, 60, 60, frame.TransparentBlack)
			topOuter, topInner, bottomOuter, bottomInner := light, dark, dark, light
			if style == paint.BorderStyleGroove {
				topOuter, topInner, bottomOuter, bottomInner = dark, light, light, dark
			}
			checks := []struct {
				x, y int
				want frame.Color
				name string
			}{
				{30, 10, topOuter, "top outer"},
				{30, 13, topInner, "top inner"},
				{49, 30, bottomOuter, "right outer"},
				{46, 30, bottomInner, "right inner"},
				{30, 49, bottomOuter, "bottom outer"},
				{30, 46, bottomInner, "bottom inner"},
				{10, 30, topOuter, "left outer"},
				{13, 30, topInner, "left inner"},
			}
			for _, check := range checks {
				if got := out.At(check.x, check.y); got != check.want {
					t.Errorf("%s pixel = %v, want %v", check.name, got, check.want)
				}
			}
		})
	}
}

func TestRasterizeTileOddRidgeWidthGivesTheExtraPixelToTheOuterHalf(t *testing.T) {
	spec := paint.SideSpec{Width: 3, Color: frame.RGB(118, 118, 118), Style: paint.BorderStyleRidge}
	out := bevelRasterize(t, []paint.DisplayCmd{{
		Kind: paint.CmdBorder, Rect: frame.Rect4(10, 10, 50, 50),
		Border: paint.BorderSpec{Top: spec, Right: spec, Bottom: spec, Left: spec},
	}}, 60, 60, frame.TransparentBlack)
	if out.At(30, 10) != frame.RGB(202, 202, 202) || out.At(30, 11) != frame.RGB(202, 202, 202) {
		t.Error("ridge outer half did not receive both outer rows")
	}
	if out.At(30, 12) != frame.RGB(33, 33, 33) {
		t.Error("ridge inner half did not receive the remaining row")
	}
	if out.At(30, 49) != frame.RGB(33, 33, 33) || out.At(30, 48) != frame.RGB(33, 33, 33) {
		t.Error("ridge bottom outer half did not receive both outer rows")
	}
	if out.At(30, 47) != frame.RGB(202, 202, 202) {
		t.Error("ridge bottom inner half did not receive the remaining row")
	}
}

func TestRasterizeTileBevelPreservesAlphaAndOpacity(t *testing.T) {
	for _, tc := range []struct {
		name    string
		colour  frame.Color
		opacity float32
		want    color.RGBA
	}{
		{"alpha", frame.RGBA(118, 118, 118, 128), 1, color.RGBA{R: 143, G: 143, B: 143, A: 255}},
		{"opacity", frame.RGB(118, 118, 118), 0.5, color.RGBA{R: 144, G: 144, B: 144, A: 255}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spec := paint.SideSpec{Width: 4, Color: tc.colour, Style: paint.BorderStyleInset}
			out := bevelRasterize(t, []paint.DisplayCmd{{
				Kind: paint.CmdBorder, Rect: frame.Rect4(10, 10, 50, 50), Opacity: tc.opacity,
				Border: paint.BorderSpec{Top: spec, Right: spec, Bottom: spec, Left: spec},
			}}, 60, 60, frame.RGB(255, 255, 255))
			if got := out.At(30, 10); !bevelPixelClose(got, tc.want, 2) {
				t.Errorf("composited dark edge = %v, want %v +-2", got.RGBA(), tc.want)
			}
		})
	}
}

func TestRasterizeTileBevelKeepsRadiusClipping(t *testing.T) {
	radius := frame.Corners{
		TL: frame.Radius{X: 12, Y: 12}, TR: frame.Radius{X: 12, Y: 12},
		BR: frame.Radius{X: 12, Y: 12}, BL: frame.Radius{X: 12, Y: 12},
	}
	spec := paint.SideSpec{Width: 4, Color: frame.RGB(118, 118, 118), Style: paint.BorderStyleInset}
	out := bevelRasterize(t, []paint.DisplayCmd{{
		Kind: paint.CmdBorder, Rect: frame.Rect4(10, 10, 50, 50), Radius: radius,
		Border: paint.BorderSpec{Top: spec, Right: spec, Bottom: spec, Left: spec},
	}}, 60, 60, frame.TransparentBlack)
	if got := out.At(30, 10); got != frame.RGB(33, 33, 33) {
		t.Errorf("top edge centre = %v, want the inset dark shade", got)
	}
	if got := out.At(10, 10).A(); got >= 255 {
		t.Errorf("rounded corner alpha = %d, want clipped coverage", got)
	}
}

func TestRasterizeTileRidgeWithSmallRadiusKeepsTheInnerHalf(t *testing.T) {
	radius := frame.Corners{
		TL: frame.Radius{X: 2, Y: 2}, TR: frame.Radius{X: 2, Y: 2},
		BR: frame.Radius{X: 2, Y: 2}, BL: frame.Radius{X: 2, Y: 2},
	}
	spec := paint.SideSpec{Width: 4, Color: frame.RGB(118, 118, 118), Style: paint.BorderStyleRidge}
	out := bevelRasterize(t, []paint.DisplayCmd{{
		Kind: paint.CmdBorder, Rect: frame.Rect4(10, 10, 50, 50), Radius: radius,
		Border: paint.BorderSpec{Top: spec, Right: spec, Bottom: spec, Left: spec},
	}}, 60, 60, frame.TransparentBlack)
	if out.At(30, 10) != frame.RGB(202, 202, 202) || out.At(30, 13) != frame.RGB(33, 33, 33) {
		t.Error("small-radius ridge lost its outer or inner half")
	}
	if out.At(10, 10).A() >= 255 {
		t.Error("small-radius ridge painted an unclipped corner")
	}
}

func TestRasterizeTileSolidPerSideColoursAreUnchanged(t *testing.T) {
	red := frame.RGB(255, 0, 0)
	blue := frame.RGB(0, 0, 255)
	out := bevelRasterize(t, []paint.DisplayCmd{{
		Kind: paint.CmdBorder, Rect: frame.Rect4(10, 10, 50, 50),
		Border: paint.BorderSpec{
			Top:    paint.SideSpec{Width: 4, Color: red, Style: paint.BorderStyleSolid},
			Bottom: paint.SideSpec{Width: 4, Color: red, Style: paint.BorderStyleSolid},
			Left:   paint.SideSpec{Width: 2, Color: blue, Style: paint.BorderStyleSolid},
			Right:  paint.SideSpec{Width: 3, Color: blue, Style: paint.BorderStyleSolid},
		},
	}}, 60, 60, frame.TransparentBlack)
	checks := []struct {
		x, y int
		want frame.Color
	}{
		{30, 10, red}, {30, 49, red}, {10, 30, blue}, {49, 30, blue}, {30, 30, frame.TransparentBlack},
	}
	for _, check := range checks {
		if got := out.At(check.x, check.y); got != check.want {
			t.Errorf("solid pixel (%d,%d) = %v, want %v", check.x, check.y, got, check.want)
		}
	}
}

func TestUAControlBevelPixelsAndSolidColorInput(t *testing.T) {
	html := `<!doctype html><html><body style="margin:0"><button>Go</button><br><input value="x"><br><input type="color" value="#ff0000"></body></html>`
	sess, err := engine.NewSession(html, nil, 200)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()
	dl, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	var borders []paint.DisplayCmd
	for _, cmd := range dl.Build(1).All() {
		if cmd.Kind == paint.CmdBorder {
			borders = append(borders, cmd)
		}
	}
	if len(borders) != 3 {
		t.Fatalf("got %d UA control borders, want three", len(borders))
	}
	out := bevelRasterize(t, borders, 200, 160, frame.RGB(255, 255, 255))
	light := frame.RGB(202, 202, 202)
	dark := frame.RGB(33, 33, 33)
	for i, cmd := range borders {
		x := cmd.Rect.X0 + cmd.Rect.W()/2
		y := cmd.Rect.Y0 + cmd.Rect.H()/2
		top := out.At(int(x), int(cmd.Rect.Y0))
		left := out.At(int(cmd.Rect.X0), int(y))
		right := out.At(int(cmd.Rect.X1-1), int(y))
		bottom := out.At(int(x), int(cmd.Rect.Y1-1))
		switch i {
		case 0:
			if top != light || left != light || right != dark || bottom != dark {
				t.Errorf("button bevel = top %v left %v right %v bottom %v, want light/light/dark/dark", top, left, right, bottom)
			}
		case 1:
			if top != dark || left != dark || right != light || bottom != light {
				t.Errorf("text input bevel = top %v left %v right %v bottom %v, want dark/dark/light/light", top, left, right, bottom)
			}
		case 2:
			if cmd.Border.Top.Style != paint.BorderStyleSolid || top != frame.RGB(0, 0, 0) || left != frame.RGB(0, 0, 0) || right != frame.RGB(0, 0, 0) || bottom != frame.RGB(0, 0, 0) {
				t.Errorf("solid color input changed: style %v top %v left %v right %v bottom %v", cmd.Border.Top.Style, top, left, right, bottom)
			}
		}
	}
}
