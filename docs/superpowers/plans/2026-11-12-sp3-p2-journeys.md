# SP-3 — P2 Journeys Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the three remaining P2 NOT RUN rows — BR-052 (main menu → bookmark/history), BR-143 (responsive toolbar, no overlap on resize), BR-142 (inspector node highlight without DOM mutation).

**Architecture:** One geometry owner (`toolbar.ComputeLayout`) drives paint, hit-test and accessibility; a right-anchored `ButtonMenu` opens a reused `ContextMenu` that doubles as the overflow affordance. The inspector highlight is a post-walk paint overlay (validation-bubble precedent) stored on the Session, never a DOM write.

**Tech Stack:** Go, `internal/toolbar`, `internal/paint`, `internal/engine`, `cmd/goosie`, `platform/headless` journey harness.

**Spec:** `docs/superpowers/specs/2026-11-12-sp3-p2-journeys-design.md` (commit `77b3a26f`).

**Conventions:** TDD red→green; each oracle gets a mutant that must go red then restored. `gofmt` clean (archtest+gate enforce). Never pipe test output through `tail` (masks exit codes) — redirect to a log and read it. Commits on `main`, no push. Session events dispatch outside `opMu`; toolbar callbacks fire after `State.mu` unlock via `pending`.

---

## Task 1: Responsive toolbar geometry owner (BR-143 foundation)

**Files:**
- Modify: `internal/toolbar/layout.go` (whole file, 39 lines)
- Test: `internal/toolbar/layout_test.go` (new)

- [ ] **Step 1: Write the failing geometry-invariant test**

Create `internal/toolbar/layout_test.go`:

```go
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
		// Menu is always a visible, hit-testable button.
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
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/toolbar/ -run TestComputeLayout -count=1 > /tmp/sp3-t1-red.log 2>&1; echo exit=$?`
Expected: FAIL — `ComputeLayout`/`Layout`/`MinAddressWidth`/`ButtonMenu` undefined (build failure).

- [ ] **Step 3: Implement the geometry owner**

Replace `internal/toolbar/layout.go` with:

```go
package toolbar

import "github.com/vyquocvu/goosie/internal/frame"

const (
	ToolbarHeight = 40
	ButtonSize    = 32
	ButtonGap     = 4
	BarPadding    = 8
	// MinAddressWidth is the narrowest the address bar may become before the
	// toolbar collapses buttons into the main menu instead of shrinking it.
	MinAddressWidth = 64
)

type Button int

const (
	ButtonBack Button = iota
	ButtonForward
	ButtonReload
	ButtonDownloads
	ButtonMenu
	ButtonGo = ButtonDownloads
)

// Layout is the single source of truth for toolbar geometry at a given width.
// Buttons lists the visible buttons in paint order; Rects maps each visible
// button (always including ButtonMenu) to its rect. Address is the URL field.
type Layout struct {
	Buttons []Button
	Rects   map[Button]frame.Rect
	Address frame.Rect
	Menu    frame.Rect
}

// ComputeLayout lays the toolbar out for width. The menu button is always
// right-anchored. As width shrinks, Downloads collapses first, then Reload,
// Forward, Back — in that order — so the address bar keeps MinAddressWidth and
// no two controls overlap. Collapsed actions live in the main menu.
func ComputeLayout(width int32) Layout {
	y := int32((ToolbarHeight - ButtonSize) / 2)
	box := func(x0 int32) frame.Rect { return frame.Rect4(x0, y, x0+ButtonSize, y+ButtonSize) }
	menuX0 := width - ButtonSize - ButtonGap
	if menuX0 < 0 {
		menuX0 = 0
	}
	menu := box(menuX0)

	leftOrder := []Button{ButtonBack, ButtonForward, ButtonReload}
	type cand struct {
		left      int
		downloads bool
	}
	// Richest first; Downloads collapses before any left button.
	cands := []cand{{3, true}, {3, false}, {2, false}, {1, false}, {0, false}}
	for _, c := range cands {
		addrX1 := menuX0 - ButtonGap
		if c.downloads {
			addrX1 = menuX0 - ButtonSize - 2*ButtonGap
		}
		addrX0 := ButtonGap + int32(c.left)*(ButtonSize+ButtonGap)
		if addrX1-addrX0 < MinAddressWidth {
			continue
		}
		l := Layout{Rects: make(map[Button]frame.Rect), Menu: menu}
		for i := 0; i < c.left; i++ {
			b := leftOrder[i]
			l.Buttons = append(l.Buttons, b)
			l.Rects[b] = box(ButtonGap + int32(i)*(ButtonSize+ButtonGap))
		}
		l.Address = frame.Rect4(addrX0, y, addrX1, y+ButtonSize)
		if c.downloads {
			l.Buttons = append(l.Buttons, ButtonDownloads)
			l.Rects[ButtonDownloads] = box(menuX0 - ButtonSize - ButtonGap)
		}
		l.Buttons = append(l.Buttons, ButtonMenu)
		l.Rects[ButtonMenu] = menu
		return l
	}
	// Degenerate width: keep menu and a non-inverted address, no buttons.
	addrX1 := menuX0 - ButtonGap
	if addrX1 < ButtonGap+1 {
		addrX1 = ButtonGap + 1
	}
	return Layout{
		Buttons: []Button{ButtonMenu},
		Rects:   map[Button]frame.Rect{ButtonMenu: menu},
		Address: frame.Rect4(ButtonGap, y, addrX1, y+ButtonSize),
		Menu:    menu,
	}
}

// ButtonRect returns the rect of a visible button, or the zero rect when the
// button is collapsed at this width. Callers that paint or hit-test should
// iterate ComputeLayout().Buttons rather than probe collapsed buttons.
func ButtonRect(btn Button, toolbarWidth int32) frame.Rect {
	return ComputeLayout(toolbarWidth).Rects[btn]
}

// AddressBarRect returns the address field rect for the width.
func AddressBarRect(toolbarWidth int32) frame.Rect {
	return ComputeLayout(toolbarWidth).Address
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/toolbar/ -run TestComputeLayout -count=1 > /tmp/sp3-t1-green.log 2>&1; echo exit=$?`
Expected: PASS (exit=0).

