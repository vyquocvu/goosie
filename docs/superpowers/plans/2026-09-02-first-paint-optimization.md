# Stream A: First-Paint Render Pipeline Optimization — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reduce first-paint latency through architecture-first rework of the style and layout pipelines, gated by measurement. Zero pixel or behavior change.

**Architecture:** Measurement-first approach: establish baseline benchmarks + pixel-hash manifest, then rework style pipeline (atoms, dedup, bucketed matching, inline cache), then evidence-driven layout/display-list tuning. Each change is a separate commit verified by benchstat, full test suite, and pixel-hash manifest.

**Tech Stack:** Go 1.25+, existing `cmd/perf-review`, `scripts/bench.sh`, `internal/renderer`, `internal/css`, layoutgolden test harness.

---

## File Structure

**New files:**
- `docs/perf/pixel-manifest.json` — SHA-256 hashes of headless PNG output for fixture set
- `docs/perf/baseline-bench.txt` — baseline benchmark results
- `docs/perf/baseline-perf-review.json` — baseline perf-review stage timings
- `docs/perf/baseline-*.prof` — CPU/mem profiles
- `test/perf/pixel_hash_test.go` — pixel-reference harness test
- `test/perf/style_atoms_test.go` — unit tests for property atoms
- `test/perf/style_dedup_test.go` — unit tests for computed-style dedup
- `test/perf/bucketed_matching_test.go` — unit tests for bucketed matching
- `test/perf/inline_style_cache_test.go` — unit tests for inline-style cache

**Modified files:**
- `cmd/perf-review/main.go` — extend with offline fixture set and stage timings
- `internal/renderer/style.go` — property atoms, computed-style dedup, bucketed matching, inline cache
- `internal/renderer/node.go` — `Style` struct field type changes
- `internal/renderer/layout.go` — layout code updates for atom types
- `internal/css/properties.go` — atom definitions (if not already present)
- `internal/css/match_cache.go` — bucketed matching integration
- `internal/renderer/display_list.go` — allocation reductions (Phase 2)
- `internal/renderer/headless.go` — stage timing instrumentation (Phase 0)

---

## Phase 0 — Measurement Foundation

### Task 0.1: Extend perf-review with offline fixture set

**Files:**
- Modify: `cmd/perf-review/main.go`
- Create: `testdata/perf/typography_sample.html` (copy from `testdata/test_001_typography.html`)
- Create: `testdata/perf/layout_sample.html` (copy from `testdata/test_011_layout.html`)
- Create: `testdata/perf/large_page.html` (synthetic large page)

- [ ] **Step 1: Create offline fixture directory**

```bash
mkdir -p testdata/perf
cp testdata/test_001_typography.html testdata/perf/typography_sample.html
cp testdata/test_011_layout.html testdata/perf/layout_sample.html
```

- [ ] **Step 2: Generate synthetic large-page fixture**

Create `testdata/perf/large_page.html` with 1000+ nodes and 100+ CSS rules. Use a script or hand-craft a representative large page.

```html
<!DOCTYPE html>
<html>
<head>
<style>
/* 100+ rules */
.container { display: block; }
.item { margin: 10px; }
/* ... generate 100+ rules ... */
</style>
</head>
<body>
<div class="container">
  <!-- 1000+ nodes -->
  <div class="item">Item 1</div>
  <!-- ... generate 1000+ nodes ... -->
</div>
</body>
</html>
```

- [ ] **Step 3: Add offline fixture mode to perf-review**

Modify `cmd/perf-review/main.go` to add a `-fixtures` flag that loads from `testdata/perf/` instead of fetching live URLs. Add stage timing instrumentation to `RenderHTMLToImage` in `internal/renderer/headless.go`:

```go
// In internal/renderer/headless.go, add timing instrumentation:
func RenderHTMLToImage(ctx context.Context, htmlContent string, width, height int) (*image.RGBA, error) {
    // Add stage timing
    var stages map[string]time.Duration
    if os.Getenv("GOOSIE_PERF_STAGES") != "" {
        stages = make(map[string]time.Duration)
    }
    
    start := time.Now()
    doc, err := html.Parse(strings.NewReader(htmlContent))
    if stages != nil { stages["parse"] = time.Since(start) }
    // ... continue with stage timing for each phase
}
```

- [ ] **Step 4: Test offline fixture mode**

