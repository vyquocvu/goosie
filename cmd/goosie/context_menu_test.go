package main

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

func TestBuildContextMenuItems(t *testing.T) {
	// 1. Link click context menu
	items := BuildContextMenuItems("https://example.com/page", "", "", "", false, true, false)
	hasOpenNewTab := false
	hasCopyLink := false
	for _, it := range items {
		if it.Action == "open_new_tab" && it.Data == "https://example.com/page" {
			hasOpenNewTab = true
		}
		if it.Action == "copy_link" && it.Data == "https://example.com/page" {
			hasCopyLink = true
		}
	}
	if !hasOpenNewTab || !hasCopyLink {
		t.Fatalf("link context menu missing actions: %+v", items)
	}

	// 2. Image click context menu
	imgItems := BuildContextMenuItems("", "https://example.com/logo.png", "", "", false, false, false)
	hasOpenImgTab := false
	hasCopyImg := false
	for _, it := range imgItems {
		if it.Action == "open_image_tab" && it.Data == "https://example.com/logo.png" {
			hasOpenImgTab = true
		}
		if it.Action == "copy_image" && it.Data == "https://example.com/logo.png" {
			hasCopyImg = true
		}
	}
	if !hasOpenImgTab || !hasCopyImg {
		t.Fatalf("image context menu missing actions: %+v", imgItems)
	}

	// 3. Text selection context menu
	selItems := BuildContextMenuItems("", "", "hello world", "", false, true, true)
	hasCopy := false
	hasSearch := false
	for _, it := range selItems {
		if it.Action == "copy" && it.Data == "hello world" {
			hasCopy = true
		}
		if it.Action == "search_google" && it.Data == "hello world" {
			hasSearch = true
		}
	}
	if !hasCopy || !hasSearch {
		t.Fatalf("selection context menu missing actions: %+v", selItems)
	}

	// 4. Editable field context menu
	editItems := BuildContextMenuItems("", "", "", "input text", true, true, false)
	hasCut := false
	hasPaste := false
	hasSelectAll := false
	for _, it := range editItems {
		if it.Action == "cut" {
			hasCut = true
		}
		if it.Action == "paste" {
			hasPaste = true
		}
		if it.Action == "select_all" {
			hasSelectAll = true
		}
	}
	if !hasCut || !hasPaste || !hasSelectAll {
		t.Fatalf("editable context menu missing actions: %+v", editItems)
	}
}

func TestContextMenuGeometryAndHitTesting(t *testing.T) {
	cm := NewContextMenu()
	if cm.Visible {
		t.Fatal("context menu should be invisible initially")
	}

	items := []ContextMenuItem{
		{Label: "Option 1", Action: "act1"},
		{Separator: true},
		{Label: "Option 2", Action: "act2"},
		{Label: "Disabled", Action: "act3", Disabled: true},
	}

	scale := int32(2)
	cm.Show(100, 100, 800, 600, items, scale)
	if !cm.Visible {
		t.Fatal("context menu should be visible after Show")
	}
	rect := cm.Rect()
	if rect.Empty() || rect.W() <= 0 || rect.H() <= 0 {
		t.Fatalf("invalid menu rect: %v", rect)
	}

	// Hit Option 1
	it, ok := cm.HitItem(frame.Point{X: 110, Y: 110}, scale)
	if !ok || it.Action != "act1" {
		t.Fatalf("hit item want act1, got ok=%v it=%+v", ok, it)
	}

	// Separator should not hit
	_, okSep := cm.HitItem(frame.Point{X: 110, Y: cm.Y + int32(menuPadding*scale) + int32(menuItemHeight*scale) + 2}, scale)
	if okSep {
		t.Fatal("separator should not be clickable")
	}

	// Disabled item should not hit
	disY := cm.Y + int32(menuPadding*scale) + int32(menuItemHeight*scale) + 6*scale + int32(menuItemHeight*scale) + 4
	_, okDis := cm.HitItem(frame.Point{X: 110, Y: disY}, scale)
	if okDis {
		t.Fatal("disabled item should not be clickable")
	}

	// Outside should not hit
	_, okOut := cm.HitItem(frame.Point{X: 50, Y: 50}, scale)
	if okOut {
		t.Fatal("outside point should not hit")
	}

	// Hover update
	changed := cm.UpdateHover(frame.Point{X: 110, Y: 110}, scale)
	if !changed || cm.Hover != 0 {
		t.Fatalf("hover update failed, changed=%v hover=%d", changed, cm.Hover)
	}

	// Hide
	cm.Hide()
	if cm.Visible || !cm.Rect().Empty() {
		t.Fatal("menu should be hidden")
	}
}

