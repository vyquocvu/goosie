# Goosie Browser: Sprint 1 Plan & Work Breakdown Structure (WBS)

| Document Version | 1.0.0 |
| :--- | :--- |
| **Status** | Approved for Execution |
| **Sprint** | Sprint 1: The First Frame Pipeline |
| **Duration** | 4 weeks (20 working days) |
| **Sprint Goal** | Dựng khung hình đầu tiên: Từ HTML input đến pixel output, qua pipeline Parse → Style → Layout → Paint → Raster → Present |
| **References** | PRODUCT_VISION_AND_STRATEGY.md, SOFTWARE_REQUIREMENTS_SPECIFICATION.md, SYSTEM_ARCHITECTURE_BLUEPRINT.md |

---

## 1. Executive Summary

Sprint 1 là sprint nền tảng (foundation sprint) của Goosie Browser, tập trung vào việc xây dựng **pipeline rendering hoàn chỉnh** từ HTML input đến pixel output trên màn hình. Sprint này thiết lập:

- **Architectural invariants** (import rules, cgo confinement, zero globals)
- **Core data structures** (frame geometry, DOM tree, layout arena, display list)
- **Security gates** (SSRF guard, admission limits, input validation)
- **Performance baseline** (0 allocs/op hot paths, < 150ms cold start, 60fps pacing)

**Success Criteria:**
- End-to-end: HTML URL → rendered frame trên screen
- Visual parity: 95%+ pixel match với Chromium trên 10 test pages
- Performance: Cold start < 150ms, idle memory < 30MB, scroll pacing < 16.6ms
- Security: 100% SSRF drop rate, zero admission limit bypasses
- Quality: All architecture tests PASS, zero data races

---

## 2. Sprint Backlog & Work Breakdown Structure

### 2.1 Epic Overview

| Epic ID | Epic Name | Priority | Effort | Dependencies | Status |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **Epic 1** | Architectural Foundation & Boundaries | P0 Must-Have | 2 days | None | ⏳ Not Started |
| **Epic 2** | Frame Package - Geometry & Tile Foundation | P0 Must-Have | 2 days | Epic 1 | ⏳ Not Started |
| **Epic 3** | Network Transport with SSRF Guard | P0 Must-Have | 2 days | Epic 1 | ⏳ Not Started |
| **Epic 4** | DOM Parser - Bounded HTML5 Tree Construction | P0 Must-Have | 2 days | Epic 1 | ⏳ Not Started |
| **Epic 5** | Style Cascade - CSS Parser & Resolver | P0 Must-Have | 3 days | Epic 2, Epic 4 | ⏳ Not Started |
| **Epic 6** | Layout Arena - Box Generation & Formatting Contexts | P0 Must-Have | 3 days | Epic 4, Epic 5 | ⏳ Not Started |
| **Epic 7** | Paint Builder - Display List Emission | P0 Must-Have | 2 days | Epic 5, Epic 6 | ⏳ Not Started |
| **Epic 8** | Tile Rasterizer - CPU Worker Pool | P0 Must-Have | 2 days | Epic 2, Epic 7 | ⏳ Not Started |
| **Epic 9** | Surface Composer - Vsync Presentation Loop | P0 Must-Have | 2 days | Epic 7, Epic 8 | ⏳ Not Started |
| **Epic 10** | Engine Session - Full Pipeline Integration | P0 Must-Have | 3 days | All above | ⏳ Not Started |

**Total Effort:** 23 person-days (4 weeks with 1 engineer, or 2 weeks with 2 engineers)

---

### 2.2 Detailed Task Breakdown

#### Epic 1: Architectural Foundation & Boundaries

**Owner:** [CTO] + [Core Dev]  
**Duration:** 2 days  
**Dependencies:** None

**Tasks:**

| Task ID | Task Name | Description | DoD |
| :--- | :--- | :--- | :--- |
| 1.1 | Define import rules table | Create `internal/archtest/rules.go:40-64` với allowed-imports matrix | Table covers all 10 packages, zero ambiguities |
| 1.2 | Implement TestImportRules | Verify mọi package chỉ import từ allowed list | `go test ./internal/archtest/ -run TestImportRules` PASS |
| 1.3 | Implement TestCgoConfinement | Verify cgo chỉ xuất hiện trong `internal/platform/darwin` | `go test ./internal/archtest/ -run TestCgoConfinement` PASS |
| 1.4 | Implement TestNoMutableGlobals | Verify zero mutable package-level globals trong `internal/` | `go test ./internal/archtest/ -run TestNoMutableGlobals` PASS |
| 1.5 | Document architecture invariants | Update `SYSTEM_ARCHITECTURE_BLUEPRINT.md` §1.2 | Document reviewed và approved bởi [CTO] |

