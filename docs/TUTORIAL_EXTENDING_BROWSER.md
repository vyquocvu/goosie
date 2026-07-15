# Tutorial Series: Extending the Goosie Browser

This series walks through extending the Goosie engine, from adding a simple CSS property to building a complete browser feature. Each tutorial builds on the previous one.

---

## Tutorial 1: Adding a CSS Property

**Goal:** Add the `text-shadow` CSS property to the engine.

**Packages touched:** `internal/css`, `internal/renderer`

**Steps:**

1. **Classify the property.** `text-shadow` is a paint-affecting property — add it to `hotProperties` in `internal/css/properties.go`.
2. **Add typed fields.** Since shadows are inherited, add a `TextShadow` field to `InheritedStyle` in `internal/css/computed.go`. Add shadow parsing to `ApplyDeclaration()`.
3. **Update fingerprint and equality.** Add `TextShadow` to `Fingerprint()` and `Equal()` methods.
4. **Add display command support.** Define a `Shadow` struct and a new `DisplayCommandKind` in `internal/renderer/displaycmd.go`. The display list builder reads `ComputedStyle.TextShadow` and emits the command.
5. **Implement raster.** Handle the new command kind in `internal/renderer/frame/raster/cpu_backend.go` by drawing the shadow before the text.
6. **Test.** Add a golden test fixture with `text-shadow`. Verify output matches the reference rendering.

**See also:** `docs/CONTRIBUTING_CSS_PROPERTIES.md`

---

## Tutorial 2: Adding a DOM API

**Goal:** Implement `element.closest(selector)` for the DOM API.

**Packages touched:** `internal/dom`, `internal/js`

**Steps:**

1. **Core function.** Add `func (s *Store) Closest(node NodeID, selector string) (NodeID, error)` in `internal/dom/store.go`. Walk ancestors from `node` to root, matching each against the compiled selector.
2. **Test.** Add `TestClosest` in `internal/dom/store_test.go` covering match, no match, self-match, and stale node cases.
3. **JS binding.** Add `closest(call)` method on `NodeHandle` in `internal/js/dom_handle.go`. Register it on the prototype.
4. **Integration test.** Write a JS snippet that calls `element.closest()` and verify the result in a Goja runtime test.
5. **Document.** Add `closest` to `docs/SUPPORTED_WEB_PLATFORM.md`.

**See also:** `docs/CONTRIBUTING_DOM_APIS.md`

---

## Tutorial 3: Adding a JavaScript Capability Gate

**Goal:** Gate `navigator.clipboard.writeText()` behind a capability, denied by default.

**Packages touched:** `internal/js`

**Steps:**

1. **Define capability.** Add `CapabilityClipboard` to the capability enum in `internal/js/capability.go`.
2. **Set default.** Add `CapabilityClipboard` to `DefaultSecurePolicy` (denied).
3. **Implement stub.** If denied, the clipboard API exists but throws a `TypeError` with "permission denied". If allowed, call through to the system clipboard.
4. **Test.** Verify `navigator.clipboard.writeText("secret")` throws in the default policy. Verify it succeeds after `runtime.SetEnforcer(policy.Allow(CapabilityClipboard))`.
5. **Audit.** Add to `PermissionDecisions()` output so the developer tools show the denial.

**See also:** `internal/js/capability.go`, `internal/js/policy.go`

---

## Tutorial 4: Adding a Headless Rendering Command

**Goal:** Add a `goosie render file.html -o output.png` subcommand.

**Packages touched:** `cmd/headless`

**Steps:**

1. **Read the existing CLI.** `cmd/headless/main.go` already renders HTML to PNG. Extend it with `-format` (png, jpg) and `-quality` flags.
2. **Add `-wait` flag.** Some pages need JS to execute before rendering. Add a `-wait duration` flag that waits before capturing.
3. **Test.** Pipe HTML files and verify output dimensions match `-width`/`-height`.
4. **Document.** Add a `README.md` section for the CLI tool with examples.

---

## Tutorial 5: Adding a Golden Test Fixture

**Goal:** Add a golden rendering test for a `border-radius` fixture.

**Packages touched:** `internal/renderer/frame/golden`

**Steps:**

1. **Create fixture.** Add a function `GoldenBorderRadius() DisplayCommandList` that builds display commands for a rounded rectangle.
2. **Reference image.** Run `GOOSIE_UPDATE_GOLDEN=1 go test ./internal/renderer/frame/golden/` to generate the reference PNG.
3. **Review.** Check the generated image in `testdata/golden/` for correctness.
4. **Test.** Rename the reference to trigger a deliberate mismatch and verify the test fails.
5. **CI.** The golden workflow (`golden.yml`) will pick up the new test automatically.

---

## Tutorial 6: Profiling a Performance Regression

**Goal:** Find and fix a scroll performance regression using the benchmark suite.

**Packages touched:** `scripts/bench.sh`, `internal/engine/metrics`

**Steps:**

1. **Run baseline.** `scripts/bench.sh suite` to capture current performance.
2. **Reproduce regression.** Introduce a deliberate slowdown (e.g., add a `time.Sleep` in the tile lookup).
3. **Profile.** `scripts/bench.sh profile-cpu BenchmarkViewportScroll` to generate a CPU profile. Open with `go tool pprof`.
4. **Compare.** `scripts/bench.sh compare` uses `benchstat` to show the delta.
5. **Fix.** Remove the slowdown, re-run `scripts/bench.sh compare` to confirm recovery.
6. **Gate.** The CI performance workflow (`performance.yml`) will catch this regression in future PRs.

---

## Tutorial 7: Adding a Cache with Memory Evictor

**Goal:** Add a bounded cache that integrates with the memory budget manager.

**Packages touched:** `internal/memory`, `your-package`

**Steps:**

1. **Define cache.** Create a struct with LRU linked list (`container/list`), entry count, and byte budget.
2. **Implement evictor.** Add `func (c *Cache) Evict(targetBytes uint64) uint64` that evicts from the tail until the target is freed.
3. **Register.** In the cache constructor, call `memory.Manager.RegisterEvictor(memory.ComponentYourCache, c.Evict)`.
4. **Report usage.** After each mutation, call `memory.Manager.UpdateUsage(memory.ComponentYourCache, c.Bytes())`.
5. **Test.** Verify that filling the cache triggers eviction, that usage reports are accurate, and that the cache reaches steady state.

**See also:** `docs/MEMORY_MODEL.md`
