# Gate 3: Browser Surface Fundamentals Design

- Date: 2026-09-23
- Status: proposed
- Gate: 3 of `docs/roadmap-v2.md`
- Scope: content Y offset, resize reflow, readable tab titles, loading/error indicators
- Deferred: links/anchors, forms, selection, find, zoom (Gate 5+)

## 1. Problem

The browser surface has four gaps that prevent it from being a usable document viewer:

1. **Chrome overwrites content pixels.** `chromeWindow.Present` paints the tab bar and toolbar directly into the document buffer (`cmd/goosie/toolbar_window.go:42-44`). The scheduler's viewport is the full window, so tile rows under the chrome area are document pixels that get overwritten. A scroll fast path (`internal/raster/scheduler.go:439-448`) shifts retained backing pixels, which can drag chrome-contaminated rows into the content area.

2. **No resize reflow.** `loadURLCtx` creates an `engine.Session`, paints once, and discards the session (`cmd/goosie/main.go:602-624`). The `Tab` struct stores only the resulting `*frame.Layer` (`internal/tabs/tab.go:26`). When the window resizes, the toolbar bounds update (`toolbar_window.go:108-110`) but the document layout is never recomputed.

3. **Tab titles are rectangles.** `drawTabText` in `internal/tabs/draw.go:74-88` renders text as a colored rectangle proportional to string length (`len(text) * 5` pixels). Users cannot read tab titles.

4. **Loading/error state is invisible.** `toolbar.State` has `Loading bool` and `Error string` fields, but `Draw` (`internal/toolbar/toolbar.go:311-327`) renders neither. A user navigating to a slow or broken URL sees no feedback.

## 2. Design

### 2.1 Content viewport offset

The scheduler viewport must exclude the chrome area so tile rows begin below the tab bar and toolbar.

**Changes:**

- `framePath` passes a content viewport to the scheduler: height = `windowH - totalChromeHeight`, where `totalChromeHeight = tabs.TabBarHeight + toolbar.ToolbarHeight = 76px`.
- `chromeWindow.Present` changes role: it receives a content-only buffer from the scheduler and composites it into a full-window bitmap with chrome painted above. The composite buffer is what goes to the platform.
- Damage rects from the scheduler are in content coordinates. `Present` translates them by `+totalChromeHeight` in Y before merging with the chrome rect.
- Scroll offset remains in document coordinates; the scheduler is unaware of chrome.

**Key constraint:** The scheduler's `SetViewport` size must be the content area, not the window. The `chromeWindow` gains an `OnResize func(contentW, contentH int)` callback (set by `framePath` at construction, like `onSwitch`). When `EvResize` arrives, `chromeWindow.intercept` calls `toolbar.SetBounds(windowW)`, then calls `OnResize(windowW, windowH - totalChromeHeight)`, then returns `false` to forward the event to the surface loop. The `framePath`'s resize handler updates `config.width` and triggers reflow on the active tab's session.

### 2.2 Resize reflow

Each tab retains its `*engine.Session` so the document can be re-laid-out at a new viewport width without re-fetching or re-parsing.

**Changes:**

- Add `Session *engine.Session` field to `Tab` (`internal/tabs/tab.go`).
- `loadURLCtx` returns the session alongside the layer, scene spec, and background color. Its signature gains a `*engine.Session` return value.
- On `EvResize`, the `framePath` handler:
  1. Updates `config.width` to the new content width.
  2. Calls `session.Reflow(newContentWidth)` on the active tab's session.
  3. Calls `session.PaintChecked(scale)` to get a new display list.
  4. Builds a new layer from the display list and calls `sched.SetPlan` with it.
- Reflow errors are logged; the old layout is retained on failure.
- Sessions are not shared across tabs; each tab has its own.