**Deliverables:**
- `internal/archtest/rules.go` - Import rules table
- `internal/archtest/import_test.go` - TestImportRules
- `internal/archtest/cgo_test.go` - TestCgoConfinement
- `internal/archtest/globals_test.go` - TestNoMutableGlobals

**Acceptance Criteria (Gherkin):**

```gherkin
Scenario: Import rules enforcement
  Given the codebase contains internal/archtest/rules.go
  When I run "go test ./internal/archtest/ -run TestImportRules"
  Then the test should PASS
  And every internal package should only import from its allowed list

Scenario: Cgo confinement
  Given the codebase contains cgo code
  When I run "go test ./internal/archtest/ -run TestCgoConfinement"
  Then the test should PASS
  And cgo should only appear in internal/platform/darwin

Scenario: Zero mutable globals
  Given the codebase contains package-level variables
  When I run "go test ./internal/archtest/ -run TestNoMutableGlobals"
  Then the test should PASS
  And there should be zero mutable globals in internal/
```

---

#### Epic 2: Frame Package - Geometry & Tile Foundation

**Owner:** [TA] + [Core Dev]  
**Duration:** 2 days  
**Dependencies:** Epic 1

**Tasks:**

| Task ID | Task Name | Description | DoD |
| :--- | :--- | :--- | :--- |
| 2.1 | Define Point, Rect, Size structs | `internal/frame/geometry.go` với float32 coordinates | Structs are 64-byte cache-line aligned |
| 2.2 | Define Color struct | `internal/frame/color.go` với RGBA8 premultiplied | Color arithmetic operations implemented |
| 2.3 | Define TileCoord, TileState | `internal/frame/tile.go` với TileSize = 256 | Tile coordinate system working |
| 2.4 | Define Bitmap struct | `internal/frame/bitmap.go` với data pointer và stride | Bitmap operations: clear, fill, blit |
| 2.5 | Implement buffer pool | `internal/frame/pool.go` với sync.Pool | 0 allocs/op cho buffer reuse |
| 2.6 | Write unit tests | `internal/frame/*_test.go` | All tests PASS, 100% coverage |

**Deliverables:**
- `internal/frame/geometry.go` - Point, Rect, Size
- `internal/frame/color.go` - Color (RGBA8 premultiplied)
- `internal/frame/tile.go` - TileCoord, TileState, TileSize
- `internal/frame/bitmap.go` - Bitmap
- `internal/frame/pool.go` - sync.Pool for buffers
- `internal/frame/*_test.go` - Unit tests

**Acceptance Criteria (Gherkin):**

```gherkin
Scenario: Frame package is leaf
  Given internal/frame package
  When I inspect its imports
  Then it should import ZERO internal engine packages
  And it should only import standard library packages

Scenario: Struct alignment
  Given frame structs (Point, Rect, Color, Bitmap)
  When I measure their memory layout
  Then they should be 64-byte cache-line aligned
  And padding should be minimal

Scenario: Buffer pool zero-allocation
  Given a frame buffer pool
  When I allocate and release 1000 buffers
  Then the benchmark should show 0 allocs/op
  And memory usage should remain constant
```

**Performance Requirements:**
- Buffer allocation: 0 allocs/op (via sync.Pool)
- Rect intersection: < 10ns
- Color blending: < 5ns

---

#### Epic 3: Network Transport with SSRF Guard

**Owner:** [AppSec] + [Core Dev]  
**Duration:** 2 days  
**Dependencies:** Epic 1

**Tasks:**

| Task ID | Task Name | Description | DoD |
| :--- | :--- | :--- | :--- |
| 3.1 | Implement HTTP client | `internal/net/client.go` với HTTP/1.1 + TLS 1.3 | Client handles redirects, timeouts |
| 3.2 | Implement SSRF guard | `internal/net/address_guard.go`: DialAddressGuarded() | Block private IPs at dial time |
| 3.3 | Define admission limits | `internal/net/limits.go`: 32MB HTML, 8MB CSS | Limits enforced before parsing |
| 3.4 | Implement cloud metadata block | Block 169.254.169.254 (AWS/GCP/Azure) | 100% drop rate |
| 3.5 | Write security tests | `test/gate/ssrf_guard_test.go` | All attack vectors covered |
| 3.6 | Write integration tests | `test/net/*_test.go` | Client works end-to-end |

**Deliverables:**
- `internal/net/client.go` - HTTP/1.1 client
- `internal/net/address_guard.go` - SSRF guard
- `internal/net/limits.go` - Admission limits
- `test/gate/ssrf_guard_test.go` - Security tests
- `test/net/*_test.go` - Integration tests

**Acceptance Criteria (Gherkin):**