- [ ] **Step 5: Run the whole toolbar package (existing tests may need rect updates)**

Run: `go test ./internal/toolbar/ -count=1 > /tmp/sp3-t1-pkg.log 2>&1; echo exit=$?`
If a test hardcodes the old Downloads rect (e.g. `toolbar_test.go:406`), read it; if it computes the rect dynamically it stays green. Update only hardcoded assertions to the new layout (Downloads now sits left of Menu) and document the change in the commit body. Expected: PASS.

- [ ] **Step 6: Mutant check**

Temporarily change `MinAddressWidth` to `0` in `layout.go`; run `go test ./internal/toolbar/ -run TestComputeLayoutNeverOverlapsOrInverts -count=1`. Expected: FAIL (overlap/inversion returns at small widths). Restore `64`; re-run green.

- [ ] **Step 7: Commit**

```bash
git add internal/toolbar/layout.go internal/toolbar/layout_test.go
git commit -m "feat(toolbar): own responsive geometry in ComputeLayout (BR-143)"
```

---

## Task 2: Main menu button + ContextMenu (BR-052)

**Files:**
- Modify: `internal/toolbar/toolbar.go` (State callback, Draw, drawButton, handlePointerLocked)
- Modify: `cmd/goosie/toolbar_window.go` (struct field, Present, intercept, accessibilityChrome)
- Modify: `cmd/goosie/main.go` (wire OnMenu)
- Modify: `cmd/goosie/interaction.go` (handleContextMenuAction cases)
- Test: `cmd/goosie/journey_main_menu_test.go` (new)

- [ ] **Step 1: Write the failing headless journey test**

Create `cmd/goosie/journey_main_menu_test.go`. Read `cmd/goosie/e2e_test.go` first for the exact `newBrowserE2E`, `inject`, `waitFor`, `clickToolbarButton`, `activeURL` helper signatures; mirror them. The test:

```go
package main

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

// TestJourneyMainMenuOpensManagers drives the chrome menu: press the menu
// button, click Bookmarks, then History, and assert the active tab navigates to
// the internal managers (BR-052).
func TestJourneyMainMenuOpensManagers(t *testing.T) {
	b := newBrowserE2E(t)
	b.navigateViaAddressBar(t, "https://example.com/")
	b.waitFor(t, "example loaded", func() bool { return b.activeURL() == "https://example.com/" })

	// Open the main menu via the toolbar button.
	b.clickToolbarButton(t, toolbar.ButtonMenu)
	b.waitFor(t, "main menu visible", func() bool { return b.f.mainMenu != nil && b.f.mainMenu.Visible })

	// Click the Bookmarks item.
	it, ok := b.f.mainMenu.HitItem(menuItemPoint(t, b.f.mainMenu, "Bookmarks"), b.f.devScale())
	if !ok || it.Action != "bookmarks" {
		t.Fatalf("Bookmarks item not hit: %+v ok=%v", it, ok)
	}
	b.inject(t, surface.Event{Kind: surface.EvPointer, Point: menuItemPoint(t, b.f.mainMenu, "Bookmarks"), Button: surface.ButtonLeft, Action: surface.ActionPress})
	b.waitFor(t, "bookmarks opened", func() bool { return b.activeURL() == "goosie://bookmarks" })

	// Reopen and click History.
	b.clickToolbarButton(t, toolbar.ButtonMenu)
	b.waitFor(t, "main menu visible again", func() bool { return b.f.mainMenu.Visible })
	b.inject(t, surface.Event{Kind: surface.EvPointer, Point: menuItemPoint(t, b.f.mainMenu, "History"), Button: surface.ButtonLeft, Action: surface.ActionPress})
	b.waitFor(t, "history opened", func() bool { return b.activeURL() == "goosie://history" })
}
```

