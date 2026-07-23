# DevTools — Task Breakdown

Generated from ROADMAP_DEVTOOLS.md + deep code analysis.
All panels live in `internal/ui/devtools/` unless noted.

## Legend

| Status | Meaning |
|--------|---------|
| ✅ | Done (tested) |
| 🟡 | Exists but no tests / needs extraction |
| ⬜ | Not started |

---

## Phase 1 — Bolt-down: extract + TDD (high priority)

Extract embedded panels from `dock.go` into separate files with full test coverage.

| # | Tab | File | Provider | Status |
|---|-----|------|----------|--------|
| 1.1–1.2 | **Security** | `security_panel.go` + `security_panel_test.go` | `CurrentURL` / `SecuritySummary` | ⬜ |
| 1.3–1.4 | **Settings** | `settings_panel.go` + `settings_panel_test.go` | `settingsProvider` | ⬜ |
| 1.5–1.6 | **Display List** | `displaylist_panel.go` + `displaylist_panel_test.go` | `rendererProvider` | ⬜ |
| 1.7–1.8 | **Script Queue** | `scriptqueue_panel.go` + `scriptqueue_panel_test.go` | `*js.Runtime` | ⬜ |
| 1.9–1.10 | **Tile Cache** | `tilecache_panel.go` + `tilecache_panel_test.go` | `*memory.Manager` / `rendererProvider` | ⬜ |
| 1.11 | **Performance** | `performance_panel_test.go` (extend) | `metricsProvider` | ⬜ |

## Phase 2 — Provider cleanup (medium priority)

Replace concrete types with interfaces so every panel can be tested with mocks.

| # | What | Current | Target | Status |
|---|------|---------|--------|--------|
| 2.1 | `memoryProvider` interface | `*memory.Manager` → `interface{ Stats() }` | ✅ done |
| 2.2 | `jsRuntimeProvider` interface | `*js.Runtime` → `interface{ ActiveTimersCount() etc. }` | ✅ done |
| 2.3 | `mockRendererProvider` | missing → created in `displaylist_panel_test.go` | ✅ done |
| 2.4 | `mockSettingsProvider` | missing → created in `settings_panel_test.go` | ✅ done |
| 2.5 | `mockMemoryProvider` | not needed — tests can use real `*memory.Manager` | ⬜ |
| 2.6 | `mockJSRuntimeProvider` | not needed — tests can use real `*js.Runtime` | ⬜ |
| 2.7 | Fill `metricsAdapter` gaps | 3/11 fields populated | ⬜ |

> **2.7 note:** The engine's `metrics.Recorder` lives inside the session, not the request log.
> Plumbing it to the browser shell requires threading the session into `TabContext`
> construction — a separate enhancement.

## Phase 3 — Roadmap enhancements (medium priority)

Build UX features per ROADMAP_DEVTOOLS.md release mapping.

| # | Release | Tab | Enhancement | Status |
|---|---------|-----|-------------|--------|
| 3.1 | v0.18 M2 | Elements | CSS Inspector — matched rules sorted by specificity | ⬜ |
| 3.2 | v0.19 M3 | Network | Waterfall timing detail — phase bars per row | ⬜ |
| 3.3 | v0.20 M4 | Storage | Key-value edit — Add/Delete/Clear buttons | ⬜ |
| 3.4 | v0.20 M4 | Security | Certificate details + CSP directives panel | ⬜ |
| 3.5 | v0.21 M5 | Performance | Rolling FPS strip chart; live update timer | ⬜ |
| 3.6 | v0.22 M6 | *(new)* | Accessibility tree tab (ARIA roles, contrast, keyboard) | ⬜ |

---

## File Inventory

```
internal/ui/devtools/
├── dock.go                     # Dock, TabContext, all provider interfaces
├── panel.go                    # vestigial Panel interface (unused)
├── network_panel.go            # Network tab (✅ 30 tests)
├── memory_panel.go             # Memory tab (✅ 3 tests + 1 visual)
├── performance_panel.go        # Performance tab (✅ 6 tests)
├── sources_panel.go            # Sources tab (✅ 22 tests + 2 visual)
├── storage_panel.go            # Storage tab (✅ 6 tests)
├── security_panel.go           # Security tab — extracted from dock.go (✅ 6 tests)
├── settings_panel.go           # Settings tab — extracted from dock.go (✅ 5 tests)
├── displaylist_panel.go        # Display List tab — extracted from dock.go (✅ 5 tests)
├── scriptqueue_panel.go        # Script Queue tab — extracted from dock.go (✅ 3 tests)
├── tilecache_panel.go          # Tile Cache tab — extracted from dock.go (✅ 4 tests)
├── network_panel_test.go       # (30 tests)
├── memory_panel_test.go        # (3 tests + 1 visual)
├── performance_panel_test.go   # (6 tests)
├── sources_panel_test.go       # (22 tests)
├── sources_panel_visual_test.go# (2 visual tests)
├── storage_panel_test.go       # (6 tests)
├── security_panel_test.go      # (6 tests)
├── settings_panel_test.go      # (5 tests)
├── displaylist_panel_test.go   # (5 tests)
├── scriptqueue_panel_test.go   # (3 tests)
└── tilecache_panel_test.go     # (4 tests)
```