```gherkin
Scenario: Block RFC 1918 private IPs
  Given a public origin (e.g., https://attacker.com)
  When it attempts to fetch http://192.168.1.1/admin
  Then the connection should be blocked at dial time
  And error should be "SSRF blocked: public origin attempted connection to private IP"

Scenario: Block cloud metadata endpoint
  Given a public origin
  When it attempts to fetch http://169.254.169.254/latest/meta-data/
  Then the connection should be unconditionally dropped
  And a security audit message should be logged

Scenario: Permit top-level localhost navigation
  Given the user explicitly types http://localhost:8080 in omnibox
  When the navigation executes as top-level user action
  Then the request to 127.0.0.1:8080 should be permitted
  And no SSRF error should be raised

Scenario: Enforce HTML size limit
  Given an HTML document of 33MB
  When the network client receives it
  Then the connection should be aborted
  And error should be "File exceeds safety budget"
```

**Security Requirements:**
- SSRF drop rate: 100% (zero bypasses)
- DNS rebinding protection: Pin connection to validated IP
- Admission limits: Enforced BEFORE parsing

---

#### Epic 4: DOM Parser - Bounded HTML5 Tree Construction

**Owner:** [Core Dev]  
**Duration:** 2 days  
**Dependencies:** Epic 1

**Tasks:**

| Task ID | Task Name | Description | DoD |
| :--- | :--- | :--- | :--- |
| 4.1 | Implement HTML tokenizer | `internal/dom/tokenizer.go` | Handles malformed tags gracefully |
| 4.2 | Implement tree builder | `internal/dom/parser.go`: ParseBounded() | 32MB cap, 128 depth, 200K nodes |
| 4.3 | Define Node struct | `internal/dom/tree.go`: NodeID, Parent, FirstChild, NextSibling | Flat arena allocation |
| 4.4 | Implement arena allocator | `internal/dom/arena.go` | Zero-pointer indirection |
| 4.5 | Implement atom table | `internal/dom/atoms.go` | Interned tag names |
| 4.6 | Write conformance tests | WPT HTML5 tree construction suite | 95%+ pass rate |

**Deliverables:**
- `internal/dom/tokenizer.go` - HTML tokenizer
- `internal/dom/parser.go` - ParseBounded()
- `internal/dom/tree.go` - Node struct
- `internal/dom/arena.go` - Arena allocator
- `internal/dom/atoms.go` - Atom table
- WPT conformance tests

**Acceptance Criteria (Gherkin):**

```gherkin
Scenario: Reject oversized HTML
  Given an HTML document of 33MB
  When ParseBounded() is called
  Then it should abort with error "File exceeds safety budget"
  And zero DOM nodes should be allocated

Scenario: Flatten deep nesting
  Given an HTML document with 150 levels of nesting
  When ParseBounded() is called
  Then nodes beyond level 128 should be flattened into siblings
  And parsing should continue without error

Scenario: Halt at node limit
  Given an HTML document with 250,000 nodes
  When ParseBounded() is called
  Then parsing should halt at 200,000 nodes
  And error should be "Node limit exceeded"

Scenario: DOM package is self-contained
  Given internal/dom package
  When I inspect its imports
  Then it should import ZERO engine packages (css, style, layout, paint, raster)
  And it should only import standard library packages
```

**Performance Requirements:**
- Parse 1MB HTML: < 50ms
- Memory: < 10MB for 10,000 nodes
- Arena allocation: 0 allocs/op after warmup

---

#### Epic 5: Style Cascade - CSS Parser & Resolver

**Owner:** [Core Dev]  
**Duration:** 3 days  
**Dependencies:** Epic 2, Epic 4

**Tasks:**

| Task ID | Task Name | Description | DoD |
| :--- | :--- | :--- | :--- |
| 5.1 | Implement CSS tokenizer | `internal/css/tokenizer.go` | CSS3 compliant |
| 5.2 | Implement stylesheet parser | `internal/css/parser.go` | Handles selectors, properties, values |
| 5.3 | Implement specificity calculator | `internal/style/cascade.go` | Inline > ID > Class > Element |
| 5.4 | Implement rule indexer | `internal/style/resolver.go` | Bucket rules by selector type |
| 5.5 | Implement computed style | `internal/style/computed.go` | Resolve inheritance, initial values |
| 5.6 | Define UA stylesheet | `internal/style/ua.go` | Default styles for HTML elements |
| 5.7 | Write WPT tests | CSS selector conformance suite | 95%+ pass rate |

**Deliverables:**
- `internal/css/tokenizer.go` - CSS tokenizer
- `internal/css/parser.go` - Stylesheet parser
- `internal/style/cascade.go` - Specificity calculation
- `internal/style/resolver.go` - Rule indexer
- `internal/style/computed.go` - Computed style
- `internal/style/ua.go` - UA stylesheet

**Acceptance Criteria (Gherkin):**