Run: `cd /Users/vyquocvu/Develop/Browser/goosie && GOOSIE_PERF_STAGES=1 go run ./cmd/perf-review -fixtures=testdata/perf/ -iterations=1 -json`

Expected: JSON output with per-stage timings for each fixture.

- [ ] **Step 5: Commit**

```bash
git add cmd/perf-review/main.go internal/renderer/headless.go testdata/perf/
git commit -m "perf: extend perf-review with offline fixture set and stage timings"
```

### Task 0.2: Create pixel-reference harness

**Files:**
- Create: `test/perf/pixel_hash_test.go`
- Create: `docs/perf/pixel-manifest.json`

- [ ] **Step 1: Write pixel-hash test**

Create `test/perf/pixel_hash_test.go`:

```go
package perf

import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "image/png"
    "os"
    "path/filepath"
    "testing"
    
    "github.com/vyquocvu/goosie/internal/renderer"
)

func TestPixelHashManifest(t *testing.T) {
    fixtures := []string{
        "testdata/perf/typography_sample.html",
        "testdata/perf/layout_sample.html",
        "testdata/perf/large_page.html",
    }
    
    manifestPath := "docs/perf/pixel-manifest.json"
    updateManifest := os.Getenv("UPDATE_MANIFEST") == "true"
    
    var manifest map[string]string
    if !updateManifest {
        data, err := os.ReadFile(manifestPath)
        if err != nil {
            t.Fatalf("failed to read manifest: %v", err)
        }
        if err := json.Unmarshal(data, &manifest); err != nil {
            t.Fatalf("failed to unmarshal manifest: %v", err)
        }
    } else {
        manifest = make(map[string]string)
    }
    
    for _, fixture := range fixtures {
        htmlContent, err := os.ReadFile(fixture)
        if err != nil {
            t.Fatalf("failed to read fixture %s: %v", fixture, err)
        }
        
        img, err := renderer.RenderHTMLToImage(context.Background(), string(htmlContent), 800, 600)
        if err != nil {
            t.Fatalf("failed to render %s: %v", fixture, err)
        }
        
        // Encode to PNG and compute hash
        var buf bytes.Buffer
        if err := png.Encode(&buf, img); err != nil {
            t.Fatalf("failed to encode PNG: %v", err)
        }
        hash := sha256.Sum256(buf.Bytes())
        hashStr := hex.EncodeToString(hash[:])
        
        if updateManifest {
            manifest[fixture] = hashStr
        } else {
            expected, ok := manifest[fixture]
            if !ok {
                t.Errorf("fixture %s not in manifest", fixture)
                continue
            }
            if hashStr != expected {
                t.Errorf("pixel hash mismatch for %s: got %s, want %s", fixture, hashStr, expected)
            }
        }
    }
    
    if updateManifest {
        data, _ := json.MarshalIndent(manifest, "", "  ")
        if err := os.WriteFile(manifestPath, data, 0644); err != nil {
            t.Fatalf("failed to write manifest: %v", err)
        }
        t.Logf("manifest updated at %s", manifestPath)
    }
}
```

- [ ] **Step 2: Generate initial manifest**

Run: `cd /Users/vyquocvu/Develop/Browser/goosie && UPDATE_MANIFEST=true go test -v ./test/perf -run TestPixelHashManifest`

Expected: `docs/perf/pixel-manifest.json` created with hashes for each fixture.

- [ ] **Step 3: Verify manifest check passes**

Run: `cd /Users/vyquocvu/Develop/Browser/goosie && go test -v ./test/perf -run TestPixelHashManifest`

Expected: PASS (no hash mismatches).

- [ ] **Step 4: Commit**

```bash
git add test/perf/pixel_hash_test.go docs/perf/pixel-manifest.json
git commit -m "perf: add pixel-reference harness with SHA-256 manifest"
```

### Task 0.3: Capture baseline

**Files:**
- Create: `docs/perf/baseline-bench.txt`
- Create: `docs/perf/baseline-perf-review.json`
- Create: `docs/perf/baseline-cpu.prof`
- Create: `docs/perf/baseline-mem.prof`

- [ ] **Step 1: Capture baseline benchmarks**

Run: `cd /Users/vyquocvu/Develop/Browser/goosie && ./scripts/bench.sh suite > docs/perf/baseline-bench.txt`