**Key constraint:** `engine.Session.Reflow` already exists (`internal/engine/engine.go:445-484`) and re-runs the full layout pipeline (block, inline, positioning) with the new viewport width. It retains the parsed DOM, computed styles, font metrics, and decoded images. The paint step after reflow produces a new display list at the new width.

### 2.3 Readable tab titles

Replace the rectangle placeholder in `drawTabText` with actual glyph rendering using the same font infrastructure the toolbar uses.

**Changes:**

- `DrawTabBar` gains a `fonts *raster.Fonts` parameter.
- `drawTabText` uses `fonts.Glyph(textSize, rune, slot)` to render each character, matching the toolbar's `drawText` approach (`internal/toolbar/toolbar.go:531-548`).
- Text that exceeds the available width is truncated with an ellipsis character (`...` or Unicode `U+2026`).
- The caller (`chromeWindow.Present`) passes `f.fonts` through to `DrawTabBar`.

**Key constraint:** The `tabs` package currently imports only `frame` and `toolbar`. Adding `raster.Fonts` as a parameter is acceptable because `tabs` already imports `toolbar` which imports `raster`. The import chain remains valid: `tabs` <- `toolbar` <- `raster`.

### 2.4 Loading and error indicators

Add visible feedback to the toolbar for in-progress navigation and failed loads.

**Loading indicator:**

- A 2px-high progress line at the bottom of the toolbar (above the separator), drawn in `activeHighlight` blue (`frame.RGB(70, 140, 220)`).
- When `Loading` is true, the line spans the full toolbar width. When false, it is not drawn.
- Drawn in `State.Draw` after the address bar.

**Error indicator:**

- When `Error` is non-empty and the address bar is not focused, the error text is drawn inside the address bar in red (`frame.RGB(200, 50, 50)`) instead of the URL.
- The error replaces the URL display; clicking the address bar switches back to URL editing mode.

**Changes:**

- `State.Draw` (`internal/toolbar/toolbar.go:311-327`): after drawing the address bar, if `s.Loading`, draw a 2px progress line.
- `State.drawAddressBar` (`internal/toolbar/toolbar.go:423-470`): if `s.Error != ""` and `s.Focus == FocusNone`, draw error text in red instead of URL.

## 3. Data flow

```
EvResize (platform)
  → chromeWindow.intercept
    → toolbar.SetBounds(windowW)
    → OnResize(windowW, windowH - 76) callback
      → framePath.handleResize(contentW, contentH)
        → config.width = contentW
        → tab.Session.Reflow(contentW)
        → tab.Session.PaintChecked(scale)
        → build new layer, sched.SetPlan
    → return false (forward to surface.Loop)
  → surface.Loop → composer.Resize(windowW, windowH)
  → scheduler.BeginFrame sees new viewport size

Navigation result (goroutine → navResults channel)
  → applyNavResult
    → tab.Session = result.session  (new)
    → tab.Layer = result.layer
    → sched.SetPlan

chromeWindow.Present(contentBuf, damage)
  → paint chrome into chromeBitmap
  → composite chromeBitmap + contentBuf into full-window bitmap
  → translate damage rects by +76 in Y
  → forward to platform Window.Present
```

## 4. Testing

- **Unit:** tab title glyph rendering test with known strings and widths.
- **Unit:** toolbar loading line and error text rendering tests.
- **Integration:** resize event triggers reflow and produces a new layer at the correct width.
- **Integration:** content viewport excludes chrome height; scroll does not contaminate chrome area.
- **Manual:** run `./goosie -url https://example.com`, resize window, verify document re-lays out. Verify tab titles are readable. Verify loading spinner appears during navigation. Navigate to an invalid URL, verify error text appears.

## 5. Deferred

- Links and anchors (Gate 5)
- Forms, selection, find (Gate 5)
- DPR/Retina reflow (the session stores the scale; reflow uses the current scale)
- Tab title from `<title>` element (requires DOM integration; current tab title is the URL)
- URL security display (origin highlighting, certificate status — Gate 4)
