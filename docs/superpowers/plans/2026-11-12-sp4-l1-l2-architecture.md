# SP-4 — L1 BoxKey identity + L2 formatting-context registry — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete `BoxKey` identity (index + generated-box keys + paint-by-key + pointer-lifetime rule) and make the display-dispatch chain an explicit, instrumented formatting-context pass registry — behavior-preserving with zero pixel diff.

**Architecture:** Refactor on existing seams. `BoxKey` already exists but is unread; generated boxes are unkeyed; two paint maps hold `*layout.Object`. L1 adds a `byKey` index, keys word/anonymous boxes with per-arena monotonic ordinals, and migrates the paint maps to `BoxKey`. L2 wraps the existing free-function passes in a registry that dispatches in today's exact order, adds per-FC counters, then makes FC roots authoritative for float/margin containment.

**Tech Stack:** Go 1.26; `internal/layout`, `internal/paint`, `internal/engine`, `internal/archtest`; Playwright/Chromium anchors; 105-fixture parity corpus + committed goldens.

**Spec:** `docs/superpowers/specs/2026-11-12-sp4-l1-l2-architecture-design.md`. Read it first. Every file:line below was verified at HEAD `3ee61413`; **read the real source before each edit and match actual signatures, not illustrative snippets.**

**Standing rules:** gofmt `-w` before every commit. NEVER pipe test output through `tail` (masks exit codes) — redirect to `/tmp/*.log` and read the log. Revert mutations with `git checkout` ONLY after pending work is committed. One TodoWrite item in_progress. Commits on `main` authorized; **NO push**. Zero-diff gate after every behavior-preserving landing: `make parity` 105/105 ≥96.39% (gaps 4 fixtures/6 bands), goldens byte-identical, gate counters unchanged.

---

## File structure

**L1:**
- `internal/layout/boxkey.go` — add `BoxKindWord`.
- `internal/layout/arena.go` — `byKey` index, `wordSeq`/`anonSeq` counters, `setKey`/`ForKey`/`anonNodeID` helpers; migrate 4 key-assignment sites; correct `Alloc` + package doc comments.
- `internal/layout/inline.go` — key word boxes at the `Alloc` site (~724).
- `internal/layout/anonymous.go` — key anonymous boxes in `spliceAnonymousBlock` (~138).
- `internal/paint/selection.go` — `SelSpan.Obj` → `SelSpan.Key`.
- `internal/paint/find.go` — `FindMark.Obj` → `FindMark.Key`.
- `internal/paint/builder.go` — maps + `SetSelection`/`SetFind` keyed by `BoxKey`.
- `internal/paint/builder_text.go` — lookups by `obj.Key`.
- `internal/engine/selection.go` — producers set `.Key`.
- `internal/engine/paint.go` — find producer sets `.Key` (nil guard).
- `internal/archtest/` — new no-`map[*layout.Object]` rule.
- `test/layout/boxkey_bijection_test.go` — new bijection oracle.

**L2:**
- `internal/layout/fc.go` — new: `FCKind`, `layoutPassFunc`, `fcRegistry`, `layoutFlex`, `lookupPass`.
- `internal/layout/block.go` — dispatch replacement; `PassCounts` increment; (second landing) `establishesFC` + authoritative containment.
- `internal/layout/arena.go` — `PassCounts [5]uint64` + `FCCount`.
- `internal/engine/engine.go` — `Session.LayoutPassCounts()` accessor.
- `test/layout/fc_registry_test.go` — new per-FC counter oracle.
- `scripts/verify-fc-containment-geometry.js` — new Chromium anchor (second landing).
- `testdata/render/edge-cases/08-nested-float-containment.html` — new fixture (second landing; confirm next free slot).

---

## Task 1: byKey index + setKey/ForKey + BoxKindWord

**Files:**
- Modify: `internal/layout/boxkey.go` (enum), `internal/layout/arena.go:254-321,430,509,677,699`
- Test: `test/layout/boxkey_bijection_test.go` (create)

- [ ] **Step 1: Write the failing bijection test (element/pseudo boxes only)**

Create `test/layout/boxkey_bijection_test.go`. Read `test/layout/boxkey_test.go` first for the existing harness/helpers (how it builds a session/arena). The test builds an arena from a small doc with elements + a `::before`, runs the passes, and asserts the index is a bijection over keyed boxes:

```go
package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/layout"
	// match the imports/helpers boxkey_test.go already uses to build a doc+styles
)

// keyedBoxes walks the arena and returns every object that carries a non-zero Key.
func keyedBoxes(a *layout.Arena) []layout.ObjectID {
	var ids []layout.ObjectID
	for i := 1; i < len(a.Objects); i++ {
		if a.Objects[i].Key != 0 {
			ids = append(ids, layout.ObjectID(i))
		}
	}
	return ids
}

func TestBoxKeyBijectionElementsAndPseudo(t *testing.T) {
	a := buildBijectionArena(t, `<div id="x"><p>hello</p></div>`, `#x::before{content:"B"}`)
	ids := keyedBoxes(a)
	if len(ids) == 0 {
		t.Fatal("no keyed boxes")
	}
	seen := map[layout.BoxKey]layout.ObjectID{}
	for _, id := range ids {
		k := a.Get(id).Key
		if prev, dup := seen[k]; dup {
			t.Fatalf("key %v shared by objects %d and %d", k, prev, id)
		}
		seen[k] = id
		got, ok := a.ForKey(k)
		if !ok || got != id {
			t.Fatalf("ForKey(%v) = (%d,%v), want (%d,true)", k, got, ok, id)
		}
	}
}
```

Add a `buildBijectionArena(t, html, css)` helper in the same file that parses html+css, resolves styles, and runs `layout.Build` + `Block` + `Inline` + `Positioning` (mirror how `boxkey_test.go` or `test/engine` builds a laid-out arena; read them first). It must return the laid-out `*layout.Arena`.

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./test/layout -run TestBoxKeyBijectionElementsAndPseudo -count=1 > /tmp/sp4-t1.log 2>&1; echo $?; cat /tmp/sp4-t1.log`
Expected: FAIL — `a.ForKey undefined` (compile error) because the index/helper do not exist yet.

- [ ] **Step 3: Add BoxKindWord, the index, and helpers**

In `internal/layout/boxkey.go`, append to the `BoxKind` const block (after `BoxKindPseudoFirstLetter`):
```go
	BoxKindWord
```

