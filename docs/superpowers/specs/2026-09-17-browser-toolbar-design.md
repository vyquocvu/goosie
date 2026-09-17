# Browser Toolbar with URL Input — Design Spec

**Date:** 2026-09-17  
**Status:** Approved for implementation

## Summary

Add a custom-drawn browser toolbar (toolbar) with URL input, back/forward navigation, and history. The toolbar is rendered as a fixed overlay on the backing store, above the tile-rendered content area.

## Architecture

The toolbar is drawn directly on the compositor's backing bitmap after content tiles are composited. A new `internal/toolbar/` package owns toolbar state and rendering.

```
┌─────────────────────────────────────────┐
│  Toolbar (40px)                          │
│  [◀] [▶] [⟳] [address bar........] [⏎]│
├─────────────────────────────────────────┤
│                                         │
│  Content area (viewport - 40px)         │
│                                         │
└─────────────────────────────────────────┘
```

## Components

### `internal/toolbar/toolbar.go`

Main state and rendering:

```go
type Focus int
const (
    FocusNone Focus = iota
    FocusAddress
)

type State struct {
    URL        string
    Input      string
    Cursor     int
    Focus      Focus
    History    *History
    Loading    bool
    Bounds     frame.Rect
    OnNavigate func(string)
}

func NewState(width int32) *State
func (s *State) Draw(backing *frame.Bitmap, fonts *raster.Fonts)
func (s *State) HandleClick(pos frame.Point, button surface.Button)
func (s *State) HandleKey(r rune)
func (s *State) Navigate(url string)
func (s *State) SetLoading(loading bool)
```

### `internal/toolbar/history.go`

Back/forward stack:

```go
type History struct {
    entries []string
    index   int
}

func NewHistory() *History
func (h *History) Back() (string, bool)
func (h *History) Forward() (string, bool)
func (h *History) Push(url string)
func (h *History) CanBack() bool
func (h *History) CanForward() bool
func (h *History) Current() string
```

### `internal/toolbar/input.go`

Text input handling:

```go
func (s *State) InsertRune(r rune)
func (s *State) DeleteBackward()
func (s *State) MoveCursorLeft()
func (s *State) MoveCursorRight()
func (s *State) MoveCursorStart()
func (s *State) MoveCursorEnd()
func (s *State) SelectAll()
```

### `internal/toolbar/layout.go`

Geometry constants:

```go
const (
    ToolbarHeight = 40
    ButtonSize    = 32
    ButtonGap     = 4
    BarPadding    = 8
)

type Button int
const (
    ButtonBack Button = iota
    ButtonForward
    ButtonReload
    ButtonGo
)

func ButtonRect(btn Button, toolbarWidth int32) frame.Rect
func AddressBarRect(toolbarWidth int32) frame.Rect
```

## Data Flow

### Navigation

1. User types URL, presses Enter
2. `HandleKey('\r')` → `OnNavigate(url)`
3. Caller fetches HTML, runs engine pipeline, publishes new FramePlan
4. `History.Push(url)`
5. Next frame: toolbar shows new URL, content shows new page

### Back/Forward

1. User clicks back button
2. `HandleClick` detects hit on back button
3. `History.Back()` returns previous URL
4. `OnNavigate(url)` called

### Input Routing

In the UI loop (or wrapper):
- Pointer events with Y < ToolbarHeight → `toolbar.HandleClick`
- Key events when Focus == FocusAddress → `toolbar.HandleKey`
- Other pointer events → existing scroll handling

## Rendering

`Draw()` paints directly on backing bitmap:
1. Fill toolbar background (#F0F0F0)
2. Draw buttons with simple glyphs (arrows, circle)
3. Draw address bar (white rect with #CCC border)
4. Draw input text + cursor (if focused)
5. Draw Go button or loading indicator

Uses `frame.Bitmap.FillRect` for rectangles. Text uses `raster.Fonts` for glyph measurement and rendering.

## Error Handling

- Network errors: caller shows error page in content area
- Invalid URL: keep focus on address bar
- Empty input: ignore Enter

## Testing

- Unit tests for History (push, back, forward, bounds)
- Unit tests for input handling (insert, delete, cursor movement)
- Unit tests for hit testing (button clicks, address bar focus)
- Visual verification via headless PNG output

## Integration Points

- `cmd/goosie/main.go`: wire toolbar into the loop, handle OnNavigate callback
- `internal/surface/loop.go`: optionally extend to route input to toolbar
- `internal/raster/fonts.go`: pass Fonts to toolbar for text rendering

## Out of Scope

- Tabs
- Bookmarks
- Persisted history
- Autocomplete
- Favicons
- SSL indicators
