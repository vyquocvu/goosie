# SP-5 flow-root + paint-identity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax. Approval gates are waived by standing delegation; commits on `main` are authorized, **no push**.

**Goal:** Land `display:flow-root` (completing L2) and remove the last long-lived arena pointer from engine state (completing L5), both verified against Chromium and the zero-diff corpus.

**Architecture:** Parse `flow-root` to `DisplayBlock` + a `FlowRoot` flag; `startsNewFC` honors the flag, reusing the existing float-containment path. Migrate `findMark.obj` to a `BoxKey` value; add an archtest rule forbidding held arena-pointer fields in engine state.

**Tech Stack:** Go, goja-free layout/style engine, Playwright/Chromium oracle, `make parity`.

**Standing rules:** gofmt -w before every commit. NEVER pipe test output through `tail` (masks exit codes) — redirect to /tmp logs and read them. Revert mutations with `git checkout` ONLY after the surrounding work is committed. Zero-diff gate: `make parity` 105/105 ≥96.39%, gaps 4 fixtures/6 bands unchanged; gate counters unchanged. Read `docs/roadmap-v2.md` with sed/Grep (Read fails on its size).

---

## Task 1: Parse `flow-root` to a block with a FlowRoot flag

**Files:**
- Modify: `internal/style/style.go` (ComputedStyle `:213`; call sites `:1762`, `:2238`; helper near `parseDisplay` `:3718`)
- Test: `test/style/flow_root_parse_test.go`

- [ ] **Step 1: Write the failing test**

Create `test/style/flow_root_parse_test.go`:

```go
package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/style"
)

func parseOne(t *testing.T, decl string) *style.ComputedStyle {
	t.Helper()
	sheet := css.ParseForViewport("div{"+decl+"}", 800, 600)
	r := style.NewResolver([]*css.Stylesheet{sheet})
	doc := parseDoc(t, `<html><body><div id="x"></div></body></html>`)
	styles := r.ResolveViewport(doc, 800, 600)
	n := doc.ElementByID("x")
	cs := styles[n.ID]
	if cs == nil {
		t.Fatal("no computed style for #x")
	}
	return cs
}

func TestFlowRootParsesToBlockWithFlag(t *testing.T) {
	cs := parseOne(t, "display:flow-root")
	if !cs.FlowRoot {
		t.Error("FlowRoot = false, want true for display:flow-root")
	}
	if cs.Display != style.DisplayBlock {
		t.Errorf("Display = %v, want DisplayBlock for flow-root", cs.Display)
	}
}

func TestPlainBlockHasNoFlowRootFlag(t *testing.T) {
	cs := parseOne(t, "display:block")
	if cs.FlowRoot {
		t.Error("FlowRoot = true for plain display:block")
	}
}
```

NOTE: confirm the real resolver/parse helpers before running. Read `test/style/` for an existing test that builds a document and resolves styles (e.g. how `css.ParseForViewport`, `style.NewResolver`, `ResolveViewport`, and document parsing are actually called) and match those signatures exactly — replace `parseDoc`/`NewResolver`/`ResolveViewport` above with the real helpers used by neighboring tests. The assertions (`cs.FlowRoot`, `cs.Display`) are the contract.

- [ ] **Step 2: Run it red**

Run: `go test ./test/style -run 'TestFlowRootParsesToBlockWithFlag|TestPlainBlockHasNoFlowRootFlag' -count=1 > /tmp/sp5-t1.log 2>&1; echo $?; cat /tmp/sp5-t1.log`
Expected: FAIL — `cs.FlowRoot undefined`.

- [ ] **Step 3: Add the field and helper**

In `internal/style/style.go`, add to `ComputedStyle` immediately after `Display Display` (`:213`):

```go
	Display     Display
	FlowRoot    bool
```

Add a helper just above `parseDisplay` (`:3718`):

```go
// applyDisplay sets the used display and records flow-root, which is block-level
// but establishes a formatting context. parseDisplay alone cannot carry the flag.
func applyDisplay(cs *ComputedStyle, value string) {
	cs.FlowRoot = value == "flow-root"
	cs.Display = parseDisplay(value)
	if cs.FlowRoot {
		cs.Display = DisplayBlock
	}
}
```