```gherkin
Scenario: Calculate specificity correctly
  Given CSS rules: "#id" (1,0,0), ".class" (0,1,0), "div" (0,0,1)
  When specificity is calculated
  Then "#id" should have specificity (1,0,0)
  And ".class" should have specificity (0,1,0)
  And "div" should have specificity (0,0,1)

Scenario: Resolve inheritance
  Given a parent element with color: red
  When a child element has no color specified
  Then the child should inherit color: red
  And computed style should reflect inherited value

Scenario: Cascade order
  Given two rules: "div { color: red }" and ".cls { color: blue }"
  When both match a <div class="cls">
  Then the computed color should be blue
  Because class selector has higher specificity than element selector

Scenario: Performance benchmark
  Given a document with 2,000 nodes
  When style cascade is computed
  Then it should complete in < 10ms
  And memory allocation should be < 1MB
```

**Performance Requirements:**
- Cascade 2,000 nodes: < 10ms
- Memory: < 1MB for computed styles
- Rule lookup: O(1) via indexer

---

#### Epic 6: Layout Arena - Box Generation & Formatting Contexts

**Owner:** [Core Dev]  
**Duration:** 3 days  
**Dependencies:** Epic 4, Epic 5

**Tasks:**

| Task ID | Task Name | Description | DoD |
| :--- | :--- | :--- | :--- |
| 6.1 | Define Object struct | `internal/layout/arena.go`: 64-byte cache-line aligned | Exactly 64 bytes |
| 6.2 | Implement block formatting context | `internal/layout/block.go` | Vertical box stacking |
| 6.3 | Implement inline formatting context | `internal/layout/inline.go` | Line breaking, text layout |
| 6.4 | Implement float positioning | `internal/layout/float.go` | Float left/right |
| 6.5 | Implement absolute positioning | `internal/layout/position.go` | Position: absolute, fixed |
| 6.6 | Implement slot handles | Generation counters, free list | Zero use-after-free |
| 6.7 | Write visual parity tests | Compare layout output với Chromium | 95%+ match |

**Deliverables:**
- `internal/layout/arena.go` - Object struct (64 bytes)
- `internal/layout/block.go` - Block formatting context
- `internal/layout/inline.go` - Inline formatting context
- `internal/layout/float.go` - Float positioning
- `internal/layout/position.go` - Absolute & fixed positioning

**Acceptance Criteria (Gherkin):**

```gherkin
Scenario: Object struct size
  Given layout.Object struct
  When I measure its size
  Then it should be exactly 64 bytes
  And fields should be cache-line aligned

Scenario: Block layout
  Given two block elements with height 100px each
  When layout is computed
  Then first block should be at Y=0
  And second block should be at Y=100
  And total height should be 200px

Scenario: Inline layout with line break
  Given an inline element with text "Hello World" that exceeds container width
  When layout is computed
  Then text should break into two lines
  And line height should match font metrics

Scenario: Float positioning
  Given a float: left element with width 200px
  When layout is computed
  Then element should be positioned at left edge
  And subsequent content should wrap around it

Scenario: Zero-allocation scroll
  Given a laid-out document
  When user scrolls
  Then layout should not reallocate boxes
  And benchmark should show 0 allocs/op
```

**Performance Requirements:**
- Layout 2,000 nodes: < 20ms
- Scroll: 0 allocs/op
- Memory: < 5MB for 10,000 boxes

---

#### Epic 7: Paint Builder - Display List Emission

**Owner:** [Core Dev]  
**Duration:** 2 days  
**Dependencies:** Epic 5, Epic 6

**Tasks:**

| Task ID | Task Name | Description | DoD |
| :--- | :--- | :--- | :--- |
| 7.1 | Define DisplayCmd struct | `internal/paint/commands.go`: 64-byte value-type | Exactly 64 bytes |
| 7.2 | Implement display list builder | `internal/paint/builder.go` | Traverse layout arena, emit commands |
| 7.3 | Implement z-index stacking | `internal/paint/layers.go` | Stacking context tree |
| 7.4 | Implement clip operations | PushClip, PopClip commands | Clipping regions correct |
| 7.5 | Freeze display list | Immutable after emission | Zero mutations after build |
| 7.6 | Write unit tests | Display list correctness | All tests PASS |

**Deliverables:**
- `internal/paint/commands.go` - DisplayCmd struct
- `internal/paint/builder.go` - Display list builder
- `internal/paint/layers.go` - Z-index stacking
- `internal/paint/clip.go` - Clip operations

**Acceptance Criteria (Gherkin):**