In `internal/layout/arena.go`, add to the `Arena` struct (after `byNode`):
```go
	byKey   map[BoxKey]ObjectID
	wordSeq uint16
	anonSeq uint16
```
In `NewArena`, initialize `byKey: make(map[BoxKey]ObjectID)`.
Add helpers (near `ForNode`):
```go
// setKey assigns a box's document-lifetime key and indexes it. The key must be
// unique within this arena generation; callers pass a non-zero key.
func (a *Arena) setKey(id ObjectID, k BoxKey) {
	a.Objects[id].Key = k
	if a.byKey == nil {
		a.byKey = make(map[BoxKey]ObjectID)
	}
	a.byKey[k] = id
}

// ForKey returns the object indexed for k, if any.
func (a *Arena) ForKey(k BoxKey) (ObjectID, bool) {
	id, ok := a.byKey[k]
	return id, ok
}
```

- [ ] **Step 4: Migrate the four existing key-assignment sites to setKey**

Read each site first, then replace the direct `.Key =` write with `a.setKey(id, …)`:
- `arena.go:430` root: `a.setKey(rootID, NewBoxKey(uint32(doc.Node.ID), BoxKindBlock, 0))`
- `arena.go:509` element: `a.setKey(id, NewBoxKey(uint32(n.ID), boxKindForStyle(obj.Style), 0))`
- `arena.go:677` pseudo: `a.setKey(id, NewBoxKey(uint32(nodeID), kind, 0))` (match the actual local var names)
- `arena.go:699` first-letter: `a.setKey(id, NewBoxKey(uint32(nodeID), BoxKindPseudoFirstLetter, 0))` (match actual locals)

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./test/layout -run TestBoxKeyBijection -count=1 > /tmp/sp4-t1.log 2>&1; echo $?; cat /tmp/sp4-t1.log`
Expected: PASS.

- [ ] **Step 6: Zero-diff sanity + commit**

Run: `go build ./... && go test ./internal/layout/ ./test/layout/ ./internal/engine/ -count=1 > /tmp/sp4-t1b.log 2>&1; echo $?; cat /tmp/sp4-t1b.log`
Expected: 0. (Full parity gate is Task 7; this is a fast regression check.)
```bash
gofmt -w internal/layout/boxkey.go internal/layout/arena.go test/layout/boxkey_bijection_test.go
git add internal/layout/boxkey.go internal/layout/arena.go test/layout/boxkey_bijection_test.go
git commit -m "feat(layout): index BoxKey and add ForKey/setKey (SP-4 L1)"
```

---

## Task 2: Key word/text-run boxes

**Files:**
- Modify: `internal/layout/inline.go:724-736`, `internal/layout/arena.go` (wordSeq already added)
- Test: `test/layout/boxkey_bijection_test.go` (extend)

- [ ] **Step 1: Extend the bijection test to assert word boxes are keyed**

Add a text-heavy case and assert every word box has a non-zero key. Read how word boxes are identified (they carry `Node != nil && Node.Text()` and `flagInlineLaidOut` context; the simplest robust check: after layout, every object with `Node != nil && !Node.Element()` that has a `DataContent` word is keyed). Add:
```go
func TestBoxKeyBijectionTextHeavy(t *testing.T) {
	a := buildBijectionArena(t, `<p>`+longText+`</p>`, ``) // longText: ~200 words, enough to wrap many lines
	ids := keyedBoxes(a)
	seen := map[layout.BoxKey]layout.ObjectID{}
	for _, id := range ids {
		k := a.Get(id).Key
		if prev, dup := seen[k]; dup {
			t.Fatalf("key %v shared by %d and %d", k, prev, id)
		}
		seen[k] = id
		if got, ok := a.ForKey(k); !ok || got != id {
			t.Fatalf("ForKey(%v)=(%d,%v) want (%d,true)", k, got, ok, id)
		}
	}
	// At least one word box must exist and be keyed with BoxKindWord.
	words := 0
	for i := 1; i < len(a.Objects); i++ {
		if a.Objects[i].Key.Kind() == layout.BoxKindWord {
			words++
		}
	}
	if words == 0 {
		t.Fatal("no BoxKindWord boxes after laying out text")
	}
}
```
Define `longText` as a ~200-word string constant in the test file.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./test/layout -run TestBoxKeyBijectionTextHeavy -count=1 > /tmp/sp4-t2.log 2>&1; echo $?; cat /tmp/sp4-t2.log`
Expected: FAIL — `no BoxKindWord boxes` (word boxes are unkeyed today).

- [ ] **Step 3: Key word boxes at the Alloc site**

