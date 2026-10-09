package tabs_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/tabs"
)

func TestTabRectSingleTab(t *testing.T) {
	r := tabs.TabRect(0, 0, 1, 1000)
	if r.X0 != int32(tabs.LeftMargin) {
		t.Fatalf("first tab X0 should be LeftMargin (%d), got %d", tabs.LeftMargin, r.X0)
	}
	if r.W() != int32(tabs.TabWidth) {
		t.Fatalf("tab width should be %d, got %d", tabs.TabWidth, r.W())
	}
	if r.Y0 != 0 || r.Y1 != int32(tabs.TabBarHeight) {
		t.Fatalf("tab should span full tab bar height, got Y0=%d Y1=%d", r.Y0, r.Y1)
	}
}

func TestTabRectMultipleTabs(t *testing.T) {
	r0 := tabs.TabRect(0, 0, 3, 1000)
	r1 := tabs.TabRect(1, 0, 3, 1000)
	r2 := tabs.TabRect(2, 0, 3, 1000)

	if r0.X0 != int32(tabs.LeftMargin) {
		t.Fatalf("tab 0 X0 should be LeftMargin, got %d", r0.X0)
	}
	if r1.X0 != r0.X1+int32(tabs.TabGap) {
		t.Fatalf("tab 1 should start after tab 0 + gap, got %d", r1.X0)
	}
	if r2.X0 != r1.X1+int32(tabs.TabGap) {
		t.Fatalf("tab 2 should start after tab 1 + gap, got %d", r2.X0)
	}
}

func TestNewTabButtonRect(t *testing.T) {
	tabCount := 2
	contentWidth := int32(1000)
	r := tabs.NewTabButtonRect(int32(tabCount), 0, contentWidth)
	if r.W() != int32(tabs.NewTabBtnSize) {
		t.Fatalf("new tab button width should be %d, got %d", tabs.NewTabBtnSize, r.W())
	}
	lastTab := tabs.TabRect(tabCount-1, 0, int32(tabCount), contentWidth)
	if r.X0 != lastTab.X1+int32(tabs.TabGap) {
		t.Fatalf("new tab button should be after last tab + gap, got X0=%d expected %d", r.X0, lastTab.X1+int32(tabs.TabGap))
	}
}

func TestCloseButtonRect(t *testing.T) {
	tabR := tabs.TabRect(0, 0, 1, 1000)
	closeR := tabs.CloseButtonRect(tabR)
	if closeR.W() != int32(tabs.CloseBtnSize) || closeR.H() != int32(tabs.CloseBtnSize) {
		t.Fatalf("close button should be %dx%d, got %dx%d", tabs.CloseBtnSize, tabs.CloseBtnSize, closeR.W(), closeR.H())
	}
	if closeR.X1 > tabR.X1-int32(tabs.CloseBtnMargin) {
		t.Fatal("close button should be inside the tab with margin")
	}
}

func TestTotalChromeHeight(t *testing.T) {
	expected := tabs.TabBarHeight + 40
	if expected != 76 {
		t.Fatalf("total chrome should be 76px, got %d", expected)
	}
}

// Ten tabs at 1000px cannot fit at full width: the strip must shrink every tab
// uniformly so the last tab and the new-tab button stay on screen and every
// tab stays reachable. Fixed 200px tabs run off the bitmap and strand the
// button beyond the clickable area.
func TestTabStripShrinksToFit(t *testing.T) {
	const tabCount, width = 10, 1000
	var prevX1 int32
	for i := 0; i < tabCount; i++ {
		r := tabs.TabRect(i, 0, tabCount, width)
		if r.W() >= int32(tabs.TabWidth) {
			t.Fatalf("tab %d width = %d, want shrunk below %d", i, r.W(), tabs.TabWidth)
		}
		if i == 0 && r.X0 != int32(tabs.LeftMargin) {
			t.Fatalf("tab 0 X0 = %d, want LeftMargin %d", r.X0, tabs.LeftMargin)
		}
		if i > 0 && r.X0 != prevX1+int32(tabs.TabGap) {
			t.Fatalf("tab %d X0 = %d, want %d (contiguous)", i, r.X0, prevX1+int32(tabs.TabGap))
		}
		prevX1 = r.X1
	}
	btn := tabs.NewTabButtonRect(tabCount, 0, width)
	if btn.X1 > width {
		t.Fatalf("new-tab button ends at %d, beyond clickable width %d", btn.X1, width)
	}
	last := tabs.TabRect(tabCount-1, 0, tabCount, width)
	if btn.X0 != last.X1+int32(tabs.TabGap) {
		t.Fatalf("new-tab button X0 = %d, want %d (after last tab + gap)", btn.X0, last.X1+int32(tabs.TabGap))
	}
}