Add helpers `menuItemPoint` (compute the center of a labeled item from `ContextMenu.Y`/`menuItemHeight`/`menuPadding`/scale) and confirm `b.f.devScale()`/`b.activeURL()` exist (read e2e_test.go; add a small accessor if missing). Adjust the pointer event field names to the actual `surface.Event`/`surface.PointerEvent` shape used by existing journey tests (read `cmd/goosie/journey_find_test.go` for the exact press injection pattern). The test must compile against the real harness; fix signatures from the read, do not guess.

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./cmd/goosie/ -run TestJourneyMainMenuOpensManagers -count=1 > /tmp/sp3-t2-red.log 2>&1; echo exit=$?`
Expected: FAIL — `toolbar.ButtonMenu` has no menu behavior / `f.mainMenu` undefined.

- [ ] **Step 3: Add the toolbar menu callback + icon + hit-test**

In `internal/toolbar/toolbar.go`:
- Add field `OnMenu func()` to `State` (near `OnDownloads`, line ~256).
- In `Draw` (line 1254-1258), replace the four fixed `drawButton` calls with a single layout pass:

```go
lay := ComputeLayout(w)
for _, btn := range lay.Buttons {
	s.drawButton(backing, btn, w, clip, oy)
}
s.drawAddressBar(backing, w, clip, oy)
```

- In `drawButton` switch (line 1292), add:

```go
case ButtonMenu:
	s.drawMenuIcon(backing, ox, oyd, cx, cy, sc, fg, clip)
```

- Add `drawMenuIcon` (three horizontal bars) mirroring `drawDownloads`'s use of `drawLineAA`:

```go
func (s *State) drawMenuIcon(backing *frame.Bitmap, ox, oy, cx, cy, sc int32, c frame.Color, clip frame.Rect) {
	fcx := float64(ox) + float64(cx)*float64(sc)
	fcy := float64(oy) + float64(cy)*float64(sc)
	thick := 1.8 * float64(sc)
	if thick < 1.5 {
		thick = 1.5
	}
	halfW := 7.0 * float64(sc)
	for _, dy := range []float64{-5, 0, 5} {
		y := fcy + dy*float64(sc)
		drawLineAA(backing, fcx-halfW, y, fcx+halfW, y, thick, c)
	}
}
```

- In `handlePointerLocked` (line ~817-931), add a `ButtonMenu` hit case alongside the other `rectContains(ButtonRect(...))` cases, appending `s.OnMenu` to `pending` (never call it under the lock):

```go
case rectContains(ButtonRect(ButtonMenu, w), lp):
	if s.OnMenu != nil {
		pending = append(pending, s.OnMenu)
	}
```

Read the exact `pending` variable name and switch structure in `handlePointerLocked` before editing; match the existing `OnDownloads` case style.

- [ ] **Step 4: Add the chrome main-menu instance + draw + hit-test**

In `cmd/goosie/toolbar_window.go`:
- Add field `mainMenu *ContextMenu` to `chromeWindow` (near `contextMenu`, line ~25-106). Initialize it in `newChromeWindow` (`mainMenu: NewContextMenu()`).
- Add a method to open it, anchored to the menu button:

```go
func (cw *chromeWindow) openMainMenu() {
	sc := cw.scale
	w := cw.toolbar.BoundsWidth() / sc
	r := toolbar.ButtonRect(toolbar.ButtonMenu, w)
	items := []ContextMenuItem{
		{Label: "Bookmarks", Action: "bookmarks"},
		{Label: "History", Action: "history"},
		{Label: "Downloads", Action: "downloads"},
		{Label: "Settings", Action: "settings"},
		{Separator: true},
		{Label: "Back", Action: "back"},
		{Label: "Forward", Action: "forward"},
		{Label: "Reload", Action: "reload"},
	}
	x := r.X0 * sc
	y := (tabs.TabBarHeight + r.Y1) * sc
	cw.mainMenu.Show(x, y, cw.windowExtent().W, cw.windowExtent().H, items, sc)
}
```

Read `windowExtent()` and the `scale` field name; match existing `contextMenu.Show` call sites for the exact maxW/maxH source.
- In `Present` (line ~468-474), after the existing `contextMenu.Draw`, add `cw.mainMenu.Draw(out, cw.fonts, cw.scale)` (match the bitmap/fonts variables used by the existing context menu draw).
- In `intercept` pointer section (line ~620-645), handle the main menu FIRST alongside the context menu: if `cw.mainMenu.Visible`, on press hit-test `HitItem`; if hit, `Hide()` and route `cw.onContextMenuAction(item.Action, item.Data)`; if press outside, `Hide()` and consume; on move, `UpdateHover`. Mirror the existing context-menu block exactly.
- Wire the toolbar callback: in `newChromeWindow` set `toolbar.OnMenu = cw.openMainMenu` (or in main.go wiring — match where `OnDownloads` is wired).

- [ ] **Step 5: Route the new actions**

In `cmd/goosie/interaction.go` `handleContextMenuAction` (line 833-907), add cases (match the existing `"inspect"` case style):

```go
case "bookmarks":
	f.navigateTab("goosie://bookmarks")
case "history":
	f.navigateTab("goosie://history")
case "downloads":
	f.navigateTab("goosie://downloads")
case "settings":
	f.navigateTab("goosie://settings")