```gherkin
Scenario: DisplayCmd struct size
  Given paint.DisplayCmd struct
  When I measure its size
  Then it should be exactly 64 bytes
  And it should be a value-type (not interface)

Scenario: Display list immutability
  Given a built display list
  When I attempt to modify it
  Then the operation should be rejected
  And display list should remain frozen

Scenario: Z-index stacking order
  Given elements with z-index: 1, 2, 3
  When display list is built
  Then commands should be emitted in z-index order
  And higher z-index should paint on top

Scenario: Paint package imports
  Given internal/paint package
  When I inspect its imports
  Then it should only import: frame, css, layout, style
  And it should NOT import: raster, surface, engine
```

**Performance Requirements:**
- Build display list: < 5ms for 2,000 nodes
- Memory: < 2MB for display list
- Command emission: 0 allocs/op

---

#### Epic 8: Tile Rasterizer - CPU Worker Pool

**Owner:** [Core Dev]  
**Duration:** 2 days  
**Dependencies:** Epic 2, Epic 7

**Tasks:**

| Task ID | Task Name | Description | DoD |
| :--- | :--- | :--- | :--- |
| 8.1 | Implement tile scheduler | `internal/raster/scheduler.go` | Partition viewport into 256x256 tiles |
| 8.2 | Implement worker pool | `internal/raster/worker.go` | 4 threads default |
| 8.3 | Implement glyph atlas | `internal/raster/glyph.go` | Cache rendered glyphs |
| 8.4 | Implement tile renderer | `internal/raster/render.go` | Rasterize display commands to bitmaps |
| 8.5 | Implement dirty tile tracking | Damage rect intersection | Only re-rasterize changed tiles |
| 8.6 | Write benchmark tests | Frame pacing < 16.6ms | 60fps target |

**Deliverables:**
- `internal/raster/scheduler.go` - Tile scheduler
- `internal/raster/worker.go` - Worker pool
- `internal/raster/glyph.go` - Glyph atlas
- `internal/raster/render.go` - Tile renderer

**Acceptance Criteria (Gherkin):**

```gherkin
Scenario: Tile partitioning
  Given a viewport of 1920x1080
  When tile scheduler partitions it
  Then it should create 8x5 = 40 tiles (256x256 each)
  And each tile should have unique TileCoord

Scenario: Parallel rasterization
  Given 40 tiles to rasterize
  When worker pool (4 threads) processes them
  Then all tiles should be rasterized in parallel
  And total time should be < 16.6ms (60fps)

Scenario: Dirty tile tracking
  Given a rendered frame
  When a small region (100x100) changes
  Then only tiles intersecting that region should be marked dirty
  And only dirty tiles should be re-rasterized

Scenario: Raster package imports
  Given internal/raster package
  When I inspect its imports
  Then it should only import: frame, paint, surface (via interface)
  And it should NOT import: dom, layout, engine
```

**Performance Requirements:**
- Frame rasterization: < 16.6ms (60fps)
- Worker pool utilization: > 90%
- Glyph cache hit rate: > 95%
- Memory: < 10MB for tile bitmaps

---

#### Epic 9: Surface Composer - Vsync Presentation Loop

**Owner:** [Platform Eng]  
**Duration:** 2 days  
**Dependencies:** Epic 7, Epic 8

**Tasks:**

| Task ID | Task Name | Description | DoD |
| :--- | :--- | :--- | :--- |
| 9.1 | Define Scheduler interface | `internal/surface/scheduler.go` | Plan, Rasterize, TileBitmap |
| 9.2 | Implement UI loop | `internal/surface/loop.go` | Vsync pacing |
| 9.3 | Implement composer | `internal/surface/composer.go` | Damage-blit compositor |
| 9.4 | Implement present | `internal/surface/present.go` | Present to native window |
| 9.5 | Implement frame pacing | 60Hz/120Hz sync | Frame drops < 1% |
| 9.6 | Write benchmark tests | Scroll pacing < 16.6ms | 99th percentile |

**Deliverables:**
- `internal/surface/scheduler.go` - Scheduler interface
- `internal/surface/loop.go` - UI loop
- `internal/surface/composer.go` - Composer
- `internal/surface/present.go` - Present to window

**Acceptance Criteria (Gherkin):**

```gherkin
Scenario: Surface reaches raster via interface only
  Given internal/surface package
  When I inspect its imports
  Then it should NOT import internal/raster directly
  And it should reach raster only via Scheduler interface

Scenario: Vsync pacing
  Given a 60Hz display
  When surface loop runs
  Then frames should be presented every 16.6ms
  And frame drops should be < 1% (99th percentile)

Scenario: Damage-blit composition
  Given a frame with 5 dirty tiles
  When composer processes it
  Then only 5 tiles should be blitted to screen
  And cached tiles should be reused without re-rasterization

Scenario: Scroll performance
  Given a rendered document
  When user scrolls continuously
  Then frame pacing should remain < 16.6ms
  And benchmark should show 0 allocs/op
```

