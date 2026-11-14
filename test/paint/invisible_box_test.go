package paint_test

import (
	"fmt"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

func invisibleBoxDoc(decl, shadow string) string {
	return fmt.Sprintf(`<!doctype html>
<html><head><style>html,body{margin:0;padding:0}</style></head>
<body><div style="width:0;height:0;%sbox-shadow:%s"></div>
</body></html>`, decl, shadow)
}

const ibHardRing = "0 0 0 4px rgba(0,0,0,.2)"

// qc:BR-196
func TestBoxThatCSSPaintsNothingEmitsNoCommands(t *testing.T) {
	corner := frame.Rect4(0, 0, 4, 5)

	for _, tc := range []struct {
		name           string
		decl           string
		wantCornerCmds int
		wantTotal      int
	}{
		{"opacity 0", "opacity:0;", 0, 0},
		{"visibility hidden", "visibility:hidden;", 0, 0},
		{"opacity 0.5 still paints", "opacity:0.5;", 1, 2},
		{"visible still paints", "", 1, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sess, err := engine.NewSession(invisibleBoxDoc(tc.decl, ibHardRing), nil, 1280, engine.WithViewportH(1024))
			if err != nil {
				t.Fatalf("NewSession: %v", err)
			}
			defer sess.Close()

			list, err := sess.PaintChecked(1)
			if err != nil {
				t.Fatalf("PaintChecked: %v", err)
			}
			cmds := list.Build(1).All()

			var inCorner []paint.DisplayCmd
			for _, c := range cmds {
				if b, ok := c.Bounds(); ok && b.Intersects(corner) {
					inCorner = append(inCorner, c)
				}
			}
			if len(inCorner) != tc.wantCornerCmds {
				got := make([]string, 0, len(inCorner))
				for _, c := range inCorner {
					b, _ := c.Bounds()
					got = append(got, fmt.Sprintf("%s(%d,%d,%d,%d) a=%d op=%v",
						c.Kind, b.X0, b.Y0, b.X1, b.Y1, c.Color.A(), c.Opacity))
				}
				t.Errorf("box declared %q put %d commands in the origin tile's corner, want %d: %v",
					tc.decl, len(inCorner), tc.wantCornerCmds, got)
			}
			if len(cmds) != tc.wantTotal {
				t.Errorf("box declared %q emitted %d commands in total, want %d", tc.decl, len(cmds), tc.wantTotal)
			}
			if tc.wantCornerCmds == 0 {
				return
			}
			var union frame.Rect
			for _, c := range inCorner {
				b, _ := c.Bounds()
				if union.Empty() {
					union = b
				} else {
					union = union.Union(b)
				}
			}
			if want := (frame.Rect4(-4, 0, 4, 4)); union != want {
				t.Errorf("visible shadow corner footprint = %v, want %v (the measured GOOSIE-LIVE-006 smudge shape; "+
					"Chromium reads its 16 visible pixels at (0,0)-(3,3) as (204,204,204))", union, want)
			}
		})
	}
}

// qc:BR-281
func TestZeroAreaBoxPaintsNoBlurredShadow(t *testing.T) {
	for _, tc := range []struct {
		name     string
		shadow   string
		wantCmds int
	}{
		{"blurred offset shadow paints nothing", "0 1px 4px rgba(0,0,0,.2)", 0},
		{"blurred shadow paints nothing", "0 0 8px rgba(0,0,0,.2)", 0},
		{"hard spread ring paints its two strips", ibHardRing, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sess, err := engine.NewSession(invisibleBoxDoc("", tc.shadow), nil, 1280, engine.WithViewportH(1024))
			if err != nil {
				t.Fatalf("NewSession: %v", err)
			}
			defer sess.Close()
			list, err := sess.PaintChecked(1)
			if err != nil {
				t.Fatalf("PaintChecked: %v", err)
			}
			cmds := list.Build(1).All()
			if len(cmds) != tc.wantCmds {
				var got []string
				for _, c := range cmds {
					if b, ok := c.Bounds(); ok {
						got = append(got, fmt.Sprintf("(%d,%d,%d,%d) a=%d", b.X0, b.Y0, b.X1, b.Y1, c.Color.A()))
					}
				}
				t.Errorf("zero-size box declaring %q emitted %d commands %v, want %d "+
					"(Chromium paints 0 ink pixels for a blurred one and 16 for the ring)", tc.shadow, len(cmds), got, tc.wantCmds)
			}
		})
	}
}