```

Confirm `"back"`, `"forward"`, `"reload"` already exist (BuildContextMenuItems uses them); if not, add `f.traverseTab(-1)` / `f.traverseTab(1)` / `f.reloadTab()`.

- [ ] **Step 6: Wire OnMenu in main.go**

In `cmd/goosie/main.go` toolbar wiring (line 414-458), if `OnMenu` was not set in `newChromeWindow`, set `tb.OnMenu = func() { cw.openMainMenu() }` following the `OnDownloads` pattern. Ensure exactly one wiring site.

- [ ] **Step 7: Update accessibilityChrome**

In `cmd/goosie/toolbar_window.go` `accessibilityChrome` (line 222-340), replace the fixed per-button `appendTool(...ButtonRect...)` calls with iteration over `toolbar.ComputeLayout(width).Buttons`, adding a menu node (`goosie.menu`, "Menu") and keeping Back/Forward/Reload/Downloads nodes only when visible. Read the current function and preserve the address-bar node and roles.

- [ ] **Step 8: Run the journey test to verify it passes**

Run: `go test ./cmd/goosie/ -run TestJourneyMainMenuOpensManagers -count=1 > /tmp/sp3-t2-green.log 2>&1; echo exit=$?`
Expected: PASS.

- [ ] **Step 9: Run toolbar + cmd/goosie packages**

Run: `go test ./internal/toolbar/ ./cmd/goosie/ -count=1 > /tmp/sp3-t2-pkg.log 2>&1; echo exit=$?`
Expected: PASS. Fix any AX/toolbar test that asserted a fixed button set (now width-dependent) by computing the expected set from `ComputeLayout`.

- [ ] **Step 10: Commit**

```bash
git add internal/toolbar/toolbar.go cmd/goosie/toolbar_window.go cmd/goosie/main.go cmd/goosie/interaction.go cmd/goosie/journey_main_menu_test.go
git commit -m "feat(chrome): add a main menu reaching bookmarks and history (BR-052)"
```

---

## Task 3: Resize journey + collapse verification (BR-143 completion)

**Files:**
- Test: `cmd/goosie/journey_resize_test.go` (new)
- Modify (only if a surface still uses a fixed button list): `cmd/goosie/toolbar_window.go`

- [ ] **Step 1: Write the failing resize journey test**

Create `cmd/goosie/journey_resize_test.go`. Read `e2e_test.go` for `inject`/`waitFor`/`devSize`/`AccessibilityTree`/`findAXNode`. The test shrinks and grows the window and asserts controls stay reachable and non-overlapping:

```go
package main

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

// TestJourneyResizeKeepsControlsReachable resizes the window small and large
// and asserts the toolbar geometry contract holds at each size and the address
// bar still navigates (BR-143).
func TestJourneyResizeKeepsControlsReachable(t *testing.T) {
	b := newBrowserE2E(t)
	b.navigateViaAddressBar(t, "https://example.com/")
	b.waitFor(t, "loaded", func() bool { return b.activeURL() == "https://example.com/" })

	for _, w := range []int32{120, 200, 320, 1280} {
		b.inject(t, surface.Event{Kind: surface.EvResize, Size: frame.Size{W: w, H: 600}, Scale: 1})
		b.waitFor(t, "resize presented", func() bool { return b.window.Stats().Presents > 0 })

		l := toolbar.ComputeLayout(w)
		if l.Address.X1 <= l.Address.X0 || l.Address.W() < toolbar.MinAddressWidth {
			t.Fatalf("width %d: address invalid %+v", w, l.Address)
		}
		// Menu always reachable; no overlap among visible controls.
		if _, ok := l.Rects[toolbar.ButtonMenu]; !ok {
			t.Fatalf("width %d: menu missing", w)
		}
		// Address bar still navigates at this width.
		b.navigateViaAddressBar(t, "https://example.org/")
		b.waitFor(t, "navigated at width", func() bool { return b.activeURL() == "https://example.org/" })
	}
}
```

Adjust `Scale`/`Size` semantics to the harness (read `TestInterceptResizeReportsLogicalContentSize`, main_test.go:653, for device-vs-logical width). If the harness uses dpr 1 and logical width, `Size.W` is the device width = logical width. Confirm `navigateViaAddressBar` works after a resize (it reads `devSize()`).

- [ ] **Step 2: Run it to verify it fails or passes**

Run: `go test ./cmd/goosie/ -run TestJourneyResizeKeepsControlsReachable -count=1 > /tmp/sp3-t3.log 2>&1; echo exit=$?`
If Task 1+2 are correct this may already pass at the geometry level; the journey confirms the live resize path. If it fails because a chrome surface still paints/hit-tests a fixed button list (overlap visible in AX), fix that surface to iterate `ComputeLayout`.

- [ ] **Step 3: Add an AX non-overlap assertion**

Extend the test to read `b.window.AccessibilityTree()` after each resize and assert the chrome button nodes (`goosie.back`/`goosie.menu`/address) do not overlap and stay within the window width. Use `findAXNode` and the node rects. This proves "controls còn truy cập được, không chồng lấn" through the accessibility surface, not just geometry.

- [ ] **Step 4: Run green + race**

Run: `go test ./cmd/goosie/ -run TestJourneyResize -count=1 > /tmp/sp3-t3-green.log 2>&1; echo exit=$?`
Run: `go test -race ./cmd/goosie/ -run TestJourneyResize -count=1 > /tmp/sp3-t3-race.log 2>&1; echo exit=$?`
Expected: PASS both.

- [ ] **Step 5: Commit**

```bash
git add cmd/goosie/journey_resize_test.go cmd/goosie/toolbar_window.go
git commit -m "test(chrome): verify resize keeps toolbar controls reachable (BR-143)"
```

---

## Task 4: Inspector highlight overlay — engine + paint (BR-142 foundation)

**Files:**
- Create: `internal/paint/inspect.go`
- Modify: `internal/paint/builder.go` (field + setter + post-walk call)
- Create: `internal/engine/inspect.go`
- Modify: `internal/engine/paint.go` (wire setter)
- Test: `test/paint/inspect_highlight_test.go` (new)
- Test: `test/engine/inspect_highlight_test.go` (new)

- [ ] **Step 1: Write the failing paint test**

Create `test/paint/inspect_highlight_test.go`. Read `test/paint/select_popup_test.go` or `listbox_pixels_test.go` for how a paint test builds an arena/session and inspects the display list. The test asserts: with `SetInspectHighlight(obj)` the list contains a fill overlay covering the node's border rect; without it, no overlay; the overlay is emitted after the document walk (unclipped). Use the existing paint-test helpers to build a session with a positioned `<div id="target">` and locate its `*layout.Object`.

```go
package paint_test