Route both call sites through it:
- `:1762` `st.Display = parseDisplay(val)` → `applyDisplay(st, val)`
- `:2238` `cs.Display = parseDisplay(value)` → `applyDisplay(cs, value)`

- [ ] **Step 4: Run it green**

Run: `go test ./test/style -run 'TestFlowRootParsesToBlockWithFlag|TestPlainBlockHasNoFlowRootFlag' -count=1 > /tmp/sp5-t1.log 2>&1; echo $?; cat /tmp/sp5-t1.log`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/style/style.go test/style/flow_root_parse_test.go
git add internal/style/style.go test/style/flow_root_parse_test.go
git commit -m "feat(style): parse display:flow-root to a block with a FlowRoot flag (SP-5 L2)"
```

---

## Task 2: `startsNewFC` and `layoutStyleKey` honor FlowRoot

**Files:**
- Modify: `internal/layout/block.go` (`startsNewFC` `:2212`)
- Modify: `internal/engine/style_diff.go` (struct `:13`, `makeLayoutStyleKey` `:98`)
- Test: `test/layout/fc_flow_root_test.go`, `test/engine/style_diff_flow_root_test.go`

- [ ] **Step 1: Write the failing layout test**

Create `test/layout/fc_flow_root_test.go` (reuse the `buildBijectionArena` helper from `test/layout/boxkey_bijection_test.go`, which builds and lays out a fragment at 800×600):

```go
package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
)

// A flow-root block contains its float: its height grows to the float bottom.
func TestFlowRootContainsFloat(t *testing.T) {
	a := buildBijectionArena(t,
		`<div id="fr" style="display:flow-root;width:300px"><div style="float:left;width:100px;height:80px"></div>text</div>`,
		"")
	n := findNodeByID(a, "fr")
	id, ok := a.ForNode(n.ID)
	if !ok {
		t.Fatal("no layout object for #fr")
	}
	_, y0, _, y1 := a.Get(id).BorderRect()
	if h := y1 - y0; h < 80 {
		t.Errorf("flow-root height = %v, want >= 80 (must contain its float)", h)
	}
}

// A plain block does NOT contain its float (control).
func TestPlainBlockDoesNotContainFloat(t *testing.T) {
	a := buildBijectionArena(t,
		`<div id="pl" style="width:300px"><div style="float:left;width:100px;height:80px"></div>text</div>`,
		"")
	n := findNodeByID(a, "pl")
	id, _ := a.ForNode(n.ID)
	_, y0, _, y1 := a.Get(id).BorderRect()
	if h := y1 - y0; h >= 80 {
		t.Errorf("plain block height = %v, want < 80 (float must escape)", h)
	}
}

var _ = style.DisplayBlock
var _ = layout.FCBlockFlow
```

NOTE: confirm `buildBijectionArena`'s exact signature and whether a `findNodeByID` helper exists in `test/layout/`. If not, add a small local helper that walks `a.Objects` for `obj.Node.GetAttribute("id")==id` (mirror `clickPoint` in `test/engine/focus_test.go`). Drop the unused `var _` lines if the imports end up used. The contract: flow-root height ≥ 80, plain height < 80.

- [ ] **Step 2: Run it red**

Run: `go test ./test/layout -run 'TestFlowRootContainsFloat|TestPlainBlockDoesNotContainFloat' -count=1 > /tmp/sp5-t2.log 2>&1; echo $?; cat /tmp/sp5-t2.log`
Expected: `TestFlowRootContainsFloat` FAIL (height < 80, float escapes because `startsNewFC` is false for a flow-root block).

- [ ] **Step 3: Honor FlowRoot in startsNewFC**

In `internal/layout/block.go` `startsNewFC` (`:2212`), after the block-level switch (`:2215-2217`) and before the overflow check (`:2218`), add:

```go
	if s.FlowRoot {
		return true
	}
```

- [ ] **Step 4: Run the layout test green**

Run: `go test ./test/layout -run 'TestFlowRootContainsFloat|TestPlainBlockDoesNotContainFloat' -count=1 > /tmp/sp5-t2.log 2>&1; echo $?; cat /tmp/sp5-t2.log`
Expected: PASS.

- [ ] **Step 5: Add FlowRoot to layoutStyleKey + test**

In `internal/engine/style_diff.go`, add to the struct (`:14` line) a field:

```go
	Display, Position, Float, Clear               uint8
	FlowRoot                                      bool
