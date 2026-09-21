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