// Build a session with <div id="target" style="position:absolute;left:10px;top:20px;width:100px;height:40px">.
// Resolve obj via the engine's objectFor equivalent exposed for tests, or via
// arena.ForNode(target.ID). Assert:
//   - baseline PaintChecked list has no inspect overlay command;
//   - after SetInspectHighlight(target), the list has a CmdFill whose Rect
//     covers the target border rect (10,20,110,60) at scale;
//   - ClearInspectHighlight removes it.
```

Write the concrete assertions using the real helper signatures read from the existing paint tests (do not leave this as prose — fill in the actual Go test body matching the corpus style, with a `// qc:BR-142` tag).

- [ ] **Step 2: Write the failing engine test (DOM immutability)**

Create `test/engine/inspect_highlight_test.go` with `// qc:BR-142`:

```go
package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

// TestInspectHighlightDoesNotMutateDOM selects a node and asserts the DOM is
// byte-identical afterward: node count, max depth, and the subtree outerHTML.
func TestInspectHighlightDoesNotMutateDOM(t *testing.T) {
	html := `<html><body><div id="t"><p>hello</p></div></body></html>`
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	target := s.Doc.ElementByID("t")
	beforeCount, beforeDepth := s.DOMStats()
	beforeHTML := target.OuterHTML()

	s.SetInspectHighlight(target)
	if s.InspectHighlight() != target {
		t.Fatal("InspectHighlight should return the selected node")
	}
	afterCount, afterDepth := s.DOMStats()
	if afterCount != beforeCount || afterDepth != beforeDepth {
		t.Fatalf("DOM stats changed: %d/%d -> %d/%d", beforeCount, beforeDepth, afterCount, afterDepth)
	}
	if target.OuterHTML() != beforeHTML {
		t.Fatalf("subtree outerHTML changed:\nbefore %q\nafter  %q", beforeHTML, target.OuterHTML())
	}
	// Paint must succeed and carry the overlay; clearing removes the selection.
	if _, err := s.PaintChecked(1); err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	s.ClearInspectHighlight()
	if s.InspectHighlight() != nil {
		t.Fatal("ClearInspectHighlight should clear the selection")
	}
}

// TestInspectHighlightDetachedNodeIsSkipped selects a node with no layout box
// and asserts paint still succeeds (overlay skipped, no panic).
func TestInspectHighlightDetachedNodeIsSkipped(t *testing.T) {
	s, err := engine.NewSession(`<html><body><div id="t">x</div></body></html>`, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	detached := s.Doc.NewElement("span")
	s.SetInspectHighlight(detached)
	if _, err := s.PaintChecked(1); err != nil {
		t.Fatalf("PaintChecked with detached highlight: %v", err)
	}
}
```

Confirm `target.OuterHTML()` exists on `*dom.Node` (grep `func (n *Node) OuterHTML`); if the method name differs, use the actual serializer. Confirm `s.Doc.NewElement` exists (node.go:1705 — yes).

- [ ] **Step 3: Run both to verify they fail**

Run: `go test ./test/engine/ ./test/paint/ -run InspectHighlight -count=1 > /tmp/sp3-t4-red.log 2>&1; echo exit=$?`
Expected: FAIL — `SetInspectHighlight`/`ClearInspectHighlight`/`InspectHighlight` undefined.

- [ ] **Step 4: Implement the paint overlay**

Create `internal/paint/inspect.go`:

```go
package paint

import (
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
)

// InspectHighlight marks a DOM node the Web Inspector has selected. Obj is the
// node's layout box in document CSS pixels. It paints after the document walk
// so an ancestor's overflow never clips it, and it never mutates the DOM.
type InspectHighlight struct {
	Obj *layout.Object
}

// SetInspectHighlight marks the node to highlight on the next paint.
func (b *Builder) SetInspectHighlight(h *InspectHighlight) {
	b.inspect = h
}

// paintInspectHighlight draws a translucent blue fill plus a border around the
// selected node's border box. Appended after the whole document so it lands on
// top and is never clipped.
func (b *Builder) paintInspectHighlight() {
	h := b.inspect
	if h == nil || h.Obj == nil || b.scale <= 0 {
		return
	}
	x0, y0, x1, y1 := h.Obj.BorderRect()
	if x1 <= x0 || y1 <= y0 {
		return
	}
	r := frame.RectF4(x0, y0, x1, y1).ToDevice(b.scale)
	b.list.Append(DisplayCmd{
		Kind:    CmdFill,
		Rect:    r,
		Color:   frame.RGB(26, 115, 232),
		Opacity: 0.25,
	})
	b.list.Append(DisplayCmd{
		Kind:   CmdBorder,
		Rect:   r,
		Color:  frame.RGB(26, 115, 232),
		Border: uniformBorder(1 * b.scale),
	})
}
```

Confirm `frame.RectF.ToDevice`, `uniformBorder` (or the border helper used by `paintSelectListBox`/`CmdBorder` elsewhere) exist; read `internal/paint/listbox.go` and `cmd.go` for the exact border-field constructor and match it. If `uniformBorder` does not exist, use the same border construction the listbox panel uses.

- [ ] **Step 5: Wire the Builder**

In `internal/paint/builder.go`:
- Add field `inspect *InspectHighlight` (near `validation`, line 22).
- In `Build` (line 94-98), add `b.paintInspectHighlight()` after `b.paintSelectListBox()`.

- [ ] **Step 6: Implement the engine session API**

Create `internal/engine/inspect.go`:

```go
package engine

import (
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/layout"
)

// SetInspectHighlight selects a DOM node for the Web Inspector to highlight.
// The selection is paint-only: it stores the node's layout box and never writes
// an attribute, style, or tree node. A node with no box is stored but skipped at
// paint time.
func (s *Session) SetInspectHighlight(n *dom.Node) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.inspectNode = n
	if n == nil {
		s.inspectObj = nil
		return
	}
	s.inspectObj = s.objectFor(n)
}

// ClearInspectHighlight removes the inspector selection.
func (s *Session) ClearInspectHighlight() {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.inspectNode = nil
	s.inspectObj = nil
}

// InspectHighlight returns the currently selected node, or nil.
func (s *Session) InspectHighlight() *dom.Node {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.inspectNode
}
```

Add fields `inspectNode *dom.Node` and `inspectObj *layout.Object` to the `Session` struct (find it in `internal/engine/engine.go`; place near `validationAnchor`/`selectPopup`). Import `layout` in inspect.go is used via the field type — ensure the field declaration's package matches.

NOTE: `objectFor` resolves the box at selection time. If layout later changes (reflow), the stored `*layout.Object` may be stale. For BR-142 the selection is applied immediately before a repaint, so this is acceptable; to be robust, re-resolve in `Paint` from `inspectNode` instead of caching `inspectObj`. Prefer re-resolving: store only `inspectNode`, and in `Paint` call `s.objectFor(s.inspectNode)`. Drop `inspectObj` if you choose this — keep one owner. Decide and keep the test green.

- [ ] **Step 7: Wire Session.Paint**

In `internal/engine/paint.go`, before `b.Build(layout.ObjectID(1))` (line 96), add:

```go
if s.inspectNode != nil {
	if obj := s.objectFor(s.inspectNode); obj != nil {
		b.SetInspectHighlight(&paint.InspectHighlight{Obj: obj})
	}
}
```

(If you kept `inspectObj`, use it instead; re-resolving is preferred.)

- [ ] **Step 8: Run the tests to verify they pass**

Run: `go test ./test/engine/ ./test/paint/ -run InspectHighlight -count=1 > /tmp/sp3-t4-green.log 2>&1; echo exit=$?`
Expected: PASS.

- [ ] **Step 9: Mutant check**

Temporarily make `paintInspectHighlight` return early (no overlay); run the paint test → FAIL. Restore. Temporarily make `SetInspectHighlight` write an attribute (e.g. `n.SetAttribute("data-inspect","1")`) → the DOM-immutability test FAILS. Restore. Re-run green.

- [ ] **Step 10: Commit**

```bash
git add internal/paint/inspect.go internal/paint/builder.go internal/engine/inspect.go internal/engine/paint.go internal/engine/engine.go test/paint/inspect_highlight_test.go test/engine/inspect_highlight_test.go
git commit -m "feat(engine): paint an inspector node highlight without touching the DOM (BR-142)"
```

---

## Task 5: Inspector selectable tree + cross-tab highlight (BR-142 completion)