```

and to `makeLayoutStyleKey` (`:98`):

```go
		Display: uint8(s.Display), Position: uint8(s.Position), Float: uint8(s.Float), Clear: uint8(s.Clear),
		FlowRoot: s.FlowRoot,
```

Create `test/engine/style_diff_flow_root_test.go`:

```go
package engine

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/style"
)

func TestLayoutStyleKeyDistinguishesFlowRoot(t *testing.T) {
	block := &style.ComputedStyle{Display: style.DisplayBlock}
	flowRoot := &style.ComputedStyle{Display: style.DisplayBlock, FlowRoot: true}
	if makeLayoutStyleKey(block) == makeLayoutStyleKey(flowRoot) {
		t.Error("layoutStyleKey ignores FlowRoot; a flow-root toggle must be layout-affecting")
	}
}
```

NOTE: this test is in `package engine` (internal) to reach `makeLayoutStyleKey`. Confirm neighboring `style_diff` tests use `package engine`; if they use `engine_test`, move the assertion to an exported seam or match the existing package. The contract: the two keys differ.

- [ ] **Step 6: Run green + regression**

Run: `go test ./test/layout ./test/engine ./internal/engine -count=1 > /tmp/sp5-t2reg.log 2>&1; echo $?; cat /tmp/sp5-t2reg.log`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
gofmt -w internal/layout/block.go internal/engine/style_diff.go test/layout/fc_flow_root_test.go test/engine/style_diff_flow_root_test.go
git add internal/layout/block.go internal/engine/style_diff.go test/layout/fc_flow_root_test.go test/engine/style_diff_flow_root_test.go
git commit -m "feat(layout): flow-root establishes a formatting context (SP-5 L2)"
```

---

## Task 3: Promote the fc-containment fixture into layout-parity (Component A acceptance)

**Files:**
- Move: `testdata/fc-containment/nested-float-containment.html` → `testdata/layout-parity/float-containment-001.html`
- Generate: `testdata/layout-parity/float-containment-001.reference.json`
- Modify: `scripts/verify-fc-containment-geometry.js` (fixture path)

- [ ] **Step 1: Measure Goosie BEFORE promoting (confirm the fix worked)**

Temporarily copy the fixture into layout-parity and generate its reference, then run the parity test to see whether Goosie now matches:

```bash
cp testdata/fc-containment/nested-float-containment.html testdata/layout-parity/float-containment-001.html
python3 testdata/gen_layout_refs.py > /tmp/sp5-t3gen.log 2>&1; echo gen=$?
go test ./test/engine -run 'TestLayoutParityWithChromium/float-containment-001' -count=1 -v > /tmp/sp5-t3.log 2>&1; echo $?; cat /tmp/sp5-t3.log
```

Expected: PASS (all boxes within 1px). If any box still diverges, STOP and investigate the block-row placement of a sole inline-block child (`block.go:294-368`, `flowsInlineContent` `:2159`) before promoting — record the divergence. `gen_layout_refs.py` regenerates ALL references; confirm `git status` shows only the new `float-containment-001.reference.json` changed (others byte-identical); `git checkout` any churned existing reference.

- [ ] **Step 2: Finalize the move**

If green, remove the now-redundant non-gated copies and point the anchor at the promoted fixture:

```bash
git rm testdata/fc-containment/nested-float-containment.html testdata/fc-containment/nested-float-containment.reference.json
```

In `scripts/verify-fc-containment-geometry.js`, change the `FIXTURE` path to
`../testdata/layout-parity/float-containment-001.html` and update the header comment
(the fixture is now gated by `TestLayoutParityWithChromium`).

- [ ] **Step 3: Run the anchor + the full layout-parity gate**

Run: `node scripts/verify-fc-containment-geometry.js > /tmp/sp5-t3anchor.log 2>&1; echo $?; tail -1 /tmp/sp5-t3anchor.log`
Expected: exit 0, `FC CONTAINMENT ORACLE OK`. (Here `tail -1` reads a recorded log file, not a live test pipe — acceptable.)
Run: `go test ./test/engine -run TestLayoutParityWithChromium -count=1 > /tmp/sp5-t3lp.log 2>&1; echo $?; cat /tmp/sp5-t3lp.log`
Expected: PASS (9 fixtures now).