- [ ] **Step 2: Capture baseline perf-review timings**

Run: `cd /Users/vyquocvu/Develop/Browser/goosie && GOOSIE_PERF_STAGES=1 go run ./cmd/perf-review -fixtures=testdata/perf/ -iterations=5 -json > docs/perf/baseline-perf-review.json`

- [ ] **Step 3: Capture CPU/mem profiles**

Run:
```bash
cd /Users/vyquocvu/Develop/Browser/goosie
./scripts/bench.sh profile-cpu ./test/perf BenchmarkRenderHTMLToImage
mv cpu.prof docs/perf/baseline-cpu.prof
./scripts/bench.sh profile-mem ./test/perf BenchmarkRenderHTMLToImage
mv mem.prof docs/perf/baseline-mem.prof
```

If `BenchmarkRenderHTMLToImage` doesn't exist yet, add it in a later task. For now, profile an existing benchmark like `BenchmarkStress_MutationThroughput`.

- [ ] **Step 4: Commit baseline files**

```bash
git add docs/perf/baseline-*
git commit -m "perf: capture baseline benchmarks, perf-review timings, and profiles"
```

---

## Phase 1 — Style Pipeline Rework

### Task 1.1: Enumerated property atoms

**Files:**
- Modify: `internal/css/properties.go` (add atom types if not present)
- Modify: `internal/renderer/node.go` (change `Style` struct fields)
- Modify: `internal/renderer/style.go` (update style application to use atoms)
- Modify: `internal/renderer/layout.go` (update layout code to compare atoms)
- Create: `test/perf/style_atoms_test.go`

- [ ] **Step 1: Define atom types for enumerated properties**

In `internal/css/properties.go`, add atom types (if not already present):

```go
type DisplayAtom uint8

const (
    DisplayBlock DisplayAtom = iota
    DisplayInline
    DisplayNone
    DisplayFlex
    DisplayGrid
    DisplayInlineBlock
    DisplayTableRow
    DisplayTableCell
    // ... etc
)

func DisplayAtomFromString(s string) DisplayAtom {
    switch s {
    case "block": return DisplayBlock
    case "inline": return DisplayInline
    case "none": return DisplayNone
    // ... etc
    default: return DisplayBlock
    }
}

func (d DisplayAtom) String() string {
    switch d {
    case DisplayBlock: return "block"
    case DisplayInline: return "inline"
    // ... etc
    default: return "block"
    }
}
```

Repeat for `PositionAtom`, `FloatAtom`, `TextAlignAtom`, etc.

- [ ] **Step 2: Update `Style` struct to use atoms**

In `internal/renderer/node.go`, change:

```go
type Style struct {
    Display    DisplayAtom    // was string
    Position   PositionAtom   // was string
    Float      FloatAtom      // was string
    TextAlign  TextAlignAtom  // was string
    // ... other hot string fields
    // Keep non-hot fields as strings for now
}
```

- [ ] **Step 3: Update style application code**

In `internal/renderer/style.go`, update `applyDeclaration` and related functions to convert string values to atoms at parse boundaries.

- [ ] **Step 4: Update layout code**

In `internal/renderer/layout.go`, update comparisons from `if node.ComputedStyle.Display == "block"` to `if node.ComputedStyle.Display == css.DisplayBlock`.

- [ ] **Step 5: Write unit tests**

Create `test/perf/style_atoms_test.go`:

```go
func TestDisplayAtomConversion(t *testing.T) {
    tests := []struct{
        input string
        want css.DisplayAtom
    }{
        {"block", css.DisplayBlock},
        {"inline", css.DisplayInline},
        {"none", css.DisplayNone},
    }
    for _, tt := range tests {
        got := css.DisplayAtomFromString(tt.input)
        if got != tt.want {
            t.Errorf("DisplayAtomFromString(%q) = %v, want %v", tt.input, got, tt.want)
        }
    }
}
```

- [ ] **Step 6: Run tests**

Run: `cd /Users/vyquocvu/Develop/Browser/goosie && go test ./... && go test -v ./test/perf -run TestPixelHashManifest`

Expected: All tests pass, pixel hashes unchanged.

- [ ] **Step 7: Run layoutgolden**

Run: `cd /Users/vyquocvu/Develop/Browser/goosie && go test ./internal/renderer/layoutgolden/...`

