# Goosie Browser Architecture

## Overview

Goosie's browser architecture uses a modern multi-tree rendering system that separates concerns between DOM parsing, styling, layout computation, and painting. This design enables maintainable, testable, and performant rendering with support for incremental updates.

## Component Flow

```
┌─────────────────────────────────────────────────────────────┐
│                        Main Browser                          │
│                    (cmd/browser/main.go)                     │
└───────────────────────────┬─────────────────────────────────┘
                            │
        ┌───────────────────┼───────────────────┐
        │                   │                   │
        ▼                   ▼                   ▼
┌──────────────┐   ┌──────────────┐   ┌──────────────┐
│   HTTP       │   │   HTML       │   │ JavaScript   │
│   Fetcher    │──▶│   Parser     │──▶│   Runtime    │
│ (internal/   │   │ (internal/   │   │ (internal/   │
│    net)      │   │    dom)      │   │    js)       │
└──────────────┘   └──────┬───────┘   └──────────────┘
                          │
                          ▼
                   ┌──────────────┐
                   │   HTML       │
                   │  Renderer    │
                   │ (internal/   │
                   │  renderer)   │
                   └──────┬───────┘
                          │
                          ▼
                   ┌──────────────┐
                   │  GUI Browser │
                   │ (internal/   │
                   │    ui)       │
                   │ [Fyne Window]│
                   └──────┬───────┘
                          │
                          ▼
                   ┌──────────────┐
                   │ Browser State│
                   │  - History   │
                   │  - Bookmarks │
                   └──────────────┘
```

## Navigation State Flow

```
User enters URL → Add to History → Fetch Page → Parse HTML → Render
       │               │
       │               ├─→ Update URL bar
       │               ├─→ Update back/forward buttons
       │               └─→ Update bookmark indicator
       │
       ├─→ Back button → GoBack() → Fetch previous URL
       ├─→ Forward button → GoForward() → Fetch next URL
       ├─→ Refresh button → Reload current URL
       └─→ Bookmark button → Toggle bookmark state
```

## Example Execution Flow

### Initial Startup
1. **GUI Browser** (`internal/ui/browser.go`)
   - Creates Fyne window titled "Goosie"
   - Initializes navigation controls (URL bar, buttons)
   - Creates BrowserState for history/bookmarks
   - Displays welcome message
   - Waits for user input

### Navigation Flow (User enters URL)
1. **User Action**
   - User enters "https://example.com" in URL bar
   - Presses Enter or clicks navigation button

2. **State Management** (`internal/ui/state.go`)
   - Adds URL to navigation history
   - Updates current index
   - Enables/disables back/forward buttons appropriately

3. **HTTP Fetcher** (`internal/net/fetcher.go`)
   - Fetches https://example.com
   - Returns HTML content

4. **HTML Parser** (`internal/dom/parser.go`)
   - Parses HTML using x/net/html
   - Extracts HTML structure for rendering
   - Provides getElementById functionality for JS

5. **HTML Renderer** (`internal/renderer/`)
   - Multi-tree architecture: DOM → Render Tree → Layout Tree → Display List
   - Builds render tree from parsed HTML with unique node IDs
   - Computes layout tree with box model calculations
   - Generates display list for efficient painting
   - Supports incremental updates with invalidation tracking
   - Hit testing for interactive elements
   - Renders to Fyne canvas objects
   - Supports headings, paragraphs, lists, links, images

6. **JavaScript Runtime** (`internal/js/runtime.go`)
   - Sets HTML content for DOM operations
   - Runs: `console.log("Page loaded: " + document.title)`
   - Output: "Page loaded: Example Domain"

7. **GUI Browser** (`internal/ui/browser.go`)
   - Updates URL bar with current URL
   - Updates button states (back/forward/bookmark)
   - Displays rendered content in scrollable canvas
   - Shows bookmark indicator if page is bookmarked

## Window Layout (when GUI is available)

```
┌───────────────────────────────────────────────────────┐
│ Goosie                                       [_][□][X]│
├───────────────────────────────────────────────────────┤
│ ← → ⟳ │ https://example.com                     │ ☆   │
├───────────────────────────────────────────────────────┤
│                                                       │
│  # Example Domain                                     │
│                                                       │
│  This domain is for use in illustrative               │
│  examples in documents. You may use this              │
│  domain in literature without prior                   │
│  coordination or asking for permission.               │
│                                                       │
│  [More information...](https://example.org)           │
│                                                       │
│                                                       │
│                                                       │
└───────────────────────────────────────────────────────┘
```

### Navigation Bar Components
- **← (Back)**: Navigate to previous page in history
- **→ (Forward)**: Navigate to next page in history  
- **⟳ (Refresh)**: Reload current page
- **URL Entry**: Enter web addresses, press Enter to navigate
- **☆/★ (Bookmark)**: Toggle bookmark for current page

## Test Coverage

- internal/net: 36.4%
- internal/dom: 95.0%
- internal/renderer: 100% (65+ tests, including benchmarks)
- internal/js: 92.9%

See [README.md](README.md) for usage and testing instructions.
