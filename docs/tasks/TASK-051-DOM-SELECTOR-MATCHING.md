# TASK-051 — the DOM selector API answers three forms and silently misses everything else (GOOSIE-DOMQUERYSEL-001)

**Priority:** P1. **Method:** UNIT (per-form oracles in `test/js/` measured against Chromium) + INT (the fast-path predicate) + PARITY (live sweep delta; no new scored fixture unless the agent finds one that pins a real page idiom).
**Owner:** agent tasked with TASK-051. **Defect:** `GOOSIE-DOMQUERYSEL-001`.
**Base:** `main` after TASK-047 merges — **do not start before that**: both tasks edit `internal/js/js.go`, and TASK-047 adds files under `test/js/`.
**Language:** English, because every number has to be quotable verbatim into the defect registry.

## 0. Rules for this branch

- **Do not touch** `docs/qa-test-catalog.md`, `docs/qa-open-defects.json`, `docs/qa-ratchet.json`,
  `test/qc/catalog.go`, `test/qc/traceability.golden.json`, `testdata/render-known-gaps.json`,
  `testdata/parity.py`. Propose BR ids and exact test names in §8; the integrator tags and `-update`s.
- **This one task is allowed to edit `internal/archtest/rules.go`** — see §3.1 — and must justify the edit in
  the commit message, not in a comment. No other archtest rule may move.
- No `git push`, no merge, no PR. Every claim carries command, number, exit code, measured on your host.
  Correct anything in this doc that your measurement disproves, in place, and say what was wrong.

## 1. Reproduction, measured by the integrator on `main` at `a5c5424`

One document, fourteen selector forms, both engines, `document.querySelector` / `querySelectorAll` and an
element-scoped `querySelectorAll`:

| selector        | Chromium docQS / docQSA | Goosie docQS / docQSA |
|-----------------|-------------------------|-----------------------|
| `*`             | hit / 8                 | **null / 0**          |
| `div`           | hit / 1                 | hit / 1               |
| `p.a`           | hit / 1                 | **null / 0**          |
| `div#o`         | hit / 1                 | **null / 0**          |
| `[data-x]`      | hit / 1                 | **null / 0**          |
| `[data-x="y"]`  | hit / 1                 | **null / 0**          |
| `div > p`       | hit / 1                 | **null / 0**          |
| `.a, .c`        | hit / 2                 | **null / 0**          |
| `p:first-child` | hit / 2                 | **null / 0**          |
| `div span`      | hit / 1                 | **null / 0**          |
| `.a`            | hit / 1                 | hit / 1               |
| `p`             | hit / 3                 | hit / 3               |
| `span > p`      | hit / 1                 | **null / 0**          |

- Document: `<html><body><div id="o" data-x="y"><p class="a">1</p><span class="s"><p class="b">2</p></span></div><p class="c">3</p></body></html>`.
- Goosie side: a throwaway `package js_test` probe using `domtest.Parse` + `newRuntime(t, js.Options{URL, DOM})`
  + `r.Run(src, "inline")` + `r.Global("out")`, run twice on 2026-10-26 and then deleted. No exception was
  thrown by any form — the API returns a wrong count, it does not fail.
- Chromium side: `NODE_PATH=<repo>/node_modules node /tmp/t50/selchrom.js` (Playwright `setContent`, same DOM).
- Element-scoped results match the doc-level ones (`#o.querySelectorAll("p.a")` → Chromium 1, Goosie 0).
- The three forms that work are exactly `tag`, `.class`, `#id`.

## 2. Cause, two mechanisms stacked

### 2.1 The index fast path swallows anything lowercase

`internal/js/js.go:2147 querySelectorAllIndexed` answers from the document indexes when the selector starts
with `.` or when `isLowerTag(sel)` is true. `isLowerTag` (`js.go:2097`) only rejects strings that contain an
uppercase letter, so `"div > p"`, `"p.a"`, `"*"`, `"[data-x]"`, `"p:first-child"` all qualify as tag names,
`ElementsByTagName` finds nothing, the function returns `ok == true`, and the general walk at `:2139` is
never reached. A fast path must be able to *decline*; this one cannot.

### 2.2 The matcher underneath implements three forms

`matchSelector` (`js.go:2186`) — its own comment: *"Supported forms are tag name ("p"), ID ("#main"), and
class (".active")"* — is the only matcher, so fixing 2.1 alone converts silent zeros into silent zeros from
the walk. The cascade is not the problem: `internal/css/selector.go` already implements all of it
(`Selector.Matches:490`, `MatchesIndexed:503`, `matchAttr:674`, `matchPseudoClass:706`, `matchHas:1004`,
`matchNth:1154`, `matchLang:1199`), which is why the same 11 forms apply as CSS rules and work.

## 3. The fix

### 3.1 The import-table decision, made deliberately

`internal/archtest/rules.go:40-64` lets `internal/js` import `internal/dom` and `internal/media` only.
Add `internal/css` to that entry. Justification to state in the commit: `css` imports `dom` and nothing
else, so `js → css → dom` introduces no cycle and keeps `dom` the import-free leaf; the alternative —
re-implementing selector matching inside `js` — creates a second source of truth for the same semantics,
which is the defect class TASK-037 and TASK-049 were both created to remove. `internal/archtest` must stay
green without any other rule change, and `PROJECT.md`'s inventory line for `internal/js` may need one
clause — say whether you edited it.