**Performance Requirements:**
- Frame pacing: < 16.6ms (60fps), < 8.33ms (120fps)
- Frame drops: < 1% (99th percentile)
- Scroll: 0 allocs/op
- Composition: < 2ms

---

#### Epic 10: Engine Session - Full Pipeline Integration

**Owner:** [Core Dev] + [Platform Eng]  
**Duration:** 3 days  
**Dependencies:** All above

**Tasks:**

| Task ID | Task Name | Description | DoD |
| :--- | :--- | :--- | :--- |
| 10.1 | Define Session struct | `internal/engine/session.go` | Holds all pipeline stages |
| 10.2 | Implement NewSession | Constructor with dependency injection | All stages initialized |
| 10.3 | Implement pipeline | Parse → Style → Layout → Paint → Raster → Present | End-to-end flow |
| 10.4 | Implement PaintChecked | `internal/engine/paint_checked.go` | For untrusted input |
| 10.5 | Enforce admission limits | `internal/engine/limits.go` | All limits active |
| 10.6 | Write integration tests | `test/entrypoints/limits_test.go` | All limits tested |
| 10.7 | Write visual parity tests | Compare 10 pages với Chromium | 95%+ match |

**Deliverables:**
- `internal/engine/session.go` - Session struct
- `internal/engine/pipeline.go` - Pipeline orchestration
- `internal/engine/paint_checked.go` - PaintChecked()
- `internal/engine/limits.go` - Admission limits
- `test/entrypoints/limits_test.go` - Integration tests
- Visual parity test suite

**Acceptance Criteria (Gherkin):**

```gherkin
Scenario: End-to-end rendering
  Given an HTML URL (e.g., https://example.com)
  When Session.NewSession() is called
  Then the page should be rendered to screen
  And visual parity should be 95%+ match với Chromium

Scenario: PaintChecked for untrusted input
  Given a binary that touches untrusted input
  When it calls PaintChecked()
  Then all admission limits should be enforced
  And malicious input should be rejected safely

Scenario: Cold start performance
  Given the Goosie binary
  When it launches and renders first frame
  Then cold start should be < 150ms
  And first interactive paint should be < 200ms

Scenario: Memory footprint
  Given an idle browser with 1 blank tab
  When memory is measured
  Then RSS should be < 30MB
  And zero memory leaks should be detected

Scenario: Race detector
  Given the full test suite
  When run with "go test -race ./..."
  Then zero data races should be detected
  And all tests should PASS
```

**Performance Requirements:**
- Cold start: < 150ms
- Idle memory: < 30MB
- First frame: < 200ms
- Visual parity: 95%+ match

---

## 3. Dependency Graph

```
Epic 1 (archtest)
  ↓
  ├──► Epic 2 (frame) ──┬──► Epic 5 (style) ──► Epic 6 (layout) ──┐
  │                      │                                          │
  │                      └──► Epic 8 (raster) ◄────────────────────┤
  │                                                                 │
  ├──► Epic 3 (net) ────────────────────────────────────────────► Epic 10 (engine)
  │                                                                 │
  └──► Epic 4 (dom) ──► Epic 5 (style) ──► Epic 6 (layout) ──► Epic 7 (paint) ──► Epic 9 (surface)
                                                                                      │
                                                                                      └──► Epic 10 (engine)
```

**Critical Path:**
Epic 1 → Epic 4 → Epic 5 → Epic 6 → Epic 7 → Epic 9 → Epic 10

**Total Duration:** 23 person-days (4 weeks with 1 engineer)

---

## 4. Sprint Schedule

### Week 1: Foundation & Core Data Structures

| Day | Morning | Afternoon | Deliverable |
| :--- | :--- | :--- | :--- |
| **Day 1** | Epic 1.1-1.2: Import rules | Epic 1.3-1.5: Cgo & globals tests | archtest PASS |
| **Day 2** | Epic 2.1-2.3: Geometry & tiles | Epic 2.4-2.6: Bitmap & pool | frame package complete |
| **Day 3** | Epic 3.1-3.2: HTTP client & SSRF | Epic 3.3-3.4: Limits & metadata block | net package complete |
| **Day 4** | Epic 4.1-4.3: Tokenizer & tree | Epic 4.4-4.6: Arena & atoms | dom package complete |
| **Day 5** | Review & refactor | Write documentation | Week 1 milestone ✅ |

### Week 2: Style & Layout

| Day | Morning | Afternoon | Deliverable |
| :--- | :--- | :--- | :--- |
| **Day 6** | Epic 5.1-5.3: CSS tokenizer & parser | Epic 5.4-5.5: Specificity & resolver | css package complete |
| **Day 7** | Epic 5.6-5.7: UA stylesheet & tests | Review style cascade | style package complete |
| **Day 8** | Epic 6.1-6.3: Object struct & block | Epic 6.4-6.5: Float & position | layout arena started |
| **Day 9** | Epic 6.6: Slot handles | Epic 6.7: Visual parity tests | layout package complete |
| **Day 10** | **Milestone 1: Static page renders** | Demo & review | 🎉 Milestone 1 ✅ |