- [ ] **Step 4: Commit**

```bash
git add testdata/layout-parity/float-containment-001.html testdata/layout-parity/float-containment-001.reference.json scripts/verify-fc-containment-geometry.js
git commit -m "test(layout): gate nested float/margin flow-root containment vs Chromium (SP-5 L2)"
```

---

## Task 4: Component A zero-diff verification + mutation

**Files:** none (verification only)

- [ ] **Step 1: Full matrix**

```bash
go build ./... > /tmp/sp5-a-build.log 2>&1; echo build=$?
gofmt -l internal cmd test > /tmp/sp5-a-fmt.log 2>&1; cat /tmp/sp5-a-fmt.log
go vet -all ./... > /tmp/sp5-a-vet.log 2>&1; echo vet=$?
go test ./... -count=1 > /tmp/sp5-a-all.log 2>&1; echo all=$?
go test ./internal/archtest/ -count=1 > /tmp/sp5-a-arch.log 2>&1; echo arch=$?
go test ./test/gate -run TestGate -count=1 > /tmp/sp5-a-gate.log 2>&1; echo gate=$?
go test ./test/qc -count=1 > /tmp/sp5-a-qc.log 2>&1; echo qc=$?
make parity > /tmp/sp5-a-parity.log 2>&1; echo parity=$?; sed -n '112,116p' /tmp/sp5-a-parity.log
```

Expected: all 0; parity 105/105 ≥96.39%, gaps 4/6 unchanged. If a timing-sensitive surface/zoom test flakes under load, re-run it in isolation to confirm (as in SP-4 T10) — do not skip a real failure.

- [ ] **Step 2: Mutation evidence**

Temporarily remove the `if s.FlowRoot { return true }` branch from `startsNewFC`, run `go test ./test/engine -run 'TestLayoutParityWithChromium/float-containment-001' -count=1` → expect FAIL (flow-root height collapses). Revert with `git checkout internal/layout/block.go` (safe: Task 2 committed). Re-run → PASS. Record the red message.

- [ ] **Step 3: Confirm clean tree**

`git status --short` → clean. No push.

---

## Task 5: Migrate `findMark.obj` to a BoxKey value (L5)

**Files:**
- Modify: `internal/engine/engine.go` (`findMark` `:2000`, `Find` `:2056`)
- Modify: `internal/engine/paint.go` (`:65`)
- Test: `test/engine/find_identity_test.go`

- [ ] **Step 1: Write the failing test**

Create `test/engine/find_identity_test.go` (use `newFocusSession` from `test/engine/focus_test.go`):

```go
package engine_test

import (
	"testing"
)

// Find highlights survive a paint and address boxes by key, not a held pointer.
func TestFindMarksPaintByKey(t *testing.T) {
	s := newFocusSession(t, `<p>alpha beta gamma alpha</p>`)
	matches := s.Find("alpha")
	if len(matches) != 2 {
		t.Fatalf("Find(alpha) = %d matches, want 2", len(matches))
	}
	if _, err := s.PaintChecked(); err != nil {
		t.Fatalf("PaintChecked after Find: %v", err)
	}
	s.SetFindActive(1)
	if _, err := s.PaintChecked(); err != nil {
		t.Fatalf("PaintChecked after SetFindActive: %v", err)
	}
}
```

NOTE: confirm `PaintChecked`'s real signature (it may take a viewport/args). Read a neighboring `test/engine` test that calls `PaintChecked` and match it. The contract: Find records marks and Paint succeeds with an active match.

- [ ] **Step 2: Run it (baseline green)**

Run: `go test ./test/engine -run TestFindMarksPaintByKey -count=1 > /tmp/sp5-t5.log 2>&1; echo $?; cat /tmp/sp5-t5.log`
Expected: PASS already (behavior unchanged). This test guards the migration; the red comes from the archtest rule in Task 6, not here.

- [ ] **Step 3: Migrate the struct and producers**

In `internal/engine/engine.go`:

```go
type findMark struct {
	key   layout.BoxKey
	rect  Match
	match int
}
```

`:2056` `marks = append(marks, findMark{obj: words[first], rect: rect, match: len(matches)})` →
`marks = append(marks, findMark{key: words[first].Key, rect: rect, match: len(matches)})`