Expected: All golden tests pass.

- [ ] **Step 8: Commit**

```bash
git add internal/css/properties.go internal/renderer/node.go internal/renderer/style.go internal/renderer/layout.go test/perf/style_atoms_test.go
git commit -m "perf: replace hot string fields in Style with interned atoms"
```

### Task 1.2: Computed-style deduplication & shared inheritance

**Files:**
- Modify: `internal/renderer/style.go` (add fingerprint + StylePool for `renderer.Style`)
- Modify: `internal/renderer/node.go` (add `InheritedStyle` pointer field)
- Create: `test/perf/style_dedup_test.go`

- [ ] **Step 1: Add Fingerprint method to `renderer.Style`**

In `internal/renderer/style.go`:

```go
func (s *Style) Fingerprint() uint64 {
    h := fnv.New64a()
    // Write atom fields
    h.Write([]byte{byte(s.Display), byte(s.Position), byte(s.Float)})
    // Write float fields
    var fbuf [8]byte
    putFloat32(fbuf[:4], s.FontSize)
    putFloat32(fbuf[4:], s.LineHeight)
    h.Write(fbuf[:8])
    // Write color fields
    if s.Color != nil {
        r, g, b, a := s.Color.RGBA()
        var buf [16]byte
        buf[0] = byte(r >> 8); buf[1] = byte(r)
        // ... etc
        h.Write(buf[:8])
    }
    // ... etc
    return h.Sum64()
}
```

- [ ] **Step 2: Add StylePool for deduplication**

```go
type StylePool struct {
    mu    sync.Mutex
    styles map[uint64]*Style
}

var globalStylePool = &StylePool{styles: make(map[uint64]*Style)}

func (p *StylePool) Intern(s *Style) *Style {
    fp := s.Fingerprint()
    p.mu.Lock()
    defer p.mu.Unlock()
    if existing, ok := p.styles[fp]; ok {
        return existing
    }
    p.styles[fp] = s
    return s
}
```

- [ ] **Step 3: Update ApplyStyles to use deduplication**

In `ApplyStyles`, after computing the style for a node, intern it:

```go
node.ComputedStyle = globalStylePool.Intern(node.ComputedStyle)
```

- [ ] **Step 4: Implement shared inheritance**

Add an `InheritedStyle` struct with only inherited fields. Store a pointer to the parent's `InheritedStyle` in child nodes. When a child overrides an inherited field, clone the `InheritedStyle` (copy-on-write).

- [ ] **Step 5: Write unit tests**

Create `test/perf/style_dedup_test.go`:

```go
func TestStyleDeduplication(t *testing.T) {
    s1 := &Style{Display: css.DisplayBlock, FontSize: 16}
    s2 := &Style{Display: css.DisplayBlock, FontSize: 16}
    
    interned1 := globalStylePool.Intern(s1)
    interned2 := globalStylePool.Intern(s2)
    
    if interned1 != interned2 {
        t.Errorf("expected identical styles to be deduplicated")
    }
}
```

- [ ] **Step 6: Run tests and verify pixel hashes**

Run: `cd /Users/vyquocvu/Develop/Browser/goosie && go test ./... && go test -v ./test/perf -run TestPixelHashManifest`

Expected: All tests pass, pixel hashes unchanged.

- [ ] **Step 7: Commit**

```bash
git add internal/renderer/style.go internal/renderer/node.go test/perf/style_dedup_test.go
git commit -m "perf: deduplicate computed styles and share inheritance via pointer"
```

### Task 1.3: Bucketed selector matching

**Files:**
- Modify: `internal/renderer/style.go` (partition prepared rules by right-most selector)
- Modify: `internal/css/match_cache.go` (integrate bucketed matching)
- Create: `test/perf/bucketed_matching_test.go`

- [ ] **Step 1: Add bucketing data structures**

In `internal/renderer/style.go`:

```go
type ruleBuckets struct {
    byTag   map[string][]preparedRule
    byClass map[string][]preparedRule
    byID    map[string][]preparedRule
    universal []preparedRule
}

func bucketRules(rules []preparedRule) *ruleBuckets {
    b := &ruleBuckets{
        byTag:   make(map[string][]preparedRule),
        byClass: make(map[string][]preparedRule),
        byID:    make(map[string][]preparedRule),
    }
    for _, rule := range rules {
        // Extract right-most compound selector
        // Bucket by tag, class, or ID
        // If universal (*), add to universal bucket
    }
    return b
}
```