**Files:**
- Modify: `cmd/goosie/internal_pages.go` (renderDOMTreeHTML, buildInspectPageHTML)
- Modify: `cmd/goosie/navigation.go` or `presentation.go` (deferred background repaint)
- Test: `cmd/goosie/journey_inspector_test.go` (new)

- [ ] **Step 1: Write the failing end-to-end journey test**

Create `cmd/goosie/journey_inspector_test.go`. Read `cmd/goosie/internal_pages_test.go` for how an inspector page is built against a live tab, and `e2e_test.go` for the harness. The test:
1. Open a page tab with a known element (`<div id="target">`).
2. Open `goosie://inspect?tab=<id>` in a second tab (or via `f.navigateTab`).
3. Capture the target session's DOM stats + `#target` outerHTML.
4. Navigate the inspector tab to `goosie://inspect?tab=<id>&select=<targetNodeID>`.
5. Assert: target session `InspectHighlight()` is the target node; DOM stats + outerHTML unchanged; the inspector HTML marks the selected row; the target tab's layer display list carries the overlay after the deferred repaint.
6. Navigate to `...&select=clear`; assert `InspectHighlight()` is nil.

Use the real harness; if a full two-tab headless journey is heavy, build the inspector HTML directly via `f.buildInspectPageHTML` against a constructed tab (as `internal_pages_test.go` does) and assert the target session state + DOM immutability, then a separate headless journey for the repaint. Fill in concrete Go, `// qc:BR-142`.

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./cmd/goosie/ -run TestJourneyInspector -count=1 > /tmp/sp3-t5-red.log 2>&1; echo exit=$?`
Expected: FAIL — `select` action unhandled / tree rows not links.

- [ ] **Step 3: Make tree rows selectable**

In `cmd/goosie/internal_pages.go` `renderDOMTreeHTML` (line 1812), add a `selected dom.NodeID` and `tabID uint64` parameter (thread them from `buildInspectPageHTML`). Wrap each element's opening tag in a link:

```go
href := fmt.Sprintf("goosie://inspect?tab=%d&select=%d", tabID, n.ID)
style := "color: #e06c75; font-weight: bold;"
if n.ID == selected {
	style += " background-color: #1a73e8; color: #ffffff;"
}
b.WriteString(fmt.Sprintf(`<a href="%s" style="text-decoration:none;"><span style="%s">%s</span></a>`, href, style, html.EscapeString(n.Data)))
```

Keep the rest of the serialization (attributes, children, closing tag) intact; only the tag name becomes a link. Update the recursive calls to pass `selected`/`tabID`. Update the call site in `buildInspectPageHTML` (line ~1098) to pass the parsed `select` id and `targetTab.ID()`.

- [ ] **Step 4: Handle the select action + deferred repaint**

In `buildInspectPageHTML` (line ~938), parse `sel := u.Query().Get("select")`. After `targetTab`/`sess` are resolved:

```go
var selectedID dom.NodeID
if sel != "" && sel != "clear" {
	if id, err := strconv.ParseUint(sel, 10, 32); err == nil {
		selectedID = dom.NodeID(id)
		var found *dom.Node
		sess.Doc.Walk(func(n *dom.Node) bool {
			if n.ID == selectedID {
				found = n
				return false
			}
			return true
		})
		if found != nil {
			sess.SetInspectHighlight(found)
			f.scheduleInspectRepaint(targetTab)
		}
	}
} else if sel == "clear" {
	sess.ClearInspectHighlight()
	f.scheduleInspectRepaint(targetTab)
}
```

Add a "Clear highlight" link near the DOM tree card: `goosie://inspect?tab=<id>&select=clear`.

Implement `scheduleInspectRepaint` so it does NOT call `repaintTab` synchronously inside the page builder (spec D3 constraint: `publishAccessibility` would read the half-built inspector session). Read how the chrome pump drains deferred work (e.g. an existing pending-repaint queue or `f.repaintTab` call sites on the pump thread). Add a small queue on `framePath` (e.g. `inspectRepaintPending []*tabs.Tab` guarded by a mutex) drained on the chrome pump after navigation commits, calling `f.repaintTab(tab, frame.Rect{})` for each. If no pump hook exists, the simplest safe path: set the highlight here, and trigger the target repaint from `applyNavResult` after the inspector tab commits (check `f.inspectRepaintPending`). Choose the mechanism that matches existing threading; add a race test.

- [ ] **Step 5: Run the journey test to verify it passes**

Run: `go test ./cmd/goosie/ -run TestJourneyInspector -count=1 > /tmp/sp3-t5-green.log 2>&1; echo exit=$?`
Expected: PASS.

- [ ] **Step 6: Race + package**

Run: `go test -race ./cmd/goosie/ -run TestJourneyInspector -count=1 > /tmp/sp3-t5-race.log 2>&1; echo exit=$?`
Run: `go test ./cmd/goosie/ -count=1 > /tmp/sp3-t5-pkg.log 2>&1; echo exit=$?`
Expected: PASS. Fix any `internal_pages_test.go` assertion that expected the old static tree (now links).

- [ ] **Step 7: Commit**