### Week 3: Paint & Raster

| Day | Morning | Afternoon | Deliverable |
| :--- | :--- | :--- | :--- |
| **Day 11** | Epic 7.1-7.3: DisplayCmd & builder | Epic 7.4-7.6: Clips & tests | paint package complete |
| **Day 12** | Epic 8.1-8.3: Scheduler & workers | Epic 8.4-8.6: Renderer & benchmarks | raster package complete |
| **Day 13** | Epic 9.1-9.3: Scheduler interface & loop | Epic 9.4-9.6: Composer & present | surface package complete |
| **Day 14** | Integration testing | Bug fixes | Pipeline integrated |
| **Day 15** | Review & refactor | Write documentation | Week 3 milestone ✅ |

### Week 4: Integration & Polish

| Day | Morning | Afternoon | Deliverable |
| :--- | :--- | :--- | :--- |
| **Day 16** | Epic 10.1-10.3: Session & pipeline | Epic 10.4-10.5: PaintChecked & limits | engine package complete |
| **Day 17** | Epic 10.6: Integration tests | Epic 10.7: Visual parity tests | All tests PASS |
| **Day 18** | Performance optimization | Memory profiling | Benchmarks meet targets |
| **Day 19** | Security review | AppSec sign-off | Security gates PASS |
| **Day 20** | **Milestone 2: Sprint 1 Complete** | Demo & retrospective | 🎉 Sprint 1 Done! 🚀 |

---

## 5. Quality Gates & Definition of Done

### 5.1 Epic-Level DoD

Mỗi epic CHỈ ĐƯỢC coi là hoàn thành khi:

- ✅ Code tuân thủ 100% `SYSTEM_ARCHITECTURE_BLUEPRINT.md`
- ✅ `go test ./internal/archtest/` PASS (import rules, cgo, globals)
- ✅ `go test -race ./...` PASS (zero data races)
- ✅ Unit tests: 100% coverage cho critical paths
- ✅ Benchmark: Performance targets met (0 allocs/op, < Xms latency)
- ✅ Documentation: Inline comments + package README
- ✅ Code review: Approved by [CTO] or [TA]
- ✅ AppSec review: Security gates PASS (for Epic 3, 4, 10)

### 5.2 Sprint-Level DoD

Sprint 1 CHỈ ĐƯỢC coi là hoàn thành khi:

- ✅ All 10 epics complete
- ✅ End-to-end: HTML URL → rendered frame trên screen
- ✅ Visual parity: 95%+ pixel match với Chromium trên 10 test pages
- ✅ Performance:
  - Cold start < 150ms
  - Idle memory < 30MB
  - Scroll pacing < 16.6ms (99th percentile)
  - 0 allocs/op cho hot paths
- ✅ Security:
  - SSRF guard: 100% drop rate
  - Admission limits: All enforced
  - Zero vulnerabilities (AppSec sign-off)
- ✅ Quality:
  - `go test ./...` PASS
  - `go test -race ./...` PASS
  - `go test ./internal/archtest/` PASS
  - `go vet -all ./...` PASS
- ✅ Documentation:
  - Sprint retrospective document
  - Architecture decision records updated
  - User-facing changelog

### 5.3 Automated Test Gates

```bash
# Gate 1: Architecture integrity
go test ./internal/archtest/ -v
# Expected: PASS (zero violations)

# Gate 2: Race detector
go test -race ./...
# Expected: PASS (zero data races)

# Gate 3: Performance benchmarks
go test -bench=. ./internal/raster/
# Expected: 0 allocs/op cho scroll operations

# Gate 4: Visual parity
make parity
# Expected: 95%+ pixel match với Chromium

# Gate 5: Security validation
go test ./test/gate/ssrf_guard_test.go
# Expected: 100% drop rate on private IPs

# Gate 6: Integration tests
go test ./test/entrypoints/limits_test.go
# Expected: All admission limits enforced
```

---

## 6. Risk Assessment & Mitigation

| Risk | Probability | Impact | Mitigation | Owner |
| :--- | :--- | :--- | :--- | :--- |
| **Layout fidelity gap** | High | Medium | Iterative visual parity testing, fix gaps incrementally | [Core Dev] |
| **Performance regression** | Medium | High | Benchmark gates ở mỗi epic, 0 allocs/op requirement | [QA] |
| **SSRF bypass** | Low | Critical | AppSec review bắt buộc, fuzz testing | [AppSec] |
| **Architecture drift** | Medium | High | archtest tự động enforce invariants | [CTO] |
| **Scope creep** | High | Medium | PO strict priority, MoSCoW enforcement | [PO] |
| **Integration complexity** | Medium | Medium | Daily integration tests, early Epic 10 start | [Core Dev] |
| **Visual parity < 95%** | Medium | High | Early parity testing, Chromium reference fixtures | [QA] |

