# SP-1: Debug Subsystem Removal + Hover Hardening — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the duplicated debug/phase-tracking subsystem landed in `1bb471a` and harden the hover-flash fix it shipped with, restoring a clean `go vet` gate.

**Architecture:** Two commits on `main`. Commit 1 deletes `internal/debug`, the two debug binaries, the engine phase-tracking hooks, and their docs/archtest entries — leaving `WorkCounts` + `FrameRecorder` as the single measurement infrastructure. Commit 2 makes `HasTextAt` band merging order-independent (sort by y0, merge only on vertical overlap AND horizontal touch), hardens the no-`:hover` work-delta test, trims comments, and deletes the now-dead `HoverDamageRect`.

**Tech Stack:** Go 1.26, goja JS runtime, `go test`/`go vet`, archtest import rules.

**Spec:** `docs/superpowers/specs/2026-11-11-sp1-debug-removal-hover-hardening-design.md`

**Base:** `main` at `b2c9db8` (spec commit), working tree clean.

---

### Task 1: Delete debug subsystem files

**Files:**
- Delete: `internal/debug/overlay.go`, `internal/debug/phase_tracker.go`, `internal/debug/profiler.go`, `internal/debug/render_debug.go`
- Delete: `cmd/debug-render/main.go`, `cmd/test-render/main.go`
- Delete: `internal/engine/phase_tracking.go`
- Delete: `docs/debug-tools.md`, `docs/phase-tracking.md`
- Keep (do NOT touch): `testdata/render/debug-test.html`, `docs/README.md`, `docs/spec/*`

- [ ] **Step 1: Delete the files**

```bash
git rm -r internal/debug cmd/debug-render cmd/test-render
git rm internal/engine/phase_tracking.go docs/debug-tools.md docs/phase-tracking.md
```

Expected: 10 files removed from index and disk. Build is broken until Task 2 — that is expected.

### Task 2: Revert engine/paint hooks and archtest entries

**Files:**
- Modify: `internal/engine/engine.go` (field ~332-335, option ~662-669, NewSession hook ~780-794, Reflow hook ~1900-1909)
- Modify: `internal/engine/paint.go` (hook ~97-106, `dom` import line 6)
- Modify: `internal/archtest/rules.go:47,50`

- [ ] **Step 1: Remove the `phaseTracker` field from `Session` in `internal/engine/engine.go`**

Delete these lines (keep `deferredActive int32` and the closing brace):

```go

	// phaseTracker tracks per-node render counts and detects phase offsets.
	// nil when debug tracking is disabled.
	phaseTracker interface{} // *debug.PhaseTracker, use interface to avoid import cycle
```

- [ ] **Step 2: Remove `WithPhaseTracker` from `internal/engine/engine.go`**

Delete:

```go
// WithPhaseTracker enables per-node render count tracking and phase offset detection.
// The tracker parameter should be a *debug.PhaseTracker; we use interface{} to avoid
// an import cycle.
func WithPhaseTracker(tracker interface{}) Option {
	return func(s *Session) {
		s.phaseTracker = tracker
	}
}

```

- [ ] **Step 3: Remove the NewSession hook in `internal/engine/engine.go`**

Delete the block between `s.workCounts.styleNodeVisits.Add(uint64(len(s.Styles)))` and `s.PseudoStyles = ...`:

```go
	// Track per-node style counts for debug phase tracking
	if tracker := s.PhaseTracker(); tracker != nil {
		nodeIDs := make([]dom.NodeID, 0, len(s.Styles))
		for id := range s.Styles {
			nodeIDs = append(nodeIDs, id)
		}
		tracker.TrackStyleBatch(nodeIDs)
		// Register node metadata for debug output
		doc.Node.Walk(func(n *dom.Node) bool {
			if n.Element() {
				tracker.RegisterNode(n.ID, n.Data, n.GetAttribute("class"), n.GetAttribute("id"))
			}
			return true
		})
	}
```

- [ ] **Step 4: Remove the Reflow hook in `internal/engine/engine.go`**

Delete the block between `candidate.NaturalSizes = s.natural` and `layout.Block(candidate, ...)`:

```go
	// Track per-node layout counts for debug phase tracking
	if tracker := s.PhaseTracker(); tracker != nil {
		nodeIDs := make([]dom.NodeID, 0, len(candidate.Objects))
		for _, obj := range candidate.Objects {
			if obj.Node != nil {
				nodeIDs = append(nodeIDs, obj.Node.ID)
			}
		}
		tracker.TrackLayoutBatch(nodeIDs)
	}
```

- [ ] **Step 5: Remove the Paint hook in `internal/engine/paint.go`**

Delete the block between `b.Build(layout.ObjectID(1))` and the z-index comment:

```go
	// Track per-node paint counts for debug phase tracking
	if tracker := s.PhaseTracker(); tracker != nil && s.Arena != nil {
		nodeIDs := make([]dom.NodeID, 0, len(s.Arena.Objects))
		for _, obj := range s.Arena.Objects {
			if obj.Node != nil {
				nodeIDs = append(nodeIDs, obj.Node.ID)
			}
		}
		tracker.TrackPaintBatch(nodeIDs)
	}
```

Then remove the now-unused import line `"github.com/vyquocvu/goosie/internal/dom"` from paint.go's import block (the hook was its only user; `go build` will confirm).

- [ ] **Step 6: Revert `internal/archtest/rules.go`**

Delete the line:

```go
	"internal/debug":             {"internal/dom", "internal/frame", "internal/paint"},
```

And change the engine entry from:

```go
	"internal/engine":            {"internal/ax", "internal/dom", "internal/css", "internal/style", "internal/layout", "internal/paint", "internal/frame", "internal/image", "internal/js", "internal/media", "internal/userscript", "internal/debug"},
```

to:

```go
	"internal/engine":            {"internal/ax", "internal/dom", "internal/css", "internal/style", "internal/layout", "internal/paint", "internal/frame", "internal/image", "internal/js", "internal/media", "internal/userscript"},
```

- [ ] **Step 7: Verify no dangling references**

```bash
grep -rn "internal/debug\|PhaseTracker\|WithPhaseTracker\|debug-render\|test-render\|debug-tools\|phase-tracking" --include="*.go" --include="*.md" . | grep -v "^./docs/superpowers/" | grep -v "^./.git"
```

Expected: no matches (the spec/plan docs under `docs/superpowers/` are excluded; they describe the removal). If a kept doc references a removed file, fix that reference.

- [ ] **Step 8: Verify build, vet, tests**

```bash
go build ./... && go vet -all ./... && go test ./internal/archtest/ ./internal/engine/ ./cmd/goosie/ ./test/engine/
```

Expected: all PASS; vet clean (the copylocks failure is gone with `internal/debug`).

- [ ] **Step 9: Commit 1**

```bash
git add -A internal/engine/engine.go internal/engine/paint.go internal/archtest/rules.go
git commit -m "refactor(debug): remove the parallel phase-tracking subsystem

PhaseTracker duplicated WorkCounts and missed every incremental restyle
path, the two debug binaries duplicated -bench/-gate, and Stats copied a
mutex (go vet copylocks). WorkCounts and FrameRecorder remain the single
measurement infrastructure."
```

### Task 3: Order-independent HasTextAt — failing test

**Files:**
- Test: `test/engine/hover_test.go` (append)

- [ ] **Step 1: Write the failing test**

Append to `test/engine/hover_test.go`:

