# Goosie Browser Architecture

## Overview

Goosie's browser architecture uses a modern multi-tree rendering system that separates concerns between DOM parsing, styling, layout computation, and painting. This design enables maintainable, testable, and performant rendering with support for incremental updates.

## Navigation ID Flow

Every page load receives a monotonic navigation ID from `internal/engine/navigation`. The browser shell uses a `Scheduler` to:

1. Cancel the previous load context when a new navigation starts
2. Assign a unique `navigation.ID` to the new load
3. Attach the ID to the load's `context.Context` for downstream phases
4. Reject stale load callbacks when a superseded navigation completes

### Resource Priorities

Each navigation and sub-resource load carries a `Priority` value indicating its
loading urgency. Priorities are ordered from highest (`PriorityDocument`) to
lowest (`PrioritySpeculative`):

- `PriorityDocument` — main document navigation (default for `Begin`)
- `PriorityBlockingCSS` — render-blocking stylesheet
- `PriorityVisibleImage` — image in or near the viewport
- `PriorityScript` — synchronous or async script
- `PriorityDeferredImage` — below-fold or lazy image
- `PrioritySpeculative` — prefetch, prerender, dns-prefetch

The `Scheduler` tracks all pending loads (main navigation + sub-resources) and
returns a priority-sorted snapshot via `PendingLoads()`. Sub-resources are
registered with `AddResource()` and removed with `RemoveResource()`. Each
sub-resource receives its own cancellable context derived from its parent,
and all are cleaned up when the main navigation is cancelled or superseded.

Priority is propagated through `context.Context` and can be retrieved with
`PriorityFromContext()` by downstream network layers for admission control.

### Concurrency Bounding

The `navigation.Scheduler` supports application-level concurrency bounding
via `SchedulerOptions`. A `RateLimiter` enforces per-origin (default 6) and
global (default 24) concurrent request limits at the application level,
complementing the transport-level `MaxConnsPerHost`. When slots are contended,
the limiter uses a `container/heap`-based priority queue so higher-priority
resources (document, blocking CSS) are admitted before lower-priority ones
(speculative, deferred images). A zero-value `SchedulerOptions` means
unlimited, preserving backward compatibility with existing code that uses
`NewScheduler()`.

## Shared HTTP Transport

The `internal/engine/session.Session` owns one configured `http.Transport` that is reused
across all HTTP requests within a browsing context. The transport is created with sensible
connection limits (MaxIdleConns=100, MaxConnsPerHost=6), timeouts (dial 30s, TLS 10s),
and HTTP/2 support. Callers obtain a client sharing the transport via `Session.HTTPClient()`
(which provides a fresh `http.Client` with a cookie jar) or access the raw transport via
`Session.Transport()`.

The transport lifecycle matches the session lifecycle — `Session.Close()` calls
`CloseIdleConnections()` to release pooled connections. In-flight requests complete
normally even after Close. This prevents connection leaks across repeated navigations
and gives the engine explicit control over network resource limits.

### Response Metadata

Every HTTP response captured by `Service.FetchWithMeta` produces an immutable
`ResponseMeta` struct containing status code, headers, content type, content length,
content encoding, protocol version, charset, and cache-hit status. The `Fetcher`
exposes the most recent response metadata via `Fetcher.Meta()`, which is safe for
concurrent reads. Cache hits synthesize metadata from the stored `CacheEntry`.
This preserves response information for security inspection and developer tools
without retaining the live `http.Response` after the body is consumed.

### Streaming Response Bodies (M1.3)

The main document path uses `Service.FetchStream` and `Fetcher.FetchStreamWithContext`
to return the response body as an `io.ReadCloser` without buffering the entire body
into an intermediate `bytes.Buffer`. This eliminates one full-body copy from the
fetch-to-parse pipeline:

```text
User enters URL
  -> Scheduler.Begin(url)
       -> navigation ID assigned
       -> previous context cancelled
  -> FetchStream (ctx carries navigation ID)
       -> returns io.ReadCloser + ResponseMeta
  -> dom.ParseDocument(stream) feeds tokenizer directly from response
  -> UI update only if Scheduler.IsActive(id)
```

The streaming path:
- Returns the body as `io.ReadCloser` — caller must close when done
- Wraps the body with `limitedContextReader` for context cancellation and size limits
- Does not populate the HTTP cache (caching requires the full body)
- Preserves `ResponseMeta` for security and developer tools
- Benchmarks show 26% less memory and 15% fewer allocations vs the buffered path

The DOM parser's `ParseDocument(io.Reader)` method accepts any `io.Reader`, enabling
the HTML tokenizer to consume the response stream directly without an intermediate
string copy.

```
User enters URL
  -> Scheduler.Begin(url)
       -> navigation ID assigned
       -> previous context cancelled
  -> Fetch (ctx carries navigation ID)
  -> Parse / Render
  -> UI update only if Scheduler.IsActive(id)
```

This keeps navigation tracing UI-independent and prepares phase-level metrics in Milestone 0.3.

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

### Atom and String Interning (M2.2)

The `internal/dom/atom` package provides compact uint32 handles (`Atom`) for
interned strings, reducing allocation pressure and pointer density in the
engine's hot paths.

**Static atoms** are pre-assigned constants for all common HTML tag names
(112 tags: `div`, `span`, `p`, `a`, etc.) and attribute names (48 attrs:
`id`, `class`, `href`, `src`, etc.). Static atom lookup is O(1) with zero
allocations.

**Dynamic atoms** are interned into a bounded LRU-evicted `Table` with
configurable entry count and byte limits. Strings exceeding the byte limit
are rejected to prevent unbounded memory growth. The default table supports
1024 entries and 64 KB of string data.

The atom table is safe for concurrent use and is designed as foundation
infrastructure for the compact DOM store (M2.3) and CSS pipeline (M3.1).

### Compact DOM Store (M2.3)

The `internal/dom` package provides a compact DOM store (`Store`) that
replaces pointer-heavy `*html.Node` trees with index-based storage using
stable `NodeID` handles.

**Data model:**

- `NodeID` (uint32) is an index into a contiguous `[]nodeRecord` slice
- Stale handle detection via `Kind` field (Kind == 0 means freed)
- Each node record is 32 bytes with first-child/next-sibling links
- Attributes stored in a packed `[]Attr` slice (8 bytes per attr)
- Text content stored in a separate `[]byte` buffer
- Rare metadata (namespace, etc.) in a dedicated map

**Traversal:**

Zero-allocation iterators for children, subtree (pre-order DFS),
reverse children, siblings, and ancestors.

**Operations:**

- `Allocate`, `Remove`, `Replace` for node lifecycle
- `AppendChild`, `PrependChild`, `InsertBefore`, `RemoveChild` for tree mutation
- `SetAttrs`, `SetText` for content with automatic offset updates
- `SetFlag`, `ClearFlag`, `SetRareData` for metadata

**Performance (VirtualApple @ 2.50GHz):**

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|----------|
| AppendChild | 542 | 215 | 0 |
| SetAttrs | 22 | 0 | 0 |
| SetText | 22 | 0 | 0 |
| ChildIterator (100 children) | 436 | 0 | 0 |
| SubtreeIterator (111 nodes) | 746 | 0 | 0 |

The store is additive infrastructure — existing `*html.Node`-based
consumers (parser, renderer, JS runtime) are unaffected until M2.4
(streaming tree construction) and M2.5 (compatibility adapter).

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
