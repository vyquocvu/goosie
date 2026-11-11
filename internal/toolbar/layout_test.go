package toolbar

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
)

func rectsOverlap(a, b frame.Rect) bool {
	return a.X0 < b.X1 && b.X0 < a.X1 && a.Y0 < b.Y1 && b.Y0 < a.Y1
}

// TestComputeLayoutNeverOverlapsOrInverts sweeps widths that break the old
// fixed arithmetic (address inverts < 152, downloads overlaps < 144) and asserts
// the responsive contract: menu always present, address non-inverted with a
// minimum width, no pairwise overlap, everything on-screen.
func TestComputeLayoutNeverOverlapsOrInverts(t *testing.T) {
	for _, w := range []int32{120, 144, 152, 200, 320, 640, 1280} {
		l := ComputeLayout(w)
		if l.Menu.X1 <= l.Menu.X0 {
			t.Fatalf("width %d: menu inverted %+v", w, l.Menu)
		}
		if l.Menu.X0 < 0 || l.Menu.X1 > w {
			t.Fatalf("width %d: menu off-screen %+v", w, l.Menu)
		}
		if l.Address.X1 <= l.Address.X0 {
			t.Fatalf("width %d: address inverted %+v", w, l.Address)
		}
		if l.Address.W() < MinAddressWidth {
			t.Fatalf("width %d: address width %d < min %d", w, l.Address.W(), MinAddressWidth)
		}
		all := append([]frame.Rect{l.Address, l.Menu}, rectList(l)...)
		for _, r := range all {
			if r.X0 < 0 || r.X1 > w {
				t.Fatalf("width %d: rect off-screen %+v", w, r)
			}
		}
		for i := 0; i < len(all); i++ {
			for j := i + 1; j < len(all); j++ {
				if rectsOverlap(all[i], all[j]) {
					t.Fatalf("width %d: overlap %+v vs %+v", w, all[i], all[j])
				}
			}
		}
		if _, ok := l.Rects[ButtonMenu]; !ok {
			t.Fatalf("width %d: menu button missing from layout", w)
		}
	}
}

func rectList(l Layout) []frame.Rect {
	out := make([]frame.Rect, 0, len(l.Buttons))
	for _, b := range l.Buttons {
		if b == ButtonMenu {
			continue
		}
		out = append(out, l.Rects[b])
	}
	return out
}

// TestComputeLayoutKeepsAllControlsAtNormalWidth pins the desktop layout so the
// responsive change does not regress the common case.
func TestComputeLayoutKeepsAllControlsAtNormalWidth(t *testing.T) {
	l := ComputeLayout(800)
	for _, b := range []Button{ButtonBack, ButtonForward, ButtonReload, ButtonDownloads, ButtonMenu} {
		if _, ok := l.Rects[b]; !ok {
			t.Fatalf("width 800: button %d collapsed, want visible", b)
		}
	}
	if got := l.Rects[ButtonBack]; got.X0 != 4 {
		t.Fatalf("back X0 = %d, want 4", got.X0)
	}
	if l.Address.X0 != 112 {
		t.Fatalf("address X0 = %d, want 112", l.Address.X0)
	}
}

// TestComputeLayoutCollapsesIntoMenu verifies the overflow priority: Downloads
// hides before the left cluster, then Reload, Forward, Back.
func TestComputeLayoutCollapsesIntoMenu(t *testing.T) {
	l := ComputeLayout(120)
	for _, b := range []Button{ButtonBack, ButtonForward, ButtonReload, ButtonDownloads} {
		if _, ok := l.Rects[b]; ok {
			t.Fatalf("width 120: button %d still visible, want collapsed into menu", b)
		}
	}
	if _, ok := l.Rects[ButtonMenu]; !ok {
		t.Fatal("width 120: menu must remain")
	}
}