```go
// TestHasTextAtOutOfDocumentOrder covers absolutely-positioned text that sits
// above later-in-document flow text: selWords returns document order, which is
// not vertical order, so the hit test must not stop at the first band.
func TestHasTextAtOutOfDocumentOrder(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin:0">
		<p style="position:absolute;left:0;top:200px;margin:0">Below</p>
		<p style="margin:0">Above</p>
	</body></html>`)
	x, y := textCenter(t, s, "Above")
	if !s.HasTextAt(x, y) {
		t.Fatalf("HasTextAt(%v, %v) = false over out-of-order text", x, y)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

```bash
go test ./test/engine/ -run TestHasTextAtOutOfDocumentOrder -v
```

Expected: FAIL — the current early-break loop stops at the "Below" band (y≈200) before reaching "Above" (y≈0).

### Task 4: Order-independent HasTextAt — implementation

**Files:**
- Modify: `internal/engine/selection.go:118-167` (HasTextAt + textLineRect), import block lines 3-10

- [ ] **Step 1: Add `sort` to the import block**

```go
import (
	"sort"
	"strings"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/paint"
)
```

- [ ] **Step 2: Replace `HasTextAt` and `textLineRect` (lines 132-192) with:**

```go
// HasTextAt reports whether a document-space point lies on a selectable text
// line. The hit is the line box, not the glyph box: line-height leading
// between wrapped lines must keep the I-beam down instead of flickering.
func (s *Session) HasTextAt(x, y float32) bool {
	for _, b := range s.textLineBands() {
		if x >= b.x0 && x < b.x1 && y >= b.y0 && y < b.y1 {
			return true
		}
	}
	return false
}

type textBand struct{ x0, y0, x1, y1 float32 }

// textLineBands merges word line-rects into visual lines. Document order is
// not vertical order (absolute/float/flex content), so sort first; merge only
// when bands overlap vertically and touch horizontally, keeping the gutter
// between side-by-side columns out of the hit area.
func (s *Session) textLineBands() []textBand {
	words := s.selWords()
	bands := make([]textBand, 0, len(words))
	for _, w := range words {
		x0, y0, x1, y1 := textLineRect(w)
		bands = append(bands, textBand{x0, y0, x1, y1})
	}
	sort.Slice(bands, func(i, j int) bool { return bands[i].y0 < bands[j].y0 })
	merged := bands[:0]
	for _, b := range bands {
		if n := len(merged); n > 0 {
			last := &merged[n-1]
			if b.y0 < last.y1 && b.y1 > last.y0 && b.x0 <= last.x1 && b.x1 >= last.x0 {
				if b.x0 < last.x0 {
					last.x0 = b.x0
				}
				if b.x1 > last.x1 {
					last.x1 = b.x1
				}
				if b.y0 < last.y0 {
					last.y0 = b.y0
				}
				if b.y1 > last.y1 {
					last.y1 = b.y1
				}
				continue
			}
		}
		merged = append(merged, b)
	}
	return merged
}

// textLineRect grows the border box by half the explicit line-height leading.
// line-height:normal (LineHeight <= 0) keeps the border rect; its font leading
// is handled with the shaping work.
func textLineRect(w *layout.Object) (x0, y0, x1, y1 float32) {
	if w == nil {
		return
	}
	x0, y0, x1, y1 = w.BorderRect()
	if w.Style == nil || w.Style.LineHeight <= 0 {
		return
	}
	h := y1 - y0
	if w.Style.LineHeight <= h {
		return
	}
	extra := (w.Style.LineHeight - h) / 2
	y0 -= extra
	y1 += extra
	return
}
```

Note: `merged := bands[:0]` reuses the backing array in place; the range reads `bands` by value before each write position, which is safe because the write index never exceeds the read index.

- [ ] **Step 3: Run the new test plus the existing hover/selection suites**

```bash
go test ./test/engine/ -run "TestHasTextAt" -v && go test ./test/engine/ ./internal/engine/ ./cmd/goosie/
```

Expected: `TestHasTextAtOutOfDocumentOrder`, `TestHasTextAtHover`, `TestHasTextAtIncludesLineLeading` PASS; all other tests PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/engine/selection.go test/engine/hover_test.go
git commit -m "fix(selection): make the text hit test independent of document order

selWords walks the arena in document order, which absolute/float content
can put out of vertical order; the early-break band merge then missed
text. Sort bands by y and merge only on vertical overlap plus horizontal
touch so column gutters stay out of the hit area."
```

### Task 5: Column-gutter regression test

**Files:**
- Test: `test/engine/hover_test.go` (append)

- [ ] **Step 1: Write the test**

Append to `test/engine/hover_test.go`:

```go
// TestHasTextAtExcludesColumnGutter keeps side-by-side columns from fusing
// into one band: the gutter between them is not text.
func TestHasTextAtExcludesColumnGutter(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin:0">
		<p style="float:left;width:100px;margin:0">Left column text</p>
		<p style="float:left;width:100px;margin:0">Right column text</p>
	</body></html>`)
	lx, ly := textCenter(t, s, "Left")
	rx, ry := textCenter(t, s, "Right")
	if ly != ry {
		t.Fatalf("fixture columns are not side by side: left y=%v right y=%v", ly, ry)
	}
	gutter := (lx + rx) / 2
	if s.HasTextAt(gutter, ly) {
		t.Fatalf("HasTextAt(%v, %v) = true in the gutter between columns", gutter, ly)
	}
	if !s.HasTextAt(lx, ly) || !s.HasTextAt(rx, ry) {
		t.Fatal("HasTextAt lost the column text itself")
	}
}
```

- [ ] **Step 2: Run it**

```bash
go test ./test/engine/ -run TestHasTextAtExcludesColumnGutter -v
```

Expected: PASS with Task 4's implementation. If the fixture does not place the columns side by side (guard fails), adjust widths until it does — do not weaken the assertion. (This test would FAIL against the pre-Task-4 union-merge code, which fused both columns into one band.)

- [ ] **Step 3: Commit**

```bash
git add test/engine/hover_test.go
git commit -m "test(selection): guard the column gutter against band fusion"
```

### Task 6: Harden the no-:hover work-delta test

**Files:**
- Modify: `cmd/goosie/render_work_test.go:375-394` (`TestHoverWithoutPseudoClassDoesNotRepaint`)

- [ ] **Step 1: Replace the test with:**

```go
func TestHoverWithoutPseudoClassDoesNotRepaint(t *testing.T) {
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}
	sess, err := engine.NewSession(`<html><body style="margin:0">
		<p id="a">This domain is for use in documentation examples without needing permission.</p>
		<p id="b">This is not a service; avoid relying on it for testing and monitoring purposes.</p>
		<script>
			window.enters = 0;
			document.querySelectorAll('p').forEach(function (p) {
				p.addEventListener('mouseenter', function () {
					window.enters++;
					document.body.setAttribute('data-enters', String(window.enters));
				});
			});
		</script>
	</body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })
	f.tabMgr.Active().SetSession(sess)
	_, x1, y1 := inputNodeAndCenter(t, sess, "a")
	_, x2, y2 := inputNodeAndCenter(t, sess, "b")
	before := sess.WorkCounts()
	f.hoverContent(x1, y1)
	f.hoverContent(x2, y2)
	f.hoverContent(x1, y1)
	assertWorkDelta(t, "hover across text without :hover", sess.WorkCounts(), before, engine.WorkCounts{})
	// The zero delta must mean "hover changed but painted nothing", not
	// "hit-testing never fired": a→b→a dispatches three mouseenter events.
	got, err := sess.EvaluateJS(`document.body.getAttribute('data-enters')`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "3" {
		t.Fatalf("mouseenter count = %q, want 3 (hover state never changed?)", got)
	}
}
```

Rationale: `SetHover` (`internal/engine/focus.go:3027`) dispatches `mouseenter` synchronously; the listener only writes a render-irrelevant `data-*` attribute, so the zero work delta still holds. The script runs during `NewSession`, before the `before` snapshot.

- [ ] **Step 2: Run it**

```bash
go test ./cmd/goosie/ -run TestHoverWithoutPseudoClassDoesNotRepaint -v
```

Expected: PASS. If `data-enters` reads `""` or `<nil>`, the listener did not run — investigate `EvaluateJS`/script execution before touching the assertion.

- [ ] **Step 3: Commit**

```bash
git add cmd/goosie/render_work_test.go
git commit -m "test(hover): assert the no-:hover zero-delta test actually hovers"
```

### Task 7: Trim comments, delete dead HoverDamageRect

**Files:**
- Modify: `cmd/goosie/interaction.go:188-192,203-203`
- Modify: `internal/platform/darwin/shim_darwin.m:1102-1107`
- Modify: `internal/engine/hover_optimization.go:9-19`

- [ ] **Step 1: Trim the hoverContent comments in `cmd/goosie/interaction.go`**

Replace the 8-line comment before `if !sess.HasPseudoClassRule("hover")` with:

```go
	// No :hover rules means no paint change; repainting anyway flashed the
	// window over viewport-sized CSS-pixel damage. Pending DOM mutations are
	// painted by the next PumpJS turn, not by a partial repaint here.