In `internal/layout/inline.go`, read the word loop (~688-748). Immediately after `wordID, wordObj := a.Alloc()` and after `wordObj.Node = &nodeCopy` (so the originating text node ID is known), add:
```go
if a.wordSeq < ^uint16(0) {
	a.setKey(wordID, NewBoxKey(uint32(k.Node.ID), BoxKindWord, a.wordSeq))
	a.wordSeq++
}
```
`k.Node.ID` is the originating text node (the `nodeCopy` preserves `ID`). Use the actual receiver/local names from the source (`k` is the text object; confirm `k.Node` is non-nil here — it is, this is the `k.Node.Text()` branch). The overflow guard leaves an extreme box unkeyed rather than wrapping into a collision.

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./test/layout -run TestBoxKeyBijection -count=1 > /tmp/sp4-t2.log 2>&1; echo $?; cat /tmp/sp4-t2.log`
Expected: PASS (both element/pseudo and text-heavy).

- [ ] **Step 5: Regression + commit**

Run: `go test ./internal/layout/ ./test/layout/ ./test/engine/ ./internal/engine/ -count=1 > /tmp/sp4-t2b.log 2>&1; echo $?; cat /tmp/sp4-t2b.log`
Expected: 0.
```bash
gofmt -w internal/layout/inline.go test/layout/boxkey_bijection_test.go
git add internal/layout/inline.go test/layout/boxkey_bijection_test.go
git commit -m "feat(layout): key word boxes with a per-generation ordinal (SP-4 L1)"
```

---

## Task 3: Key anonymous + anonymous-table boxes

**Files:**
- Modify: `internal/layout/anonymous.go:134-164`, `internal/layout/arena.go` (anonNodeID helper)
- Test: `test/layout/boxkey_bijection_test.go` (extend)

- [ ] **Step 1: Extend the bijection test for anonymous/table boxes**

Add a fixture with mixed inline/block content (forces anonymous block wrappers) and a table with implicit rows/cells:
```go
func TestBoxKeyBijectionAnonymousAndTable(t *testing.T) {
	a := buildBijectionArena(t,
		`<div>text<span>inline</span>more<p>block</p></div>`+
			`<table><td>a</td><td>b</td></table>`, ``)
	ids := keyedBoxes(a)
	seen := map[layout.BoxKey]layout.ObjectID{}
	for _, id := range ids {
		k := a.Get(id).Key
		if prev, dup := seen[k]; dup {
			t.Fatalf("key %v shared by %d and %d", k, prev, id)
		}
		seen[k] = id
		if got, ok := a.ForKey(k); !ok || got != id {
			t.Fatalf("ForKey(%v)=(%d,%v) want (%d,true)", k, got, ok, id)
		}
	}
	anon := 0
	for i := 1; i < len(a.Objects); i++ {
		if a.Objects[i].Key.Kind() == layout.BoxKindAnonymous {
			anon++
		}
	}
	if anon == 0 {
		t.Fatal("no BoxKindAnonymous boxes after laying out mixed content + table")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./test/layout -run TestBoxKeyBijectionAnonymousAndTable -count=1 > /tmp/sp4-t3.log 2>&1; echo $?; cat /tmp/sp4-t3.log`
Expected: FAIL — `no BoxKindAnonymous boxes`.

- [ ] **Step 3: Add anonNodeID and key the splice**

In `internal/layout/arena.go`, add:
```go
// anonNodeID returns the DOM node ID of the nearest keyed ancestor of id, or 0.
// Anonymous boxes borrow it for their key's node field; uniqueness comes from
// the per-generation anonSeq ordinal, not from this node ID.
func (a *Arena) anonNodeID(id ObjectID) uint32 {
	for cur := id; cur != 0; cur = a.Objects[cur].Parent {
		if n := a.Objects[cur].Node; n != nil && n.ID != 0 {
			return uint32(n.ID)
		}
	}
	return 0
}
```
In `internal/layout/anonymous.go` `spliceAnonymousBlock`, after `id, _ := a.Alloc()` and the `*a.Get(id) = Object{…}` assignment, add:
```go
if a.anonSeq < ^uint16(0) {
	a.setKey(id, NewBoxKey(a.anonNodeID(parent), BoxKindAnonymous, a.anonSeq))
	a.anonSeq++
}
```
This covers anonymous blocks AND anonymous table rows/cells (both go through `spliceAnonymousBlock`, per `table.go:336,362`).

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./test/layout -run TestBoxKeyBijection -count=1 > /tmp/sp4-t3.log 2>&1; echo $?; cat /tmp/sp4-t3.log`
Expected: PASS (all three bijection tests).

- [ ] **Step 5: Regression + commit**

Run: `go test ./internal/layout/ ./test/layout/ ./test/engine/ ./internal/engine/ -count=1 > /tmp/sp4-t3b.log 2>&1; echo $?; cat /tmp/sp4-t3b.log`
Expected: 0.
```bash
gofmt -w internal/layout/anonymous.go internal/layout/arena.go test/layout/boxkey_bijection_test.go
git add internal/layout/anonymous.go internal/layout/arena.go test/layout/boxkey_bijection_test.go
git commit -m "feat(layout): key anonymous and table boxes by generation ordinal (SP-4 L1)"
```

---

## Task 4: Migrate paint selection map to BoxKey

**Files:**
- Modify: `internal/paint/selection.go:9-16`, `internal/paint/builder.go:19,67-75`, `internal/paint/builder_text.go:40`, `internal/engine/selection.go:564,590`
- Test: existing selection tests (read `test/engine` + `test/paint` for selection/highlight tests)

- [ ] **Step 1: Confirm the existing selection oracle is green (baseline)**

Run: `go test ./test/paint ./test/engine ./internal/engine -run 'Select|Selection|IME|Marked' -count=1 > /tmp/sp4-t4base.log 2>&1; echo $?; cat /tmp/sp4-t4base.log`
Expected: PASS. Note the test names — they are the regression oracle for this task. If none match, grep `test/` for `SetSelection`/`SelSpan`/`paintSelectionHighlight` to find the real oracle and run it.

- [ ] **Step 2: Change SelSpan.Obj → SelSpan.Key**

In `internal/paint/selection.go`:
```go
// SelSpan marks the rune range [Start, End) of one word box as selected.
// Key names the same box the builder's arena walk visits, so paint matches by
// document-lifetime identity instead of holding an arena pointer.
type SelSpan struct {
	Key   layout.BoxKey
	Start int
	End   int
}
```
(`layout` is already imported.)

- [ ] **Step 3: Change the builder map + setter + consumer**

`internal/paint/builder.go:19`: `selection map[layout.BoxKey]SelSpan`.
`SetSelection` (~70-75):
```go
func (b *Builder) SetSelection(spans []SelSpan) {
	b.selection = make(map[layout.BoxKey]SelSpan, len(spans))
	for _, sp := range spans {
		b.selection[sp.Key] = sp
	}
}
```
Update the `SetSelection` doc comment to say identity is by `BoxKey`, not pointer.
`internal/paint/builder_text.go:40`: `if sp, ok := b.selection[obj.Key]; ok {`.

- [ ] **Step 4: Change the engine producers**

`internal/engine/selection.go:564`: `spans = append(spans, paint.SelSpan{Key: words[i].Key, Start: start, End: end})`.
`internal/engine/selection.go:590`: `return []paint.SelSpan{{Key: markedObj.Key, Start: start, End: end}}`.
(`words[i]` and `markedObj` are `*layout.Object`; `.Key` is populated by Tasks 2-3.)

- [ ] **Step 5: Build + run the selection oracle**

Run: `go build ./... > /tmp/sp4-t4build.log 2>&1; echo build=$?; cat /tmp/sp4-t4build.log`
Expected: build=0 (no remaining `sp.Obj`/`SelSpan{Obj:` references — grep to confirm: `grep -rn "SelSpan{Obj\|\.Obj\b" internal/paint/selection.go internal/engine/selection.go` should show none for SelSpan).
Run the Step-1 oracle again: `go test ./test/paint ./test/engine ./internal/engine -run 'Select|Selection|IME|Marked' -count=1 > /tmp/sp4-t4.log 2>&1; echo $?; cat /tmp/sp4-t4.log`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
gofmt -w internal/paint/selection.go internal/paint/builder.go internal/paint/builder_text.go internal/engine/selection.go
git add internal/paint/selection.go internal/paint/builder.go internal/paint/builder_text.go internal/engine/selection.go
git commit -m "refactor(paint): key selection spans by BoxKey, not arena pointer (SP-4 L1)"
```

---

## Task 5: Migrate paint find map to BoxKey

**Files:**
- Modify: `internal/paint/find.go:8-15`, `internal/paint/builder.go:20,77-85`, `internal/paint/builder_text.go:37`, `internal/engine/paint.go:77-87`
- Test: existing find-in-page tests

- [ ] **Step 1: Confirm the find oracle is green (baseline)**

Run: `go test ./test/paint ./test/engine ./internal/engine ./cmd/goosie -run 'Find|find' -count=1 > /tmp/sp4-t5base.log 2>&1; echo $?; cat /tmp/sp4-t5base.log`
Expected: PASS. If none match, grep `test/` for `SetFind`/`FindMark`/`findMarks` to locate the oracle.

- [ ] **Step 2: Change FindMark.Obj → FindMark.Key**

`internal/paint/find.go`:
```go
// FindMark is one find-in-page highlight rectangle in document CSS pixels.
// Key names the text box whose painting emits the fill: emitting it there puts
// the highlight underneath that run's glyphs, before they are painted.
type FindMark struct {
	Key    layout.BoxKey
	Rect   frame.RectF
	Active bool
}
```

- [ ] **Step 3: Change the builder map + setter + consumer**

`internal/paint/builder.go:20`: `find map[layout.BoxKey][]FindMark`.
`SetFind` (~80-85):
```go
func (b *Builder) SetFind(marks []FindMark) {
	b.find = make(map[layout.BoxKey][]FindMark, len(marks))
	for _, m := range marks {
		b.find[m.Key] = append(b.find[m.Key], m)
	}
}
```
Update the `SetFind` doc comment (identity by `BoxKey`).
`internal/paint/builder_text.go:37`: `if marks := b.find[obj.Key]; len(marks) > 0 {`.

- [ ] **Step 4: Change the engine find producer (with nil guard)**

`internal/engine/paint.go:77-87`: `m.obj` is `*layout.Object` and may be nil for a mark whose box was not resolved. Today a nil `Obj` keys nil and is silently never matched; after migration `m.obj.Key` would panic. Skip nil boxes:
```go
if len(s.findMarks) > 0 {
	marks := make([]paint.FindMark, 0, len(s.findMarks))
	for _, m := range s.findMarks {
		if m.obj == nil {
			continue
		}
		marks = append(marks, paint.FindMark{
			Key:    m.obj.Key,
			Rect:   frame.RectF4(m.rect.X0, m.rect.Y0, m.rect.X1, m.rect.Y1),
			Active: m.match == s.findActive,
		})
	}
	b.SetFind(marks)
}
```
Read the real `findMark` struct first to confirm field names (`obj`, `rect`, `match`).

- [ ] **Step 5: Build + run the find oracle**

Run: `go build ./... > /tmp/sp4-t5build.log 2>&1; echo build=$?; cat /tmp/sp4-t5build.log` → build=0.
Run the Step-1 oracle again → PASS.
Confirm no production pointer map remains: `grep -rn "map\[\*layout.Object\]" internal/ cmd/` → expected **no matches**.

- [ ] **Step 6: Commit**

```bash
gofmt -w internal/paint/find.go internal/paint/builder.go internal/paint/builder_text.go internal/engine/paint.go
git add internal/paint/find.go internal/paint/builder.go internal/paint/builder_text.go internal/engine/paint.go
git commit -m "refactor(paint): key find marks by BoxKey, not arena pointer (SP-4 L1)"
```

---

## Task 6: Pointer-lifetime rule + archtest no-pointer-map guard

**Files:**
- Modify: `internal/layout/arena.go:8-12,301-303` (comments), `internal/archtest/` (new rule)
- Test: `internal/archtest` (the rule itself)

- [ ] **Step 1: Correct the false Alloc comment**

`internal/layout/arena.go:301-303` — replace the stability claim with:
```go
// Alloc reserves a new ObjectID and returns a pointer to the object. The pointer
// is valid ONLY until the next call that can allocate: growing the Objects slice
// reallocates the backing array and invalidates earlier pointers. Hold the
// ObjectID and re-fetch with Get at use; never keep a *Object across an Alloc.
```

- [ ] **Step 2: Correct the stale package doc**

`internal/layout/arena.go:8-12` — read it, then replace the "block, inline, float, flex, grid, table" order claim with the real one:
```go
// Layout runs as Block, then Inline, then Positioning. The Block pass dispatches
// each box to its formatting-context pass (block flow, flex, grid, table) by used
// display; float placement happens inside block flow. See fc.go for the registry.
```
(If `fc.go` does not exist yet at this task, reference `block.go` dispatch instead; Task 8 creates `fc.go`.)

- [ ] **Step 3: Add the archtest rule (write it to fail first via a mutation)**

Read `internal/archtest/rules.go` and the existing globals/cgo rule implementations to match the pattern (how rules scan packages and report). Add a rule that fails if any production package (not `test/`, not `third_party/`) declares a map keyed by `*layout.Object`. Illustrative shape — match the real archtest API:
```go
// noArenaPointerMaps forbids map[*layout.Object] in production: paint and engine
// must name boxes by BoxKey/ObjectID, never hold arena pointers (SP-4 L1).
```
Implement it using the same AST/scan helper the other rules use. The rule should inspect map type keys for `*layout.Object` (selector expr `layout.Object` under a star expr, as a map key).

- [ ] **Step 4: Verify the rule passes now, and red under a mutation**

Run: `go test ./internal/archtest/ -count=1 > /tmp/sp4-t6.log 2>&1; echo $?; cat /tmp/sp4-t6.log` → PASS (Tasks 4-5 removed both maps).
Mutation check: temporarily reintroduce `var _ = map[*layout.Object]int{}` in a production file (e.g. `internal/paint/builder.go`), run archtest → expect FAIL naming the new rule. Revert with `git checkout internal/paint/builder.go` (safe: Tasks 4-5 are committed). Re-run archtest → PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/layout/arena.go internal/archtest/
git add internal/layout/arena.go internal/archtest/
git commit -m "test(archtest): forbid arena-pointer maps; document the re-fetch rule (SP-4 L1)"
```

---

## Task 7: L1 full verification (zero-diff gate)

**Files:** none (verification only)

- [ ] **Step 1: Build + vet + fmt**

Run:
```bash
go build ./... > /tmp/sp4-l1-build.log 2>&1; echo build=$?
gofmt -l internal/layout internal/paint internal/engine internal/archtest test/layout > /tmp/sp4-l1-fmt.log 2>&1; cat /tmp/sp4-l1-fmt.log
go vet -all ./... > /tmp/sp4-l1-vet.log 2>&1; echo vet=$?
```
Expected: build=0, empty fmt list, vet=0.

- [ ] **Step 2: Full suite + race + archtest + gate + qc**

Run:
```bash
go test ./... -count=1 > /tmp/sp4-l1-all.log 2>&1; echo all=$?
go test -race ./internal/layout/ ./internal/paint/ ./internal/engine/ ./cmd/goosie/ ./test/engine/ ./test/gatecheck/ ./test/goldens/ ./internal/archtest/ -count=1 > /tmp/sp4-l1-race.log 2>&1; echo race=$?
go test ./internal/archtest/ -count=1 > /tmp/sp4-l1-arch.log 2>&1; echo arch=$?
go test ./test/gate -run TestGate -count=1 > /tmp/sp4-l1-gate.log 2>&1; echo gate=$?
go test ./test/qc -count=1 > /tmp/sp4-l1-qc.log 2>&1; echo qc=$?
```
Expected: all=0, race=0, arch=0, gate=0, qc=0. Read logs (never `tail`). Investigate any failure root-cause; do not skip. Gate counters must be unchanged from the SP-3 baseline.

- [ ] **Step 3: Zero-diff parity + goldens**

Run: `make parity > /tmp/sp4-l1-parity.log 2>&1; echo parity=$?; sed -n '112,119p' /tmp/sp4-l1-parity.log`
Expected: 105/105 (100%), average ≥96.39%, known gaps 4 fixtures/6 bands unchanged. Goldens are covered by `go test ./test/goldens/` in Step 2 (byte-identical). If Playwright/Chromium is unavailable, record that parity was not re-run and why — do NOT claim a pass. (They were available at SP-3; re-probe with `python3 -c "from playwright.sync_api import sync_playwright; p=sync_playwright().start(); b=p.chromium.launch(); b.close(); p.stop()"`.)

- [ ] **Step 4: Mutation evidence for the bijection oracle**

Temporarily break word keying (e.g. in `inline.go` Task-2 site, pass a constant ordinal `0` instead of `a.wordSeq`), run `go test ./test/layout -run TestBoxKeyBijectionTextHeavy -count=1` → expect FAIL (duplicate key). Revert with `git checkout internal/layout/inline.go` (safe: Task 2 committed). Re-run → PASS. Record the red message.

- [ ] **Step 5: Commit any verification fixups (else nothing to commit)**

If Steps 1-4 required fixes, commit them with `fix(layout): … (SP-4 L1)`. Otherwise the L1 work is already committed (Tasks 1-6). Confirm clean tree: `git status --short`.

---

## Task 8: L2 first landing — behavior-preserving pass registry + per-FC counters

**Files:**
- Create: `internal/layout/fc.go`
- Modify: `internal/layout/block.go:253-271` (dispatch), `internal/layout/arena.go` (PassCounts + FCCount)
- Test: `test/layout/fc_registry_test.go` (create)

- [ ] **Step 1: Write the failing per-FC counter test**

Create `test/layout/fc_registry_test.go`. It lays out one fixture per FC kind and asserts the matching counter incremented and the others did not. Read `test/layout/boxkey_bijection_test.go`'s `buildBijectionArena` helper and reuse it (or factor a shared `buildLaidOutArena(t, html, css)`):
```go
package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/layout"
)

func TestFCRegistryCountsPerKind(t *testing.T) {
	cases := []struct {
		name string
		html string
		kind layout.FCKind
	}{
		{"flex-row", `<div style="display:flex"><i>a</i><i>b</i></div>`, layout.FCFlexRow},
		{"flex-column", `<div style="display:flex;flex-direction:column"><i>a</i><i>b</i></div>`, layout.FCFlexColumn},
		{"grid", `<div style="display:grid"><i>a</i></div>`, layout.FCGrid},
		{"table", `<table><tr><td>a</td></tr></table>`, layout.FCTable},
		{"block", `<div><p>a</p></div>`, layout.FCBlockFlow},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := buildBijectionArena(t, c.html, ``)
			if got := a.FCCount(c.kind); got == 0 {
				t.Fatalf("%s: FCCount(%v)=0, want >0", c.name, c.kind)
			}
		})
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./test/layout -run TestFCRegistryCountsPerKind -count=1 > /tmp/sp4-t8.log 2>&1; echo $?; cat /tmp/sp4-t8.log`
Expected: FAIL — `layout.FCFlexRow undefined` / `a.FCCount undefined` (compile error).

- [ ] **Step 3: Create the registry**

Create `internal/layout/fc.go`:
```go
package layout

import "github.com/vyquocvu/goosie/internal/style"

// FCKind names a formatting-context pass. The block-flow pass is the fallback;
// the others are selected by used display value.
type FCKind int

const (
	FCBlockFlow FCKind = iota
	FCTable
	FCGrid
	FCFlexRow
	FCFlexColumn
)

// layoutPassFunc is the shared container-pass signature: it lays out the box id
// inside containingW and returns content height. layoutTable also writes W/H.
type layoutPassFunc func(a *Arena, id ObjectID, containingW float32) float32

// fcRegistry maps a used display value to its pass. Flex dispatches on direction
// inside layoutFlex, exactly as the former else-if chain did.
var fcRegistry = map[style.Display]layoutPassFunc{
	style.DisplayTable:       layoutTable,
	style.DisplayInlineTable: layoutTable,
	style.DisplayGrid:        layoutGrid,
	style.DisplayInlineGrid:  layoutGrid,
	style.DisplayFlex:        layoutFlex,
	style.DisplayInlineFlex:  layoutFlex,
}

func layoutFlex(a *Arena, id ObjectID, containingW float32) float32 {
	s := a.Get(id).Style
	if s != nil && (s.FlexDirection == style.FlexColumn || s.FlexDirection == style.FlexColumnReverse) {
		return layoutFlexColumn(a, id, containingW)
	}
	return layoutFlexRow(a, id, containingW)
}

// lookupPass returns the pass for a style, or false when the box is block flow
// (including a nil style, which falls through exactly as the else-if did).
func lookupPass(s *style.ComputedStyle) (layoutPassFunc, FCKind, bool) {
	if s == nil {
		return nil, FCBlockFlow, false
	}
	switch s.Display {
	case style.DisplayTable, style.DisplayInlineTable:
		return fcRegistry[s.Display], FCTable, true
	case style.DisplayGrid, style.DisplayInlineGrid:
		return fcRegistry[s.Display], FCGrid, true
	case style.DisplayFlex, style.DisplayInlineFlex:
		if s.FlexDirection == style.FlexColumn || s.FlexDirection == style.FlexColumnReverse {
			return fcRegistry[s.Display], FCFlexColumn, true
		}
		return fcRegistry[s.Display], FCFlexRow, true
	}
	return nil, FCBlockFlow, false
}
```
NOTE on the globals rule (verified): `internal/archtest` `MutableGlobals` flags only **assignments to a package-level var outside its initializer and outside `init()`**; its clean fixture explicitly accepts `var table = map[string]int{"seed": 1}`. `fcRegistry` is initialized once with a composite literal and only **read** (`fcRegistry[s.Display]`) in `lookupPass`, never written, so it is accepted. Do not add any `fcRegistry[k] = …` or `fcRegistry = …` write in a function body — that would trip the rule. Keep the map immutable after init.

- [ ] **Step 4: Add PassCounts + FCCount to the arena**

`internal/layout/arena.go` struct: add `PassCounts [5]uint64`. Add:
```go
// FCCount returns how many times the formatting-context pass k was dispatched
// during this arena generation's layout.
func (a *Arena) FCCount(k FCKind) uint64 { return a.PassCounts[k] }
```

- [ ] **Step 5: Replace the dispatch chain (behavior-preserving)**

Read `internal/layout/block.go:253-271` and the block-flow `else` body (272-611). Replace ONLY the `else if table / else if grid / else if flex` branches with a registry dispatch, keeping the `ContentVisibility=="hidden"` first branch and the block-flow `else` body byte-for-byte:
```go
if obj.Style != nil && obj.Style.ContentVisibility == "hidden" {
	// … unchanged W/H short-circuit …
} else if pass, kind, ok := lookupPass(obj.Style); ok {
	a.PassCounts[kind]++
	y = pass(a, id, containingW)
} else {
	a.PassCounts[FCBlockFlow]++
	// … unchanged block-flow child loop (272-611) …
}
```
Confirm `obj`, `id`, `containingW`, `y` are the actual locals in scope. The block-flow `else` must keep its exact body. The `nil`-style case falls to `else` (block flow) exactly as today.

- [ ] **Step 6: Run the counter test + regression**

Run: `go test ./test/layout -run TestFCRegistryCountsPerKind -count=1 > /tmp/sp4-t8.log 2>&1; echo $?; cat /tmp/sp4-t8.log` → PASS.
Run: `go test ./internal/layout/ ./test/layout/ ./test/engine/ ./internal/engine/ ./test/gate -count=1 > /tmp/sp4-t8b.log 2>&1; echo $?; cat /tmp/sp4-t8b.log` → 0.
Run: `go test ./internal/archtest/ -count=1 > /tmp/sp4-t8arch.log 2>&1; echo $?; cat /tmp/sp4-t8arch.log` → 0 (globals rule satisfied per Step 3 note).

- [ ] **Step 7: Zero-diff parity**

Run: `make parity > /tmp/sp4-t8parity.log 2>&1; echo parity=$?; sed -n '112,119p' /tmp/sp4-t8parity.log`
Expected: 105/105 ≥96.39%, gaps unchanged. Any nonzero diff falsifies behavior-preservation — investigate the dispatch order/return conventions before proceeding.

- [ ] **Step 8: Commit**

```bash
gofmt -w internal/layout/fc.go internal/layout/block.go internal/layout/arena.go test/layout/fc_registry_test.go
git add internal/layout/fc.go internal/layout/block.go internal/layout/arena.go test/layout/fc_registry_test.go
git commit -m "feat(layout): dispatch formatting contexts through a pass registry (SP-4 L2)"
```

---

## Task 9: Surface per-FC counters through the engine

**Files:**
- Modify: `internal/engine/engine.go` (accessor near `WorkCounts()` ~363)
- Test: `test/engine/fc_counts_test.go` (create) or extend an existing engine stats test

- [ ] **Step 1: Write the failing accessor test**

Read how `test/engine` builds a session and reads `WorkCounts()`. Add a test that lays out a flex fixture and asserts the engine exposes a non-zero flex-row pass count:
```go
func TestSessionLayoutPassCounts(t *testing.T) {
	s := newTestSession(t, `<div style="display:flex"><i>a</i></div>`) // match the real helper
	counts := s.LayoutPassCounts()
	if counts[layout.FCFlexRow] == 0 {
		t.Fatal("flex-row pass count = 0 after laying out a flex container")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./test/engine -run TestSessionLayoutPassCounts -count=1 > /tmp/sp4-t9.log 2>&1; echo $?; cat /tmp/sp4-t9.log`
Expected: FAIL — `s.LayoutPassCounts undefined`.

- [ ] **Step 3: Add the accessor**

In `internal/engine/engine.go`, near `WorkCounts()`:
```go
// LayoutPassCounts returns the per-formatting-context dispatch counts from the
// current arena generation. The array is indexed by layout.FCKind.
func (s *Session) LayoutPassCounts() [5]uint64 {
	if s.Arena == nil {
		return [5]uint64{}
	}
	return s.Arena.PassCounts
}
```
Read the real `Session.Arena` access pattern (locking) first — if `WorkCounts()` reads under a lock or via atomics, match that. `PassCounts` is written during Reflow on the session goroutine; if the accessor can be called cross-goroutine, guard it the way `Arena` access is already guarded. Confirm whether `s.Arena` reads elsewhere take `opMu`; match the established pattern.

- [ ] **Step 4: Run to verify it passes + regression**

Run: `go test ./test/engine -run TestSessionLayoutPassCounts -count=1 > /tmp/sp4-t9.log 2>&1; echo $?; cat /tmp/sp4-t9.log` → PASS.
Run: `go test ./internal/engine/ ./test/engine/ -count=1 > /tmp/sp4-t9b.log 2>&1; echo $?; cat /tmp/sp4-t9b.log` → 0.

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/engine/engine.go test/engine/fc_counts_test.go
git add internal/engine/engine.go test/engine/fc_counts_test.go
git commit -m "feat(engine): expose per-formatting-context layout pass counts (SP-4 L2)"
```

---

## Task 10: L2 first-landing full verification (zero-diff gate)

**Files:** none (verification only)

- [ ] **Step 1: Full matrix**

Run the same matrix as Task 7 Steps 1-2 (build/vet/fmt, full suite, race subset, archtest, gate, qc) into `/tmp/sp4-l2-*.log`. Expected: all 0; gate counters unchanged.

- [ ] **Step 2: Zero-diff parity + goldens**

Run: `make parity > /tmp/sp4-l2-parity.log 2>&1; echo parity=$?; sed -n '112,119p' /tmp/sp4-l2-parity.log` → 105/105 ≥96.39%, gaps unchanged. Goldens byte-identical (test/goldens in Step 1).

- [ ] **Step 3: Mutation evidence for the registry**

Temporarily change `lookupPass` to return `FCTable` for `DisplayFlex` (mis-dispatch), run `go test ./test/layout -run TestFCRegistryCountsPerKind -count=1` → expect FAIL (flex-row count 0 / table count wrong). Revert with `git checkout internal/layout/fc.go` (safe: Task 8 committed). Re-run → PASS. Record the red message.

- [ ] **Step 4: Confirm clean tree**

`git status --short` → clean (Tasks 8-9 committed). No push.

---

## Task 11: L2 second landing — measure Chromium nested float/margin containment

**Files:**
- Create: `scripts/verify-fc-containment-geometry.js`, `testdata/render/edge-cases/08-nested-float-containment.html` (confirm next free slot first)

- [ ] **Step 1: Confirm the fixture slot is free**

Run: `ls testdata/render/edge-cases/` and pick the next free `NN-` prefix. Read an existing anchor (e.g. `scripts/verify-shybreak-geometry.js`) for the Playwright/Chromium harness pattern, pinned browser, and exit-code contract.

- [ ] **Step 2: Author the nested float/margin fixture**

Create an HTML fixture that exercises FC containment the current local patch handles ad-hoc: a float inside a plain block that must be contained by the nearest FC root; a `flow-root`/`overflow:hidden` container that must grow around its floats; nested blocks where a later sibling's `clear` must move below floats merged from an earlier sibling; an inline-block and a flex item that each establish their own FC. Keep it deterministic (fixed widths, embedded font, no network).

- [ ] **Step 3: Write the Chromium anchor**

Create `scripts/verify-fc-containment-geometry.js` modeled on the existing geometry anchors: launch the pinned Chromium, load the fixture, measure the relevant box rects (container heights around floats, sibling clearance positions, FC-root boundaries) via `getBoundingClientRect`/`getComputedStyle`, and assert the measured values against expected geometry derived from the CSS spec. Print `FC CONTAINMENT ORACLE OK` and exit 0 on success; exit non-zero with the failing measurement otherwise. **Measure Chromium first and record the actual numbers** — do not hardcode guesses.

- [ ] **Step 4: Run the anchor against Chromium**

Run: `node scripts/verify-fc-containment-geometry.js > /tmp/sp4-t11.log 2>&1; echo $?; cat /tmp/sp4-t11.log`
Expected: exit 0, `FC CONTAINMENT ORACLE OK`. Record the measured geometry. If Chromium/Playwright is unavailable, record that and STOP this landing (defer to SP-5 per spec §3.2) — do not fabricate measurements.

- [ ] **Step 5: Measure Goosie's current behavior on the fixture**

Render the fixture with `./bin/goosie` (or `goosie-headless`) and compare to the Chromium numbers. Record where Goosie already matches and where it diverges. This is the baseline the second landing must improve without regressing the corpus.

- [ ] **Step 6: Commit the anchor + fixture (measurement only, no engine change yet)**

```bash
gofmt -w testdata/render/edge-cases/ 2>/dev/null || true
git add scripts/verify-fc-containment-geometry.js testdata/render/edge-cases/08-nested-float-containment.html
git commit -m "test(layout): anchor nested float/margin FC containment vs Chromium (SP-4 L2)"
```

---

## Task 12: L2 second landing — authoritative FC roots

**Files:**
- Modify: `internal/layout/block.go` (establishesFC + containment), `internal/layout/inline.go:84` (boundary)
- Test: the Task-11 anchor + existing float/clearance fixtures

**This landing changes behavior. It is gated on the Chromium anchor, not zero-diff. If it cannot match Chromium without regressing the corpus within budget, STOP and defer to SP-5 with evidence (spec §3.2); SP-4 ships L1 + L2-first.**

- [ ] **Step 1: Separate establishesFC from the margin predicate**

Read `startsNewFC` (`block.go:2893-2913`) and all 6 callers. Keep `startsNewFC` unchanged for the margin-collapse callers (`collapsesThroughTop/Bottom` at 2925/2935). Add a new `establishesFC(a, obj)` encoding the full CSS rule set: own `float`, `position:absolute/fixed`, `display:inline-block`, `display:table-cell`, flex/grid item (parent display), `overflow` not visible, `contain:paint/layout`, `flow-root`. Write a unit test (`test/layout/fc_establishes_test.go`) asserting the predicate on a table of shapes BEFORE wiring it into containment (red→green).

- [ ] **Step 2: Run the predicate test (red→green)**

Write the test first, run it red (function undefined), implement `establishesFC`, run green. Record both.

- [ ] **Step 3: Route containment through FC roots**

Read the local seed/merge patch (`block.go:545-599`) and the float-intrusion boundary (`inline.go:84`). Make the block pass descend into FC roots and merge their float extents using `establishesFC` as the boundary, replacing the ad-hoc hand-down/merge-up. Do this incrementally: keep the existing fixtures green at each sub-step. Remove the local patch only once the new path reproduces it.

- [ ] **Step 4: Run the anchor + the full float/clearance corpus**

Run: `node scripts/verify-fc-containment-geometry.js > /tmp/sp4-t12anchor.log 2>&1; echo $?; cat /tmp/sp4-t12anchor.log` → exit 0.
Run: `go test ./internal/layout/ ./test/layout/ ./test/engine -count=1 > /tmp/sp4-t12.log 2>&1; echo $?; cat /tmp/sp4-t12.log` → 0.
Run: `make parity > /tmp/sp4-t12parity.log 2>&1; echo parity=$?; sed -n '112,119p' /tmp/sp4-t12parity.log` → 105/105, no regression vs the SP-3 baseline (the new fixture may add a band; existing fixtures must not regress).

- [ ] **Step 5: Mutation evidence**

Mutate `establishesFC` (e.g. drop the `overflow` rule), run the anchor + the predicate test → expect FAIL. Revert (safe after committing). Record the red message.

- [ ] **Step 6: Commit (or record deferral)**

If green:
```bash
gofmt -w internal/layout/block.go internal/layout/inline.go test/layout/fc_establishes_test.go
git add internal/layout/block.go internal/layout/inline.go test/layout/fc_establishes_test.go
git commit -m "feat(layout): make formatting-context roots authoritative for containment (SP-4 L2)"
```
If deferred: do NOT commit a half-change. `git checkout` the engine files back to the Task-11 state, and record the deferral with the measured divergence in the SP-4 completion summary and the roadmap. SP-4 ships L1 + L2-first.

---

## Task 13: Full verification + catalog/roadmap sign-off

**Files:**
- Modify: `docs/roadmap-v2.md` (SP-4 status block), `docs/engine-compatibility-review.md` (L1/L2 status, correct stale snapshot refs)

- [ ] **Step 1: Full matrix**

Run:
```bash
go build ./... > /tmp/sp4-final-build.log 2>&1; echo build=$?
gofmt -l internal cmd test scripts > /tmp/sp4-final-fmt.log 2>&1; cat /tmp/sp4-final-fmt.log
go vet -all ./... > /tmp/sp4-final-vet.log 2>&1; echo vet=$?
go test ./... -count=1 > /tmp/sp4-final-all.log 2>&1; echo all=$?
go test -race ./internal/layout/ ./internal/paint/ ./internal/engine/ ./cmd/goosie/ ./test/engine/ ./test/gatecheck/ ./test/goldens/ ./internal/archtest/ -count=1 > /tmp/sp4-final-race.log 2>&1; echo race=$?
go test ./internal/archtest/ -count=1 > /tmp/sp4-final-arch.log 2>&1; echo arch=$?
go test ./test/gate -run TestGate -count=1 > /tmp/sp4-final-gate.log 2>&1; echo gate=$?
go test ./test/qc -count=1 > /tmp/sp4-final-qc.log 2>&1; echo qc=$?
make parity > /tmp/sp4-final-parity.log 2>&1; echo parity=$?
```
Expected: all 0; parity 105/105 ≥96.39%. Read every log. If the L2-second landing shipped, also run its anchor.

- [ ] **Step 2: Record the SP-4 status in the roadmap**

In `docs/roadmap-v2.md`, add an SP-4 status block (match the Vietnamese prose style and the "đã có trong mã / đã kiểm chứng" distinction used by the SP-3 block at ~181). Record: L1 complete (byKey index, word/anonymous keys, paint-by-key, pointer rule, archtest guard, bijection oracle); L2 first landing complete (registry + per-FC counters, zero diff); L2 second landing shipped OR deferred with the measured evidence. State the zero-diff parity result and that gate counters are unchanged. Do not claim native CUA or remote CI (no push).

- [ ] **Step 3: Correct the review's stale L1/L2 snapshot refs**

In `docs/engine-compatibility-review.md` §6 L1/L2 (and §4.1/4.2 where they cite shifted lines), add a short "Status (2026-11-12, SP-4)" note recording what landed and correcting the stale claims (BoxKey already existed; pseudo-bg maps keyed by style not Object; dispatch is 5-branch at block.go:253-271; startsNewFC at 2893). Keep it additive — do not rewrite the historical audit.

- [ ] **Step 4: Commit docs**

```bash
git add docs/roadmap-v2.md docs/engine-compatibility-review.md
git commit -m "docs(sp4): record L1 identity + L2 registry status and correct stale refs"
```

- [ ] **Step 5: Final history check**

Run: `git --no-pager log --oneline 3ee61413..HEAD && git status --short`
Expected: the SP-4 spec + task commits, clean tree. No push.

---

## Self-review notes

- **Spec coverage:** L1 byKey/index (spec §2.1) → T1; generated-box keys (§2.2) → T2,T3; paint-by-key (§2.3) → T4,T5; pointer rule + archtest (§2.4) → T6; L1 acceptance (§2.5) → T7. L2 registry (§3.1) → T8; counters surfaced (§3.1) → T9; L2-first acceptance (§3.3) → T10; FC-root anchor (§3.2/3.3) → T11; authoritative containment (§3.2) → T12; verification + docs (§4) → T13. Deferred L3/L4/L5 (§5) untouched.
- **Type consistency:** `BoxKindWord`, `byKey`, `wordSeq`, `anonSeq`, `setKey`, `ForKey`, `anonNodeID` (T1-T3) used consistently; `SelSpan.Key`/`FindMark.Key` replace `.Obj` in every named producer/consumer (T4,T5); `FCKind`/`FCBlockFlow..FCFlexColumn`/`layoutPassFunc`/`fcRegistry`/`layoutFlex`/`lookupPass`/`PassCounts`/`FCCount`/`LayoutPassCounts` (T8,T9) consistent; `establishesFC` distinct from `startsNewFC` (T12).
- **Known read-before-edit points:** archtest globals rule verified to accept `fcRegistry` (initialized once, only read — T8 Step 3 note); `Session.Arena` locking pattern for the accessor (T9 Step 3); `findMark` field names + nil guard (T5 Step 4); pseudo/first-letter key-site local names (T1 Step 4); the block-flow `else` body must stay byte-for-byte (T8 Step 5); fixture slot availability (T11 Step 1). Each step says to read the real source first.
- **Risk gates:** every behavior-preserving landing (T7, T10) requires zero-diff parity + goldens + unchanged gate counters. The behavior-changing landing (T12) is gated on a measured Chromium anchor and is explicitly deferrable to SP-5 with evidence, so SP-4 always ships at least L1 + L2-first.