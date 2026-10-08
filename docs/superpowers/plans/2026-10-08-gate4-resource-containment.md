# Gate 4: Safe Resource Access and Content Containment Implementation Plan

**Goal:** Implement safe resource admission, aggregate budgets, origin policy, and content containment for untrusted web content.

**Status:** Planning phase - October 8, 2026

## Analysis of Requirements

From `docs/roadmap-v2.md` section 5 (Ordered production gates):

> **Gate 4**: Safe resource access and content containment | Aggregate request/byte/time limits, local-file and network-origin policy, validated native/IPC boundaries, restricted content processes, cancellation/crash recovery, adversarial tests

From section 6 (Deferred items):

| Deferred item | Current evidence | Why separate |
| --- | --- | --- |
| Aggregate fetch/font budgets and cancellation | `internal/engine/imports.go:18-63`; `internal/engine/engine.go:187-215,333-367` | Requires a shared resource-loader lifecycle and policy |
| Origin, mixed-content, CSP and private-network rules | `internal/net/http.go:273-281`; `internal/engine/limits.go:571-584` | Policy work must accompany the relevant web capabilities |

## Gate 4 Scope (Phased Approach)

### Phase 1: Aggregate Resource Budgets ✅ In-progress
- Per-document byte budget across all subresources (images, fonts, CSS, scripts)
- Concurrent fetch/decode limit with reservation system
- Timeout/deadline for each resource family
- Graceful degradation on budget exhaustion

### Phase 2: Origin & Network Policy 
- Strict HTTPS-to-HTTPS redirect validation
- Mixed-content blocking (HTTP→HTTPS origins can't load HTTP)
- Private network access prevention from public pages
- CORS enforcement for cross-origin requests

### Phase 3: Content Process Boundaries (Future - requires redesign)
- Separate process for untrusted content (not in current scope)
- IPC boundary validation
- Sandbox policies

## Implementation Strategy

### Files to Modify
- `internal/engine/engine.go` - Resource loader interface, concurrent fetch coordination
- `internal/engine/limits.go` - Add aggregate budget constants and validators
- `internal/net/http.go` - Enforce origin policy at transport layer
- `internal/image/decode.go` - Image-specific budgets and reservations  
- New test files in `test/engine/` for comprehensive adversarial testing

### Constants to Add (`internal/engine/limits.go`)

```go
const (
    // Aggregate budgets per document
    MaxSubresourceBytes     = 128 << 20        // 128 MiB total raw bytes
    MaxImagesPerDocument    = 256              // Unique image references
    MaxFontsPerDocument     = 64               // Font face count
    MaxCSSImports           = 64               // @import expansion depth
    maxLinkedSheets         = 32               // <link rel=stylesheet>
    
    // Time budgets per family (already mostly present)
    cssFetchBudget          = 6 * time.Second  // Linked sheets
    imageFetchBudget        = 6 * time.Second  // Raster images  
    fontFetchBudget         = 3 * time.Second  // Custom fonts
    
    // Individual resource caps
    MaxEncodedImageBytes    = 8 << 20          // 8 MiB encoded
    MaxImageDimension       = 8192             // 8K max side
    MaxDecodedImagePixels   = 16 * 1024 * 1024 // 16MP max pixels
    MaxRasterCacheBytes     = 64 << 20         // 64 MiB raster cache
)
```

### Key Interfaces

#### 1. SubresourceBudget (Atomic accounting)
```go
type subBudget struct {
    mu   sync.Mutex
    used int64
}

func (b *subBudget) take(n int) bool {
    b.mu.Lock()
    defer b.mu.Unlock()
    if b.used+int64(n) > maxSubresourceBytes {
        return false
    }
    b.used += int64(n)
    return true
}
```

#### 2. FetchGuard (Deadline + concurrency control)
```go
type fetchGuard struct {
    deadline time.Time
    workers  chan struct{} // semaphore
    done     chan struct{} // cancellation signal
}
```

## Task Breakdown

### Task 1: Define Budget Constants and Validators

**Files:** `internal/engine/limits.go`, new test file

**Steps:**
- [ ] Document existing constants (already have many!)
- [ ] Add missing constants for aggregate budgets
- [ ] Export validator interfaces for external use
- [ ] Write unit tests for budget math

### Task 2: Implement Aggregate Byte Budget

**Files:** `internal/engine/limits.go`, `internal/engine/engine.go`

**Steps:**
- [ ] Design shared budget structure (`subBudget` already exists! ✓)
- [ ] Add budget tracking to Session construction
- [ ] Coordinate budgets across images/fonts/stylesheets
- [ ] Implement graceful rejection when exhausted
- [ ] Tests: exact equality, one-pixel overflow, no double-release

### Task 3: Strengthen Origin Policy at Network Layer

**Files:** `internal/net/http.go`, `internal/engine/limits.go`

**Steps:**
- [ ] Validate HTTPS redirects reject HTTP → HTTP transitions
- [ ] Add strict mixed-content blocking rules
- [ ] Enforce private-network address guards
- [ ] Tests: redirect chains, mixed-content attempts, localhost URLs

### Task 4: Resource Family Concurrency Limits

**Files:** `internal/engine/engine.go`

**Steps:**
- [ ] Design per-family worker pools (images, fonts, CSS)
- [ ] Implement concurrent fetch scheduling with limits
- [ ] Add cancellation propagation on tab/navigation change
- [ ] Tests: concurrency under load, cancellation propagation

### Task 5: Adversarial Testing Suite

**New files:** `test/engine/adversarial_test.go`

**Steps:**
- [ ] Create test cases for extreme inputs
- [ ] Flood scenario: 257 unique images
- [ ] Dimension attack: 8193×1 image
- [ ] Redirect loop detection
- [ ] Memory pressure simulation

### Task 6: Integration with Existing Code

**Files:** All modified above + `cmd/goosie/main.go`, `cmd/goosie-headless/main.go`

**Steps:**
- [ ] Wire budgets into engine.Session creation
- [ ] Update loaders to honor cancellation
- [ ] Verify backward compatibility
- [ ] Performance regression checks

## Acceptance Criteria

1. **Tests pass:** All adversarial tests pass without panic or excessive allocation
2. **Race-free:** `-race` detector clean under concurrent stress
3. **Graceful degradation:** Budget exhaustion doesn't crash browser
4. **Policy enforcement:** Mixed-content attacks blocked
5. **Valid documents still render:** No false negatives on normal pages

## Implementation Order

1. **Task 1** - Budget constants (1-2 hours)
2. **Task 2** - Aggregate byte budget (3-4 hours)
3. **Task 3** - Origin policy fixes (2-3 hours)
4. **Task 4** - Concurrency limits (2-3 hours)
5. **Task 5** - Adversarial tests (2-3 hours)
6. **Task 6** - Integration (1-2 hours)

**Total estimate:** 11-17 hours of focused work

## Risk Assessment

- **Low risk:** Most infrastructure already present (`subBudget`, `ValidateViewport`, etc.)
- **Medium risk:** Concurrent budget coordination needs careful locking
- **Known limitation:** True sandboxing requires process separation (future gate)

## Dependencies

- Gate 1 (Guarded Rendering) - **COMPLETE**
- Gate 2 (Tab Lifecycle) - **COMPLETE**  
- Gate 3 (Browser Surface) - **COMPLETE**
- Future Gates 5-8 - Not required for Gate 4

## Next Steps

Ready to begin implementation following the phased approach above. Starting with Task 1 to audit existing code and add any missing constants.