```

Replace the 3-line comment before `f.repaintTab(tab, frame.Rect{})` with:

```go
	// Empty damage diffs display lists in device pixels; the hovered border
	// box is CSS pixels, misses descendant ink, and spans the document for body.
```

- [ ] **Step 2: Trim the shim comment in `internal/platform/darwin/shim_darwin.m`**

Replace the 6-line comment before `[view setLayerContentsRedrawPolicy:...]` with:

```objc
		// The frame path owns layer.contents; the default redraw policy clears
		// the layer on cursor/accessibility updates and flashes the window
		// until the next present.
```

- [ ] **Step 3: Delete `HoverDamageRect` from `internal/engine/hover_optimization.go`**

Delete lines 9-32 (the doc comment and the whole `func (s *Session) HoverDamageRect() frame.Rect` body). Keep the `frame` import — `FocusDamageRect`/`SelectionDamageRect` still use it. Zero callers were verified repo-wide.

- [ ] **Step 4: Verify**

```bash
go build ./... && go vet -all ./... && go test ./cmd/goosie/ ./internal/engine/ ./test/engine/
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/goosie/interaction.go internal/platform/darwin/shim_darwin.m internal/engine/hover_optimization.go
git commit -m "chore(hover): trim rationale comments and drop dead HoverDamageRect"
```

### Task 8: Full verification

**Files:** none (verification only)

- [ ] **Step 1: Full suite + race + archtest**

```bash
go build ./... && go vet -all ./... && go test ./... && go test -race ./cmd/goosie/ ./internal/engine/ ./test/engine/ ./internal/archtest/
```

Expected: everything PASS, vet clean.

- [ ] **Step 2: Render parity (if the oracle is available)**

```bash
make parity
```

Expected: corpus scores at or above the pre-change baseline (104 fixtures, ~96% average per roadmap 2026-11-02). If Playwright/Chromium is not installed, record "parity not run: no local oracle" in the completion summary — do not install browsers silently.

- [ ] **Step 3: Record limits**

Note in the completion summary: the `shim_darwin.m` redraw-policy change cannot be verified natively in a display-less session (comment-only change in this plan; the policy line itself landed in `1bb471a`).

- [ ] **Step 4: Consolidate into the two spec commits**

The spec requires exactly two commits. Tasks 2-7 produced granular commits on top of the spec commit `b2c9db8`; squash them into two (local history only, nothing pushed):

```bash
git reset --soft b2c9db8
git reset
git add internal/debug cmd/debug-render cmd/test-render internal/engine/phase_tracking.go docs/debug-tools.md docs/phase-tracking.md internal/engine/engine.go internal/engine/paint.go internal/archtest/rules.go
git commit -m "refactor(debug): remove the parallel phase-tracking subsystem

PhaseTracker duplicated WorkCounts and missed every incremental restyle
path, the two debug binaries duplicated -bench/-gate, and Stats copied a
mutex (go vet copylocks). WorkCounts and FrameRecorder remain the single
measurement infrastructure."
git add internal/engine/selection.go test/engine/hover_test.go cmd/goosie/render_work_test.go cmd/goosie/interaction.go internal/platform/darwin/shim_darwin.m internal/engine/hover_optimization.go
git commit -m "fix(hover): order-independent text hit test and harden the hover fix

Sort line bands by y and merge only on vertical overlap plus horizontal
touch so out-of-order content hits and column gutters do not. Assert the
no-:hover zero-delta test actually hovers, trim rationale comments, and
drop the dead HoverDamageRect."
```

Note: `git add` on the deleted paths stages the deletions (they are already removed from disk and index by Task 1's `git rm`; after the soft reset they show as staged deletions — `git add -A <path>` also works).

- [ ] **Step 5: Final history check**

```bash
git --no-pager log --oneline b2c9db8..HEAD && git status --short && go build ./... && go test ./internal/archtest/
```

Expected: exactly 2 commits, clean working tree, build and archtest PASS.