// Shrinking has a floor: tabs must stay wide enough for an ellipsis and the
// close affordance instead of collapsing to slivers.
func TestTabStripMinWidthClamp(t *testing.T) {
	const tabCount, width = 40, 1000
	for i := 0; i < tabCount; i++ {
		if got := tabs.TabRect(i, 0, tabCount, width).W(); got != int32(tabs.MinTabWidth) {
			t.Fatalf("tab %d width = %d, want MinTabWidth %d", i, got, tabs.MinTabWidth)
		}
	}
}

// StripMaxOffset is zero while the strip fits and the clipped excess once
// tabs sit at MinTabWidth: it is the only legal scroll range.
func TestStripMaxOffset(t *testing.T) {
	if got := tabs.StripMaxOffset(3, 1000); got != 0 {
		t.Fatalf("StripMaxOffset(3, 1000) = %d, want 0 (fits)", got)
	}
	if got := tabs.StripMaxOffset(10, 1000); got != 0 {
		t.Fatalf("StripMaxOffset(10, 1000) = %d, want 0 (shrinks to fit)", got)
	}
	// 40 tabs at MinTabWidth 72: 8 + 40*73 + 32 - (1000-56) = 2016. The max
	// leaves the new-tab button visible left of the chevron zone.
	if got := tabs.StripMaxOffset(40, 1000); got != 2016 {
		t.Fatalf("StripMaxOffset(40, 1000) = %d, want 2016", got)
	}
}

// EnsureTabVisible scrolls a clipped strip just enough to show idx: the last
// tab pins the offset at the maximum, the first resets it, and a visible tab
// leaves the offset alone.
func TestEnsureTabVisible(t *testing.T) {
	const tabCount, width = 40, 1000
	if got := tabs.EnsureTabVisible(39, tabCount, width, 0); got != 2016 {
		t.Fatalf("EnsureTabVisible(last) = %d, want 2016 (scroll to end)", got)
	}
	if got := tabs.EnsureTabVisible(0, tabCount, width, 1960); got != 0 {
		t.Fatalf("EnsureTabVisible(first) = %d, want 0", got)
	}
	if got := tabs.EnsureTabVisible(5, tabCount, width, 0); got != 0 {
		t.Fatalf("EnsureTabVisible(visible) = %d, want 0 (leave alone)", got)
	}
	if got := tabs.EnsureTabVisible(2, 3, 1000, 0); got != 0 {
		t.Fatalf("EnsureTabVisible(fitting) = %d, want 0", got)
	}
}

// Chevrons appear exactly when the strip overflows: two 24px buttons pinned
// to the strip's right end. A fitting strip has none, so the zone stays tabs.
func TestChevronRects(t *testing.T) {
	if _, _, ok := tabs.ChevronRects(3, 1000); ok {
		t.Fatal("ChevronRects(3, 1000) shown, want hidden (fits)")
	}
	left, right, ok := tabs.ChevronRects(40, 1000)
	if !ok {
		t.Fatal("ChevronRects(40, 1000) hidden, want shown (overflows)")
	}
	// Right button: 4px margin; 24px buttons with a 4px gap.
	if right.X1 != 1000-4 || right.W() != 24 {
		t.Fatalf("right chevron = %+v, want 24px ending 4px from the edge", right)
	}
	if left.X1 != right.X0-4 || left.W() != 24 {
		t.Fatalf("left chevron = %+v, want 24px gapped 4px before right", left)
	}
	if left.Y0 != 6 || left.Y1 != 30 || right.Y0 != 6 || right.Y1 != 30 {
		t.Fatalf("chevrons y = [%d,%d]/[%d,%d], want [6,30] centered in the 36px bar",
			left.Y0, left.Y1, right.Y0, right.Y1)
	}
}

// A chevron page keeps one tab of context: visible strip minus one tab,
// floored at one tab so a narrow strip still moves.
func TestTabScrollPage(t *testing.T) {
	// 40 tabs at 1000: visible 1000-8-56, minus one 72px tab.
	if got := tabs.TabScrollPage(40, 1000); got != 864 {
		t.Fatalf("TabScrollPage(40, 1000) = %d, want 864", got)
	}
}