func TestContextMenuRendering(t *testing.T) {
	cm := NewContextMenu()
	items := []ContextMenuItem{
		{Label: "Copy", Action: "copy"},
		{Separator: true},
		{Label: "Paste", Action: "paste"},
	}
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Skipf("no fonts: %v", err)
	}

	buf := frame.NewBitmap(400, 400)
	cm.Show(50, 50, 400, 400, items, 1)
	cm.Draw(buf, fonts, 1)

	// Context menu card background should be white/grey
	px := buf.At(55, 55)
	if px.A() != 255 {
		t.Fatalf("context menu was not drawn on bitmap: pixel=%v", px)
	}
}

func TestContextMenuInterception(t *testing.T) {
	cw2 := &capWin{}
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Skipf("no fonts: %v", err)
	}
	mgr := tabs.NewManager(nil)
	mgr.NewTab()
	tb := toolbar.NewState(800, fonts)
	cw := newChromeWindow(cw2, tb, mgr,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fonts, 1)

	var contextMenuOpened bool
	cw.onContextMenu = func(contentX, contentY, winX, winY int32) {
		contextMenuOpened = true
		cw.contextMenu.Show(winX, winY, 800, 600, []ContextMenuItem{{Label: "Action 1", Action: "act1"}}, 1)
	}

	var actionTriggered string
	cw.onContextMenuAction = func(action, data string) {
		actionTriggered = action
	}

	// 1. Right click in content area opens context menu
	rClick := surface.Event{
		Kind:   surface.EvPointer,
		Action: surface.PointerPress,
		Button: surface.ButtonRight,
		Pos:    frame.Point{X: 100, Y: 200},
	}
	consumed := cw.intercept(rClick)
	if !consumed || !contextMenuOpened || !cw.contextMenu.Visible {
		t.Fatalf("right click did not trigger context menu: consumed=%v opened=%v visible=%v", consumed, contextMenuOpened, cw.contextMenu.Visible)
	}

	// 2. Click on item in context menu triggers action and hides menu
	itemClick := surface.Event{
		Kind:   surface.EvPointer,
		Action: surface.PointerPress,
		Button: surface.ButtonLeft,
		Pos:    frame.Point{X: 110, Y: 210},
	}
	consumedClick := cw.intercept(itemClick)
	if !consumedClick || actionTriggered != "act1" || cw.contextMenu.Visible {
		t.Fatalf("context menu click item failed: consumed=%v action=%s visible=%v", consumedClick, actionTriggered, cw.contextMenu.Visible)
	}

	// 3. Re-open and press Escape to dismiss
	cw.contextMenu.Show(100, 100, 800, 600, []ContextMenuItem{{Label: "Action 1", Action: "act1"}}, 1)
	escKey := surface.Event{
		Kind: surface.EvKey,
		Key:  0x1B, // Escape
	}
	consumedEsc := cw.intercept(escKey)
	if !consumedEsc || cw.contextMenu.Visible {
		t.Fatalf("escape did not close context menu: consumed=%v visible=%v", consumedEsc, cw.contextMenu.Visible)
	}
}