- [ ] **Step 2: Update applyMatchingRules to use buckets**

```go
func (sm *StyleManager) applyMatchingRules(stylesheet *css.StyleSheet, node *RenderNode) {
    buckets := sm.getBuckets(stylesheet)
    
    // Collect candidate rules from relevant buckets
    var candidates []preparedRule
    if tag := node.TagName; tag != "" {
        candidates = append(candidates, buckets.byTag[tag]...)
    }
    for _, class := range node.Classes() {
        candidates = append(candidates, buckets.byClass[class]...)
    }
    if id := node.ID(); id != "" {
        candidates = append(candidates, buckets.byID[id]...)
    }
    candidates = append(candidates, buckets.universal...)
    
    // Sort candidates by specificity and source order
    sort.Slice(candidates, func(i, j int) bool {
        // ... specificity comparison
    })
    
    // Apply candidates
    for _, rule := range candidates {
        // ... apply rule
    }
}
```

- [ ] **Step 3: Write unit tests**

Create `test/perf/bucketed_matching_test.go`:

```go
func TestBucketedMatching(t *testing.T) {
    // Test that bucketed matching produces same results as linear scan
    // on a fixture with known rules
}
```

- [ ] **Step 4: Run tests and verify pixel hashes**

Run: `cd /Users/vyquocvu/Develop/Browser/goosie && go test ./... && go test -v ./test/perf -run TestPixelHashManifest`

Expected: All tests pass, pixel hashes unchanged.

- [ ] **Step 5: Commit**

```bash
git add internal/renderer/style.go internal/css/match_cache.go test/perf/bucketed_matching_test.go
git commit -m "perf: bucket selector matching by right-most compound selector"
```

### Task 1.4: Inline-style parse cache

**Files:**
- Modify: `internal/renderer/style.go` (add LRU cache for inline style parsing)
- Create: `test/perf/inline_style_cache_test.go`

- [ ] **Step 1: Add inline-style cache**

In `internal/renderer/style.go`:

```go
type inlineStyleCache struct {
    mu    sync.Mutex
    cache map[string][]css.Declaration
    limit int
}

var globalInlineStyleCache = &inlineStyleCache{
    cache: make(map[string][]css.Declaration),
    limit: 1000,
}

func (c *inlineStyleCache) Get(styleAttr string) ([]css.Declaration, bool) {
    c.mu.Lock()
    defer c.mu.Unlock()
    decls, ok := c.cache[styleAttr]
    return decls, ok
}

func (c *inlineStyleCache) Put(styleAttr string, decls []css.Declaration) {
    c.mu.Lock()
    defer c.mu.Unlock()
    if len(c.cache) >= c.limit {
        // Evict oldest (simple: clear half)
        for k := range c.cache {
            delete(c.cache, k)
            if len(c.cache) < c.limit/2 {
                break
            }
        }
    }
    c.cache[styleAttr] = decls
}
```

- [ ] **Step 2: Update applyInlineStyles to use cache**

```go
func (sm *StyleManager) applyInlineStyles(node *RenderNode, styleAttr string) {
    if decls, ok := globalInlineStyleCache.Get(styleAttr); ok {
        for _, decl := range decls {
            sm.applyDeclaration(node, decl)
        }
        return
    }
    
    // Parse and cache
    decls := parseInlineStyle(styleAttr)
    globalInlineStyleCache.Put(styleAttr, decls)
    for _, decl := range decls {
        sm.applyDeclaration(node, decl)
    }
}
```

- [ ] **Step 3: Write unit tests**

Create `test/perf/inline_style_cache_test.go`:

```go
func TestInlineStyleCache(t *testing.T) {
    cache := &inlineStyleCache{cache: make(map[string][]css.Declaration), limit: 10}
    
    styleAttr := "color: red; font-size: 16px"
    decls := []css.Declaration{{Property: "color", Value: "red"}}
    
    cache.Put(styleAttr, decls)
    got, ok := cache.Get(styleAttr)
    if !ok || len(got) != 1 {
        t.Errorf("cache miss or wrong length")
    }
}
```

- [ ] **Step 4: Run tests and verify pixel hashes**

