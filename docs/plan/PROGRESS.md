# Plan progress — Goosie

- Roadmap of record: [docs/roadmap-v2.md](../roadmap-v2.md). The superseded `docs/engine-compat-roadmap.md` was removed on 2026-11-18 (R-0); its E0–E8 vocabulary is not a second sequencing scheme.
- Scope source: roadmap-v2 P0–P3 plus the open-defect register [docs/qa-open-defects.json](../qa-open-defects.json) — 64 open records (4 P1, 45 P2, 15 P3). roadmap-v2 continues "theo bằng chứng", so each task below closes one measured defect rather than opening a new API list.
- Base revision: `ea26ab01` (main, clean, 15 commits ahead of `origin/main`). Work happens on `plan/<id>-<slug>` branches; nothing is pushed or merged to main.
- Verification mode: the gate is **not installed** (`.git/hooks` has no `goosie-verify`), so every result here is bootstrap evidence with `trusted: false`. Full checks need a disposable runner (`GOOSIE_VERIFICATION_DISPOSABLE_HOST=1`) and are not run on this personal host. Native macOS CUA stays BLOCKED — this session has no usable display.
- Baseline measured 2026-11-18 on `ea26ab01`: smoke bootstrap `check --suite smoke --bootstrap` exit 0 (`status: bootstrap-pass`, 5/5 journeys, screenshot inspected); 72 focused `-race` tests for the last 15 commits exit 0; `go test ./internal/archtest/` exit 0; `go vet -all ./...` exit 0; `go test ./test/gate -run TestGate` exit 0 (8 tests). `go test -race ./test/engine` is red — see the register.

## R-0 — Consolidate the roadmap and open this plan

- Dependencies: none.
- Done criteria: `docs/engine-compat-roadmap.md` is gone; no live citation points at it without a status note; this file exists and lists every task with an executable done criterion.
- Verification: `git ls-files docs/*roadmap*`, `grep -rn engine-compat-roadmap docs/`, `git status --short`. Docs-only, so no engine check applies.
- Risk: low.
- Status: done.

Evidence (2026-11-18): `git rm docs/engine-compat-roadmap.md`. `git ls-files docs | grep -i roadmap` now returns only `docs/roadmap-v2.md`. In `docs/engine-compatibility-review.md` the doc-set sentence at `:36` was repointed at `roadmap-v2.md`'s own verification-rules section, and the Section 9 subsection that reviewed the old file now carries a removal note with the recovery commit `48866387`; the remaining `:431/:606/:713` mentions are historical citations inside a table and a rule row, covered by that note. Pre-existing finding, not caused by this change and not fixed because roadmap goals/scope are out of my edit rights: `docs/roadmap-v2.md:9` links `roadmap-v2-history.md`, which does not exist in the tree. Roadmap goals and scope were not edited.

## R-1 — `font: inherit | initial | unset` on form controls

- Closes: `GOOSIE-FONTSHORTHANDWIDE-001` (P2).
- Dependencies: none. Establishes the Chromium-differential → unit → parity → smoke loop the later tasks reuse.
- Done criteria: in one document with a 16px sans-serif body, `font: inherit` on `input`/`select`/`button`/`textarea` computes 16px/sans-serif and `font: initial` computes 16px/Times, replacing today's 13.3333px/Arial; `unset` resolves per longhand (inherit on inherited, initial on non-inherited); the measured control boxes land on Chromium's field 174×24, select 49×22, button 37.34375×24, textarea 182×42 and `initial` field 168×24 within the anchor's tolerance; a CSS-wide keyword as the whole value of any other shorthand is rejected or applied to that shorthand's full longhand set, never half-applied.
- Verification: re-measure the table with a Playwright anchor script before coding; `go test ./test/style ./test/layout -run 'TestFontShorthand'` (new oracle, red before the fix and green after, plus a recorded mutation); `make parity` on the new fixture; smoke `check --suite smoke --bootstrap`; `go test ./...`, `go vet -all ./...`, `go test ./internal/archtest/`, gate suite.
- Risk: low–medium (one parser path in `internal/style`, no frame-path change).
- Status: proposed.