In `internal/engine/paint.go` `:65` `Key: m.obj.Key,` → `Key: m.key,`

- [ ] **Step 4: Run green + regression**

Run: `go test ./test/engine ./internal/engine -count=1 > /tmp/sp5-t5reg.log 2>&1; echo $?; cat /tmp/sp5-t5reg.log`
Expected: PASS. Grep to confirm no remaining `m.obj` / `findMark{obj:` references: `grep -rn "\.obj" internal/engine/paint.go` and `grep -rn "findMark{" internal/engine/engine.go`.

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/engine/engine.go internal/engine/paint.go test/engine/find_identity_test.go
git add internal/engine/engine.go internal/engine/paint.go test/engine/find_identity_test.go
git commit -m "refactor(engine): key find marks by BoxKey, not a held arena pointer (SP-5 L5)"
```

---

## Task 6: Archtest forbids held arena-pointer fields in engine state

**Files:**
- Modify: `internal/archtest/rules.go` (add `ArenaPointerFields` near `ArenaPointerMaps` `:432`)
- Test: `internal/archtest/pointer_field_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/archtest/pointer_field_test.go` (package `archtest_test`, reuse `writePackageGo`/`moduleRoot` from `globals_test.go`/`boundary_test.go`):

```go
package archtest_test

import (
	"path/filepath"
	"testing"

	"github.com/vyquocvu/goosie/internal/archtest"
)

func TestEngineHasNoArenaPointerFields(t *testing.T) {
	root := moduleRoot(t)
	found, err := archtest.ArenaPointerFields(filepath.Join(root, "internal", "engine"))
	if err != nil {
		t.Fatalf("ArenaPointerFields: %v", err)
	}
	if len(found) != 0 {
		for _, f := range found {
			t.Errorf("held arena pointer field: %s", f)
		}
		t.Fatalf("engine state holds %d *layout.Object field(s); use BoxKey", len(found))
	}
}

