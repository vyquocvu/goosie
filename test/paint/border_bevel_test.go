package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

func TestBorderBevelStylesReachTheDisplayList(t *testing.T) {
	cmds := ibBorders(ibDisplayList(t,
		`div{box-sizing:border-box;width:120px;height:60px;border-width:1px 2px 3px 4px;border-style:inset outset ridge groove;border-color:rgb(200,80,40) rgb(40,80,200) rgb(80,160,60) rgb(160,40,120)}`,
		`<div></div>`, 320))
	if len(cmds) != 1 {
		t.Fatalf("got %d border commands, want one", len(cmds))
	}
	border := cmds[0].Border
	widths := [4]int32{border.Top.Width, border.Right.Width, border.Bottom.Width, border.Left.Width}
	styles := [4]paint.BorderStyle{border.Top.Style, border.Right.Style, border.Bottom.Style, border.Left.Style}
	colours := [4]frame.Color{border.Top.Color, border.Right.Color, border.Bottom.Color, border.Left.Color}
	wantWidths := [4]int32{1, 2, 3, 4}
	wantStyles := [4]paint.BorderStyle{paint.BorderStyleInset, paint.BorderStyleOutset, paint.BorderStyleRidge, paint.BorderStyleGroove}
	wantColours := [4]frame.Color{frame.RGB(200, 80, 40), frame.RGB(40, 80, 200), frame.RGB(80, 160, 60), frame.RGB(160, 40, 120)}
	for i := range widths {
		if widths[i] != wantWidths[i] || styles[i] != wantStyles[i] || colours[i] != wantColours[i] {
			t.Errorf("side %d = width %d style %v colour %v, want width %d style %v colour %v", i,
				widths[i], styles[i], colours[i], wantWidths[i], wantStyles[i], wantColours[i])
		}
	}
}

func TestUAControlBorderStylesReachTheDisplayList(t *testing.T) {
	cmds := ibBorders(ibDisplayList(t, ``,
		`<button>Go</button><br><input value="x"><br><input type="color" value="#ff0000">`, 320))
	if len(cmds) != 3 {
		t.Fatalf("got %d control border commands, want three", len(cmds))
	}
	wantStyles := [4]paint.BorderStyle{paint.BorderStyleOutset, paint.BorderStyleInset, paint.BorderStyleSolid}
	wantWidths := [4]int32{2, 2, 1}
	wantColours := [4]frame.Color{frame.RGB(118, 118, 118), frame.RGB(118, 118, 118), frame.RGB(0, 0, 0)}
	for i, cmd := range cmds {
		side := cmd.Border.Top
		if side.Style != wantStyles[i] || side.Width != wantWidths[i] || side.Color != wantColours[i] {
			t.Errorf("control %d top border = width %d style %v colour %v, want width %d style %v colour %v", i,
				side.Width, side.Style, side.Color, wantWidths[i], wantStyles[i], wantColours[i])
		}
	}
}

func TestAuthorBorderStylesOverrideControlUA(t *testing.T) {
	for _, tc := range []struct {
		name  string
		style string
		want  paint.BorderStyle
	}{
		{"inset", "inset", paint.BorderStyleInset},
		{"outset", "outset", paint.BorderStyleOutset},
		{"ridge", "ridge", paint.BorderStyleRidge},
		{"groove", "groove", paint.BorderStyleGroove},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmds := ibBorders(ibDisplayList(t,
				`button,input,input[type=color]{border-style:`+tc.style+`;border-color:rgb(200,80,40)}`,
				`<button>Go</button><br><input value="x"><br><input type="color" value="#ff0000">`, 320))
			if len(cmds) != 3 {
				t.Fatalf("got %d border commands, want three", len(cmds))
			}
			for i, cmd := range cmds {
				if cmd.Border.Top.Style != tc.want || cmd.Border.Top.Color != frame.RGB(200, 80, 40) {
					t.Errorf("control %d kept top border %v/%v, want %v/rgb(200,80,40)", i, cmd.Border.Top.Style, cmd.Border.Top.Color, tc.want)
				}
			}
		})
	}
}

func TestSolidBorderCommandsRemainFlat(t *testing.T) {
	cmds := ibBorders(ibDisplayList(t,
		`div{width:120px;height:60px;border:4px solid rgba(200,80,40,.5)}`, `<div></div>`, 320))
	if len(cmds) != 1 {
		t.Fatalf("got %d border commands, want one", len(cmds))
	}
	cmd := cmds[0]
	for i, side := range [4]paint.SideSpec{cmd.Border.Top, cmd.Border.Right, cmd.Border.Bottom, cmd.Border.Left} {
		if side.Style != paint.BorderStyleSolid || side.Color != frame.RGBA(200, 80, 40, 128) || side.Width != 4 {
			t.Errorf("solid side %d changed to %+v", i, side)
		}
	}
}

func TestCollapsedTableDelegatesBevelStyle(t *testing.T) {
	cmds := ibBorders(ibDisplayList(t,
		`table{border-collapse:collapse}td{border:2px inset rgb(51,51,51)}`,
		`<table><tr><td>a</td><td>b</td></tr><tr><td>c</td><td>d</td></tr></table>`, 320))
	var right, bottom bool
	for _, cmd := range cmds {
		if cmd.Border.Right.Width > 0 {
			right = true
			if cmd.Border.Right.Style != paint.BorderStyleInset {
				t.Errorf("collapsed right style = %v, want inset", cmd.Border.Right.Style)
			}
		}
		if cmd.Border.Bottom.Width > 0 {
			bottom = true
			if cmd.Border.Bottom.Style != paint.BorderStyleInset {
				t.Errorf("collapsed bottom style = %v, want inset", cmd.Border.Bottom.Style)
			}
		}
	}
	if !right || !bottom {
		t.Fatalf("collapsed table delegated edges right=%v bottom=%v", right, bottom)
	}
}

func TestInlineFragmentsCarryBevelStyles(t *testing.T) {
	cmds := ibBorders(ibDisplayList(t,
		`div{width:80px}span{border:3px outset rgb(200,80,40)}`,
		`<div><span>aa bb cc dd ee ff</span></div>`, 100))
	if len(cmds) < 2 {
		t.Fatalf("got %d inline border fragments, want at least two", len(cmds))
	}
	for i, cmd := range cmds {
		if cmd.Border.Top.Style != paint.BorderStyleOutset || cmd.Border.Bottom.Style != paint.BorderStyleOutset {
			t.Errorf("fragment %d lost its top/bottom outset style", i)
		}
	}
	if cmds[0].Border.Left.Style != paint.BorderStyleOutset || cmds[len(cmds)-1].Border.Right.Style != paint.BorderStyleOutset {
		t.Error("inline start/end fragments lost their outset style")
	}
}

func TestDashedAndDottedBordersKeepTheExistingFlatFallback(t *testing.T) {
	for _, style := range []string{"dashed", "dotted"} {
		cmds := ibBorders(ibDisplayList(t, `div{border:4px `+style+` rgb(200,80,40)}`, `<div></div>`, 320))
		if len(cmds) != 1 {
			t.Fatalf("%s emitted %d border commands, want one", style, len(cmds))
		}
		for i, side := range [4]paint.SideSpec{cmds[0].Border.Top, cmds[0].Border.Right, cmds[0].Border.Bottom, cmds[0].Border.Left} {
			if side.Style != paint.BorderStyleSolid || side.Color != frame.RGB(200, 80, 40) {
				t.Errorf("%s side %d changed to %+v", style, i, side)
			}
		}
	}
}