Run: `cd /Users/vyquocvu/Develop/Browser/goosie && go test ./... && go test -v ./test/perf -run TestPixelHashManifest`

Expected: All tests pass, pixel hashes unchanged.

- [ ] **Step 5: Commit**

```bash
git add internal/renderer/style.go test/perf/inline_style_cache_test.go
git commit -m "perf: cache inline style parsing by attribute string"
```

---

## Phase 2 — Layout & Display-List Tuning (evidence-driven)

### Task 2.1: Intrinsic size memoization (if profile shows hotspot)

**Files:**
- Modify: `internal/renderer/intrinsic_size_cache.go` (extend with better key)
- Modify: `internal/renderer/layout.go` (use extended cache)

- [ ] **Step 1: Analyze profiles**

Run: `cd /Users/vyquocvu/Develop/Browser/goosie && go tool pprof -http=:8080 docs/perf/baseline-cpu.prof`

Look for hotspots in intrinsic size computation. If present, proceed.

- [ ] **Step 2: Extend intrinsic size cache key**

Update cache key to include (node structure hash, available width).

- [ ] **Step 3: Run tests and verify pixel hashes**

- [ ] **Step 4: Commit**

### Task 2.2: Text-run glyph pooling (if profile shows hotspot)

**Files:**
- Modify: `internal/renderer/headless.go` (pool `frame.Glyph` slices in `buildTextRun`)

- [ ] **Step 1: Analyze profiles**

Look for allocations in `buildTextRun`. If present, proceed.

- [ ] **Step 2: Add glyph slice pool**

```go
var glyphPool = sync.Pool{
    New: func() interface{} {
        return make([]frame.Glyph, 0, 64)
    },
}
```

- [ ] **Step 3: Run tests and verify pixel hashes**

- [ ] **Step 4: Commit**

### Task 2.3: convertPaintCommands allocation reduction (if profile shows hotspot)

**Files:**
- Modify: `internal/renderer/headless.go` (reduce allocations in `convertPaintCommands`)

- [ ] **Step 1: Analyze profiles**

Look for allocations in `convertPaintCommands`. If present, proceed.

- [ ] **Step 2: Pre-allocate output slice**

```go
out := make([]raster.DisplayCmd, 0, len(cmds))
```

(Already done — look for other allocation sources.)

- [ ] **Step 3: Run tests and verify pixel hashes**

- [ ] **Step 4: Commit**

---

## Final Verification

### Task 3: Final benchstat comparison

- [ ] **Step 1: Run full bench suite**

Run: `cd /Users/vyquocvu/Develop/Browser/goosie && ./scripts/bench.sh suite > docs/perf/final-bench.txt`

- [ ] **Step 2: Compare with baseline**

Run: `cd /Users/vyquocvu/Develop/Browser/goosie && ./scripts/bench.sh compare docs/perf/baseline-bench.txt docs/perf/final-bench.txt`

Expected: Measurable improvement in affected benchmarks.

- [ ] **Step 3: Run full test suite**

Run: `cd /Users/vyquocvu/Develop/Browser/goosie && go test ./...`

Expected: Zero failures.

- [ ] **Step 4: Verify pixel hashes**

Run: `cd /Users/vyquocvu/Develop/Browser/goosie && go test -v ./test/perf -run TestPixelHashManifest`

Expected: PASS (no hash changes).

- [ ] **Step 5: Run E2E on representative fixtures**

Run: `cd /Users/vyquocvu/Develop/Browser/goosie && go test -tags=e2e ./test/e2e -run TestComprehensiveSuite`

Expected: All comparisons pass.

- [ ] **Step 6: Commit final results**

```bash
git add docs/perf/final-bench.txt
git commit -m "perf: final benchstat comparison showing improvement vs baseline"
```

---

## Summary

This plan decomposes Stream A into 10 tasks across 3 phases:
- **Phase 0 (3 tasks):** Measurement foundation — offline fixtures, pixel-hash manifest, baseline capture.
- **Phase 1 (4 tasks):** Style pipeline rework — atoms, dedup, bucketed matching, inline cache.
- **Phase 2 (3 tasks):** Evidence-driven layout/display-list tuning.
- **Final (1 task):** Benchstat comparison and verification.

Each task is 2–8 hours of work, independently verifiable, and gated by benchstat + full test suite + pixel-hash manifest. Total estimated time: 6–10 days.
