package paint_test

import (
	"fmt"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

// This is the oracle for GOOSIE-LIVE-006, the 20-32 px ink smudge in the
// top-left corner of Goosie's renders of live pages.
//
// The shape that produced it, measured on a saved copy of vnexpress.net: an
// inline box with a zero-size border rect at content (0,0), hidden by CSS, and
// carrying the page's own dropdown rule `box-shadow: 0 1px 4px rgba(0,0,0,.2)`.
// paintBoxShadow approximates the 4px blur with two concentric rings, and for a
// box with no height only the top and bottom strips of each ring are non-empty,
// so the box emits four fills. Two such boxes on the page put eight fills over
// (-4,-3)-(4,5); both top strips lie entirely above the origin, so it is the
// four bottom strips - two per box - that overlap into the 4x5 px footprint at
// the corner, and that footprint is exactly the 20 px the parity harness
// counted as a content band
// (docs/tasks/TASK-039-LIVE-PAGE-NO-OUTPUT.md §5.5, branch F).
//
// They painted at the shadow's full alpha instead of the blur's attenuation
// because both boxes computed `opacity: 0`, the builder carried that onto the
// commands, and DisplayCmd.Opacity 0 means "unset, draw opaque" (paint/cmd.go).
//
// The owning stage is the display list: the boxes that painted it were
// `.sub-area` and `.nav-folder .ul-nav-folder li.has-submn .sub-more`, both
// declared `opacity: 0; visibility: hidden`. A box CSS says paints nothing must
// contribute no command at all, so that - not a pixel count - is what this
// asserts. Asserting downstream would let the same list keep emitting ink that
// a filter has to remember to remove.
//
// The two control cases hold the fix to its real width. A partially transparent
// and a fully visible box must still paint: a suppression that rounds "any
// opacity", "any clip" or "any zero-size box" down to nothing fails here as
// loudly as no suppression at all. The visible control is also the proof that
// the geometry above is the true footprint and not an artefact of the test.

// invisibleBoxDoc renders one zero-size box at the content origin with an extra
// declaration folded in ahead of the shadow that made the smudge.
func invisibleBoxDoc(decl string) string {
	return fmt.Sprintf(`<!doctype html>
<html><head><style>html,body{margin:0;padding:0}</style></head>
<body><div style="width:0;height:0;%sbox-shadow:0 1px 4px rgba(0,0,0,.2)"></div>
</body></html>`, decl)
}

// qc:BR-196
func TestBoxThatCSSPaintsNothingEmitsNoCommands(t *testing.T) {
	// corner is what the tile keeps of the shadow's footprint at the origin: the
	// 4x5 px smudge, nothing more.
	corner := frame.Rect4(0, 0, 4, 5)

	for _, tc := range []struct {
		name string
		decl string
		// wantCornerCmds is how many commands the box must put into the tile's
		// corner: 0 when CSS says it paints nothing. A visible box's shadow emits
		// four fills (two rings x top+bottom strips) but both top strips lie
		// entirely at y<0, so only the two bottom strips reach the corner - and
		// they are the whole smudge.
		wantCornerCmds int
		wantTotal      int
	}{
		{"opacity 0", "opacity:0;", 0, 0},
		{"visibility hidden", "visibility:hidden;", 0, 0},
		{"opacity 0.5 still paints", "opacity:0.5;", 2, 4},
		{"visible still paints", "", 2, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sess, err := engine.NewSession(invisibleBoxDoc(tc.decl), nil, 1280, engine.WithViewportH(1024))
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
			// Control: the corner footprint must be the measured smudge shape, so
			// the 0-cases above are compared against a real reproduction and not
			// against a document that never painted anything to begin with.
			var union frame.Rect
			for _, c := range inCorner {
				b, _ := c.Bounds()
				if union.Empty() {
					union = b
				} else {
					union = union.Union(b)
				}
			}
			if want := (frame.Rect4(-4, 0, 4, 5)); union != want {
				t.Errorf("visible shadow corner footprint = %v, want %v (the measured GOOSIE-LIVE-006 smudge shape)", union, want)
			}
		})
	}
}