```bash
git add cmd/goosie/internal_pages.go cmd/goosie/navigation.go cmd/goosie/presentation.go cmd/goosie/journey_inspector_test.go
git commit -m "feat(inspector): select a DOM node to highlight it without mutation (BR-142)"
```

---

## Task 6: Full verification + catalog/roadmap sign-off

**Files:**
- Modify: `docs/qa-test-catalog.md` (BR-052, BR-142, BR-143 rows + header counts)
- Modify: `docs/roadmap-v2.md` (P2 status: no remaining NOT RUN rows)

- [ ] **Step 1: Full build + format + vet**

Run:
```bash
go build ./... > /tmp/sp3-build.log 2>&1; echo build=$?
gofmt -l internal/toolbar internal/paint internal/engine cmd/goosie > /tmp/sp3-fmt.log 2>&1; cat /tmp/sp3-fmt.log
go vet -all ./... > /tmp/sp3-vet.log 2>&1; echo vet=$?
```
Expected: build=0, empty fmt list, vet=0. Fix any gofmt with `gofmt -w`.

- [ ] **Step 2: Archtest + gate**

Run:
```bash
go test ./internal/archtest/ -count=1 > /tmp/sp3-arch.log 2>&1; echo arch=$?
go test ./test/gate -run TestGate -count=1 > /tmp/sp3-gate.log 2>&1; echo gate=$?
```
Expected: arch=0, gate=0. If archtest flags a new import (e.g. `internal/paint` already allowed), verify against `internal/archtest/rules.go`; no new package is added so the table should be unchanged.

- [ ] **Step 3: Full suite + race**

Run:
```bash
go test ./... -count=1 > /tmp/sp3-all.log 2>&1; echo all=$?
go test -race ./internal/toolbar/ ./internal/paint/ ./internal/engine/ ./cmd/goosie/ -count=1 > /tmp/sp3-race.log 2>&1; echo race=$?
```
Expected: all=0, race=0. Read logs (never `tail`). Investigate any failure root-cause; do not skip.

- [ ] **Step 4: Parity unchanged**

Run: `make parity > /tmp/sp3-parity.log 2>&1; echo parity=$?`
Expected: 105/105, average ~96.39%, known gaps 4 fixtures/6 bands unchanged. Chrome UI changes do not touch the content corpus; the overlay is opt-in. If Playwright/Chromium is unavailable in this environment, record that parity was not re-run and why (do not claim a pass).

- [ ] **Step 5: Update the catalog rows**

In `docs/qa-test-catalog.md`:
- BR-052 (line 74): NOT RUN → PASS (INT). Document the main menu journey test, the `ComputeLayout`/`ContextMenu` reuse, and "native CUA chưa chạy do phiên không có display".
- BR-142 (line 311): NOT RUN → PASS (INT mechanics). Document the overlay + DOM-immutability oracle and CUA pending.
- BR-143 (line 312): NOT RUN → PASS (INT). Document the geometry-invariant sweep + resize journey and CUA pending.
- Update the header count line (line 7): NOT RUN drops from 5 to 2 (BR-085, BR-180 remain); PASS rises by 3. Keep BR-085/BR-180 unchanged.

- [ ] **Step 6: Update the roadmap P2 status**

In `docs/roadmap-v2.md` P2 status (line ~193), record that BR-052/BR-142/BR-143 are signed PASS (INT) with CUA display-blocked, so no P2 NOT RUN rows remain; the P2 gate's automated half is satisfied. Do not claim the native half. Match the existing Vietnamese prose style and the "đã có trong mã / đã kiểm chứng" distinction.

- [ ] **Step 7: Commit docs**

```bash
git add docs/qa-test-catalog.md docs/roadmap-v2.md
git commit -m "docs(qa): sign off BR-052, BR-142, BR-143 P2 journeys (INT, CUA pending)"
```

- [ ] **Step 8: Final history check**

Run: `git --no-pager log --oneline 77b3a26f..HEAD && git status --short`
Expected: the SP-3 task commits, clean working tree. No push.

---

## Self-review notes

- **Spec coverage:** BR-052 → Tasks 2,6; BR-143 → Tasks 1,3,6; BR-142 → Tasks 4,5,6. D1 (menu reuse) Task 2; D2 (geometry owner) Tasks 1,3; D3 (post-walk overlay, no DOM write) Tasks 4,5; D4 (CUA recorded) Task 6. Out-of-scope items untouched.
- **Type consistency:** `ComputeLayout`/`Layout`/`MinAddressWidth`/`ButtonMenu` defined in Task 1 and used in Tasks 2,3. `SetInspectHighlight`/`ClearInspectHighlight`/`InspectHighlight` defined in Task 4 and used in Task 5. `InspectHighlight{Obj}` paint type vs `Session.InspectHighlight()` method share a name across packages (paint vs engine) — acceptable, distinct packages; keep signatures exactly as written.
- **Known read-before-edit points:** `handlePointerLocked` pending queue, `surface.Event` pointer shape, `windowExtent`/`scale` fields, `OuterHTML` method name, border-field constructor, deferred-repaint pump hook. Each step says to read the real source first; the executor must match actual signatures, not the illustrative snippets.