### 3.2 Behaviour

- Route `document.querySelector`, `document.querySelectorAll`, `Element.querySelector(All)`,
  `Element.matches`, `Element.closest` (audit the whole list; if one of them is missing from
  `internal/js` today, report that as its own defect rather than adding it silently) through parsed
  `css.Selector` values and `Selector.Matches`/`MatchesIndexed`.
- Cache compiled selectors per document. `js.go:1085` already records that this path was hot enough to
  need the index fast path, so a naive parse-per-node walk is a regression, not a fix. Measure it: report
  allocations or wall time for a fixed number of queries before and after (the gate suite asserts counters,
  not durations — put the number in the task doc, not in a duration assertion).
- Keep the index fast path, but tighten its predicate so it can only answer a *single simple selector*
  (whole string is `tag` | `.class` | `#id`, with the existing case rules), and add a test that a complex
  selector provably reaches the general path.
- Specific semantics to pin with tests, because Chromium was measured on them:
  1. `*` counts every element under the root, doc-level and element-scoped (8 and 3 in §1).
  2. Compound forms without combinators (`p.a`, `div#o`) match both parts on one element.
  3. Attribute presence `[data-x]` and equality with a **quoted** value `[data-x="y"]`. Note the existing
     `test/js/element_query_test.go:42 TestElementQuerySelectorAttr` covers only the unquoted
     `[jsname=sIJmDf]` form, which is why the quoted form is broken today — keep that test passing and add
     the quoted one.
  4. Combinators `>`, ` `, `+`, `~`, and selector lists, with document order and de-duplicated results
     (a list must not report the same element twice).
  5. Pseudo-classes the cascade supports (`:first-child`, `:not(...)`, `:nth-child(...)`) — and, for any form
     the cascade itself does not support, that `querySelector` agrees with the cascade instead of diverging.
  6. Invalid selectors must throw `SyntaxError` (measure Chromium's `DOMException` name; the current
     silent-0 behaviour is wrong even where the answer would be empty).
  7. `matches`/`closest` semantics: `closest` walks up including the element itself and stops at the first
     match; `matches` never looks at descendants.
- Case rules: tag names ASCII-case-insensitive in HTML, `id`/attribute values case-sensitive — do not
  regress `isLowerTag`'s original purpose (uppercase-in-tag-name must not take the index path).
- Caps: no new limit; the selector string is already admitted input. State whether compiled-selector caching
  interacts with `internal/engine/limits.go:23-59` per-document caps and what you did about it.

## 4. Oracles

- A table-driven `test/js` case holding the fourteen §1 forms and asserting the exact Chromium numbers for
  all four entry points (doc `querySelector`, doc `querySelectorAll`, element-scoped, `matches`/`closest`).
  The oracle must be the measured count, never "did not throw".
- A case per §3.2 numbered item.
- A fast-path case proving a complex selector is declined by `querySelectorAllIndexed` and answered by the
  walk (white-box: an unexported-function test in `package js` is acceptable here; say which package you used).
- If you add a scored fixture, it must encode a real idiom (e.g. a boot script that reads
  `document.querySelectorAll('*')` and paints what it finds), and you report the corpus numbers instead of
  touching `testdata/parity.py`.

## 5. Mutations

Every case must die when its mechanism is restored. Minimum:
- M01 revert `matchSelector`'s call site to the three-form subset: the §1 table must die.
- M02 restore `isLowerTag` as the fast-path predicate while keeping the new matcher: complex selectors must
  die through the index path (this proves 2.1 is a separate bug, not a detail of 2.2).
- M03 strip quotes from attribute values before comparing **nowhere** (i.e. leave the quoted form
  uncompared): the `[data-x="y"]` case dies.
- M04 make `closest` start at the parent instead of the element: its case dies.
- M05 remove the compiled-selector cache: no test may go red — instead report the measured cost difference,
  so the cache's necessity is recorded, not asserted.

## 6. Out of scope

- CSS *parsing* of selectors used by the cascade: if you find a cascade bug while wiring this, report it as
  an id (`GOOSIE-CSSSELECT-001` already covers quoted values holding escaped quotes) rather than fixing it here.
- Shadow DOM scoping rules beyond what `internal/js` already exposes.
- Any live-site claim. Attribution happens in the sweep after merge (§7.4).

## 7. Acceptance

1. `go test -count=1 ./...`, `go test ./internal/archtest/`, `go vet -all ./...`, `gofmt -l` clean, with
   timings and exit codes.
2. All fourteen §1 forms match Chromium exactly in Goosie, for all four entry points.
3. Every mutation in §5 behaves as described; restore rc 0.
4. The integrator re-runs `make live-parity` before and after your merge; the verdict is recorded per
   `build.source_sha8`, and a category count that does not move is reported as "did not move", never as a
   win.
5. §8 proposes BR ids continuing from the catalog total you read in `docs/qa-test-catalog.md` (report that
   total), lists exact test names, and says which form(s) each case pins.
6. §9 states what you did not measure.