func TestArenaPointerFieldsFlagsHeldPointersOnly(t *testing.T) {
	dir := t.TempDir()
	writePackageGo(t, dir, "dirty.go", `package p
import "github.com/vyquocvu/goosie/internal/layout"
type held struct { obj *layout.Object }
`)
	writePackageGo(t, dir, "clean.go", `package p
import "github.com/vyquocvu/goosie/internal/layout"
type keyed struct { key layout.BoxKey }
type fn struct { scroll func(obj *layout.Object) float64 }
`)
	found, err := archtest.ArenaPointerFields(dir)
	if err != nil {
		t.Fatalf("ArenaPointerFields: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("flagged %d fields, want 1 (only the held *layout.Object field)", len(found))
	}
}
```

NOTE: confirm `writePackageGo`'s real signature (it may take a package name or write a go.mod). Read `globals_test.go`/`boundary_test.go` and match. The contract: a held `*layout.Object` field is flagged; a `BoxKey` field and a func-typed field with a `*layout.Object` parameter are NOT.

- [ ] **Step 2: Run it red**

Run: `go test ./internal/archtest -run 'ArenaPointerFields' -count=1 > /tmp/sp5-t6.log 2>&1; echo $?; cat /tmp/sp5-t6.log`
Expected: FAIL — `archtest.ArenaPointerFields undefined`.

- [ ] **Step 3: Add the rule**

In `internal/archtest/rules.go`, after `ArenaPointerMaps` (`:466`), add:

```go
// ArenaPointerField is a struct field that holds a *layout.Object. Engine state
// must name boxes by BoxKey, not hold arena pointers that dangle after a reflow.
type ArenaPointerField struct {
	File string
	Line int
}

func (f ArenaPointerField) String() string {
	return fmt.Sprintf("%s:%d: struct field holds *layout.Object", filepath.ToSlash(f.File), f.Line)
}

// ArenaPointerFields flags struct fields typed *layout.Object under dir. It is
// scoped to engine state by the caller; paint's walk-local overlays are not
// scanned. Func-typed fields and parameters are not fields and are not flagged.
func ArenaPointerFields(dir string) ([]ArenaPointerField, error) {
	pkgs, err := goFilesByPackage(dir)
	if err != nil {
		return nil, err
	}
	dirs := make([]string, 0, len(pkgs))
	for d := range pkgs {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	var found []ArenaPointerField
	for _, d := range dirs {
		for _, path := range pkgs[d] {
			fset, file, err := parseGo(path)
			if err != nil {
				return nil, err
			}
			aliases := layoutImportNames(file)
			if len(aliases) == 0 {
				continue
			}
			ast.Inspect(file, func(n ast.Node) bool {
				st, ok := n.(*ast.StructType)
				if !ok || st.Fields == nil {
					return true
				}
				for _, fld := range st.Fields.List {
					if isLayoutObjectPointer(fld.Type, aliases) {
						found = append(found, ArenaPointerField{File: path, Line: fset.Position(fld.Pos()).Line})
					}
				}
				return true
			})
		}
	}
	return found, nil
}
```

- [ ] **Step 4: Run green**

Run: `go test ./internal/archtest -run 'ArenaPointerFields' -count=1 > /tmp/sp5-t6.log 2>&1; echo $?; cat /tmp/sp5-t6.log`
Expected: PASS (engine has zero held fields after Task 5; dirty fixture flagged, clean exempt).

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/archtest/rules.go internal/archtest/pointer_field_test.go
git add internal/archtest/rules.go internal/archtest/pointer_field_test.go
git commit -m "test(archtest): forbid held arena-pointer fields in engine state (SP-5 L5)"
```

---

## Task 7: L5 verification + mutation

**Files:** none (verification only)

- [ ] **Step 1: Race + find regression**

Run: `go test -race ./internal/engine/ ./test/engine/ ./internal/archtest/ -count=1 > /tmp/sp5-t7race.log 2>&1; echo race=$?; cat /tmp/sp5-t7race.log`
Expected: 0.

- [ ] **Step 2: Mutation evidence**

Temporarily re-add an `obj *layout.Object` field to `findMark` (alongside `key`), run `go test ./internal/archtest -run TestEngineHasNoArenaPointerFields -count=1` → expect FAIL (held field flagged). Revert with `git checkout internal/engine/engine.go` (safe: Task 5 committed). Re-run → PASS. Record the red message.

- [ ] **Step 3: Confirm clean tree**

`git status --short` → clean. No push.

---

## Task 8: Full matrix + roadmap/review sign-off

**Files:**
- Modify: `docs/roadmap-v2.md` (SP-5 status block), `docs/engine-compatibility-review.md` (§4.2/§4.5/§6 L2/L5 status)

- [ ] **Step 1: Full matrix**

Run the same matrix as Task 4 Step 1 plus the race subset into `/tmp/sp5-final-*.log`. Expected: all 0; parity 105/105 ≥96.39%, gaps 4/6 unchanged; gate counters unchanged.

- [ ] **Step 2: Roadmap SP-5 block**

In `docs/roadmap-v2.md`, after the SP-4 block, add an SP-5 status block (Vietnamese prose, "đã có trong mã / đã kiểm chứng" distinction). Record: L2 complete (`flow-root` parsed to block+flag, `startsNewFC` honors it, `float-containment-001` gated vs Chromium); L5 complete (`findMark` keyed by BoxKey, `ArenaPointerFields` archtest guard); zero-diff parity result; L3/L4 deferred to SP-6+ with the terrain evidence (hard pseudo-veto assertions, no per-rule Deps, full-rebuild reality, missing L1 cross-reflow persistence). No native CUA, no push.

- [ ] **Step 3: Review status notes**

In `docs/engine-compatibility-review.md`, update the §4.2 status (L2 second landing now landed via flow-root, not the containment refactor), §4.5 (findMark migrated; transient overlays documented as walk-local; archtest widened to fields), and §6 L2/L5 status lines. Keep additive; correct stale line refs found in the terrain map (anon veto `arena.go:189-203/399-425`; `startsNewFC` `block.go:2212`).

- [ ] **Step 4: Commit docs**

```bash
git add docs/roadmap-v2.md docs/engine-compatibility-review.md
git commit -m "docs(sp5): record L2 flow-root + L5 paint-identity completion; defer L3/L4"
```

- [ ] **Step 5: Final history check**

Run: `git --no-pager log --oneline 03bc86d7..HEAD && git status --short`
Expected: SP-5 spec + task commits, clean tree. No push.

---

## Self-review notes

- **Spec coverage:** flow-root parse (spec §2) → T1; startsNewFC + layoutStyleKey (§2) → T2; fixture promotion acceptance (§6) → T3; zero-diff + mutation (§6/7) → T4; findMark migration (§3) → T5; archtest widening (§3) → T6; L5 verification (§6) → T7; docs/sign-off (§7) → T8. L3/L4 explicitly deferred (§1).
- **Type consistency:** `FlowRoot bool` on ComputedStyle used in `applyDisplay`, `startsNewFC`, `layoutStyleKey`; `findMark.key layout.BoxKey` replaces `.obj` in Find + paint; `ArenaPointerField`/`ArenaPointerFields` mirror `ArenaPointerMap`/`ArenaPointerMaps`.
- **Read-before-edit points:** real `test/style` resolver helpers (T1), `buildBijectionArena`/`findNodeByID` (T2), `PaintChecked` signature (T5), `writePackageGo`/`moduleRoot` (T6). Each step says to confirm against neighboring tests.
- **Risk gates:** Component A is behavior-changing but zero-diff-safe (no corpus fixture uses flow-root) and gated by the promoted Chromium fixture. L5 is behavior-preserving. Both require the full matrix + zero-diff parity.

## Execution record (2026-11-12, completed)

All 8 tasks landed on `main` (no push). Commits: `21c0edf5` (T1 parse),
`2475d244` (T2 startsNewFC + key), `b552976d` (T3 fixture promotion), `1ab706f0`
(T5 findMark→BoxKey), `54598216` (T6 archtest fields), `2319a683` (T8 docs).
T4/T7 were verification-only (no commit).

**Deviations from the plan (all justified, behavior unchanged):**
- T1: `applyDisplay` routes only the main `ComputedStyle` cascade (`applyProperty`).
  The `@starting-style` site operates on `*StartingStyle` (a distinct type with no
  `FlowRoot`), so it keeps `parseDisplay`; that rare transition edge is residual.
  Added `TestFlowRootDoesNotInheritToChild` to lock non-inheritance (display is
  non-inherited; `inheritFromParent` copies neither `Display` nor `FlowRoot`).
- T2: the layout test uses a local `arenaObjectByID` walker instead of
  `findNodeByID(a, …)` — `findNodeByID` takes a `*dom.Node`, but
  `buildBijectionArena` returns only the arena. The key test lives in
  `internal/engine/style_diff_flow_root_test.go` (`package engine`), not
  `test/engine/` (uniformly `engine_test`), to reach `makeLayoutStyleKey`; it
  compares via `reflect.DeepEqual` (the key holds a non-comparable
  `DeferredLength`), matching the production seam at `style_diff.go:154`.
- T5: no new `find_identity_test.go` — `TestFindHighlightsPainted`
  (`test/engine/find_test.go:193`) already guards Find→SetFindActive→PaintChecked
  →ClearFind with fill/color assertions, so a baseline-green duplicate was skipped
  (DRY). `PaintChecked` takes a `scale float32` arg. The structural red is T6.
- T6: `writePackageGo` takes `map[string]string`, not `(name, body)`; the fixture
  test mirrors `pointer_map_test.go` and adds a renamed-import (`alias.go`) case,
  expecting 2 flagged fields (held + heldAlias), BoxKey/func-typed exempt.

**Verification:** full matrix green — `go build`, `gofmt -l` (0), `go vet -all`,
`go test ./...`, archtest, gate, qc all 0; `-race` on `internal/engine`,
`test/engine`, `internal/archtest` 0. `make parity` 105/105 (100%), average
96.39%, known gaps 4/6 — exact SP-3/SP-4 baseline (zero-diff). `layout-parity`
now 9 fixtures (added `float-containment-001`), all green at 1px. Mutations red→
restored→green: drop `FlowRoot` branch in `startsNewFC` (`#flowroot h=22 want 82`);
drop `FlowRoot` from key builder; re-add `findMark.obj` (archtest flags
`engine.go:2003`). One load-induced flake (`test/raster
TestUpdateLayerCoalescesDamage`) under the concurrent full suite confirmed green
in isolation ×5 and package ×3, and on a full-suite re-run — not a regression.
L3/L4 deferred to SP-6+ with terrain evidence (see roadmap/review).