## R-2 — Paint `::placeholder` text-decoration and border

- Closes: residual (b) of `GOOSIE-PLACEHOLDER-001` (P2); residual (c) stays open as `GOOSIE-PSEUDO-002` incremental staleness.
- Dependencies: R-1 (shares the placeholder/pseudo cascade oracle and the parity loop).
- Done criteria: `input::placeholder{text-decoration:underline}` draws the underline on the placeholder text at Chromium's geometry while an unstyled placeholder keeps `rgb(117,117,117)` with no decoration; a `::placeholder` border draws its box; `getComputedStyle(el,'::placeholder')` keeps answering the pseudo's own style; a colour-only change restyles without a full layout rebuild.
- Verification: Playwright anchor comparing placeholder ink between closed and decorated states; `go test ./test/paint ./test/style -run 'TestPlaceholder'` (new oracle plus the two existing BR-321 tests unchanged); `make parity` with a new fixture and the scored-fixture floor bumped; smoke; full `go test ./...`, vet, archtest, gate.
- Risk: medium (touches `internal/paint` control path and the pseudo style handoff in `internal/layout/arena.go`).
- Status: proposed.

## R-3 — `getComputedStyle` pseudo `content` normalisation

- Closes: residual (i) of `GOOSIE-GCSPSEUDO-001` (P2); (ii) vendor-prefixed tokens, (iii) used-box geometry, (iv) `::part()`/`::slotted()` author rules and (v) declaration breadth stay registered.
- Dependencies: R-1 (same `internal/style/pseudo.go` read path and oracle family).
- Done criteria: `content:url(https://…)` answers the quoted form, `content:attr(data-x)` answers the evaluated attribute value quoted instead of the literal `attr(data-x)`, and `content:counter(c)` answers `counter(c)` instead of `undefined`, each matching the Chromium table measured for the same document; the spellings already pinned by BR-322 (`"PRE"`, `""`, `normal→none` for before/after only) do not move.
- Verification: extend `scripts/verify-br322-gcs-pseudo.js` and re-measure; `go test ./test/js ./test/engine -run 'TestGetComputedStyle'` (new oracle, red before / green after, plus a recorded mutation); smoke; `go test ./...`, vet, archtest, gate. Paint-visible only if a fixture is added, otherwise no parity change is claimed.
- Risk: medium (three layers: `internal/css` value spelling, `internal/style/pseudo.go`, `internal/engine/computed_style.go`).
- Status: proposed.

## Deferred and blocked — evidence, not silence

- `GOOSIE-RACEFENCE-001` (P2) — reproduced fresh on this host at `ea26ab01`: `go test -race ./test/engine -run '^TestDocumentLoadAllocationFence$'` → 411646 allocations over the 400000 ceiling, exit 1, no data race reported; the same test without `-race` → 399608, exit 0. Both fixes in the defect record edit the check that judges the change (`test/engine/load_bench_test.go:56-59`), so this needs your decision, not a self-approved threshold change. Consequence: no green `-race ./test/engine` claim is available for R-1..R-3.
- `GOOSIE-CIPARITY-001` and `GOOSIE-LICENSE-001` (P1) — both need a non-darwin host or measured-green CI. This session cannot produce that evidence; nothing here claims cross-platform provenance.
- `GOOSIE-LIVE-001` and `GOOSIE-SCROLLRANGE-001` (P1) — both are live-network findings (`testdata/live-parity.py`, `scripts/live-frame-stability.py`) with a large blast radius. They are real P1s and should be the next approved slice after R-1..R-3, sized separately once their reproduction is re-measured.
- L3/L4 incremental style/layout invalidation — roadmap-v2 defers it until a measurement basis exists (`docs/roadmap-v2.md:272`, terrain recorded in SP-5).
- `GOOSIE-PSEUDO-002` (P2) — the `::first-letter` admission rule exists in three copies and a gutted incremental copy leaves the whole suite green; maintainability, not user-visible, so it does not displace R-1..R-3.
- Native macOS CUA sign-off for every UI row — BLOCKED, no display in this session.