---

## 7. Resource Allocation

### 7.1 Team Roles & Responsibilities

| Role | Person | Epic Ownership | Time Commitment |
| :--- | :--- | :--- | :--- |
| **CEO** | Product Director | Sprint approval, milestone sign-off | 2 hours/week |
| **CTO** | Principal Architect | Epic 1, architecture review | 4 hours/week |
| **PO** | Product Owner | Backlog priority, acceptance criteria | 4 hours/week |
| **BA** | Lead Business Analyst | User stories, Gherkin scenarios | 2 hours/week |
| **TA** | Lead Technical Architect | Epic 2, struct design | 4 hours/week |
| **Core Dev** | Senior Systems Engineer | Epics 3-8, 10 | Full-time |
| **Platform Eng** | Platform & UI Engineer | Epic 9, native integration | Full-time |
| **QA** | Test Lead | Test strategy, benchmarks | 8 hours/week |
| **AppSec** | Security Engineer | Epic 3, security review | 4 hours/week |

### 7.2 Infrastructure Requirements

- **Development:** macOS ARM64 or x86_64, Go 1.21+, Chromium for parity testing
- **CI/CD:** GitHub Actions hoặc equivalent, headless Linux for automated tests
- **Testing:** Playwright for visual parity, fuzzing tools for security
- **Monitoring:** Benchmark tracking, memory profiling tools

---

## 8. Success Metrics & KPIs

### 8.1 Sprint 1 KPIs

| Metric | Target | Measurement Method |
| :--- | :--- | :--- |
| **Cold start latency** | < 150ms | Benchmark from binary launch to first frame |
| **Idle memory footprint** | < 30MB | RSS measurement with 1 blank tab |
| **Scroll frame pacing** | < 16.6ms (99th percentile) | Benchmark continuous scroll operations |
| **Visual parity** | 95%+ pixel match | Automated comparison với Chromium on 10 pages |
| **SSRF drop rate** | 100% | Security test suite |
| **Architecture compliance** | 100% | archtest gates |
| **Race conditions** | 0 | `go test -race ./...` |
| **Allocation hotspots** | 0 allocs/op | Benchmark critical paths |

### 8.2 Quality Metrics

| Metric | Target | Measurement |
| :--- | :--- | :--- |
| **Test coverage** | > 80% | `go test -cover ./...` |
| **Code review coverage** | 100% | All PRs reviewed by [CTO] or [TA] |
| **Documentation coverage** | 100% | All packages have README |
| **Bug density** | < 0.5 bugs/KLOC | Post-sprint bug tracking |
| **Technical debt** | < 5% | Code smell detection |

---

## 9. Appendices

### 9.1 Glossary

- **archtest:** Architecture test suite enforcing import rules, cgo confinement, zero globals
- **SSRF:** Server-Side Request Forgery - attack vector blocked by network guard
- **Display List:** Immutable list of paint commands emitted by layout stage
- **Tile:** 256x256 pixel unit rasterized by CPU worker pool
- **Vsync:** Vertical synchronization for smooth 60Hz/120Hz presentation
- **Arena:** Flat memory allocation strategy for cache-friendly data structures

### 9.2 References

- `docs/spec/PRODUCT_VISION_AND_STRATEGY.md` - Product vision and MoSCoW priorities
- `docs/spec/SOFTWARE_REQUIREMENTS_SPECIFICATION.md` - Functional & non-functional requirements
- `docs/spec/SYSTEM_ARCHITECTURE_BLUEPRINT.md` - Architecture design and ADRs
- `docs/roadmap-v2.md` - UX-focused roadmap with current status

### 9.3 Approval Signatures

| Role | Name | Signature | Date |
| :--- | :--- | :--- | :--- |
| **CEO** | Product Director | _________________ | 2026-11-01 |
| **CTO** | Principal Architect | _________________ | 2026-11-01 |
| **PO** | Product Owner | _________________ | 2026-11-01 |
| **TA** | Lead Technical Architect | _________________ | 2026-11-01 |
| **QA** | Test Lead | _________________ | 2026-11-01 |
| **AppSec** | Security Engineer | _________________ | 2026-11-01 |

---

**End of Sprint 1 Plan & Work Breakdown Structure**

*Document Version: 1.0.0*  
*Last Updated: 2026-11-01*  
*Next Review: Sprint 2 Planning (after Sprint 1 completion)*
