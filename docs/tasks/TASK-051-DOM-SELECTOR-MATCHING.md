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

## 8. Hand-off: BR ids, tests, measurements, gates, mutations

### 8.0 Catalog total and the id range used here

`docs/qa-test-catalog.md:6` says `Tổng: **210 case**`. The file actually holds **212** entries
(`grep -c -- '^- \*\*BR-' docs/qa-test-catalog.md` → `212`), and the highest allocated id is **BR-212**
(`grep -o 'BR-[0-9]*' docs/qa-test-catalog.md | sort -t- -k2 -n | tail -1` → `BR-212`). The ids below
therefore continue from the *actual* maximum, not from the header, so nothing collides: **BR-213 … BR-228**.
This branch adds no `// qc:BR-` markers, because none of these ids is in the catalog yet; the integrator tags
and `-update`s.

### 8.1 Behaviour → exact test

Every `test/js` case runs one goja script through `runSelectorScript`, which fails the case if the script
throws; every expectation is the number measured on Chromium 153.0.8010.12 (§8.2), never "did not throw".

| BR | what it pins | exact test | file | §3.2 item |
|----|--------------|-----------|------|-----------|
| BR-213 | the fourteen §1 forms × 8 columns: doc `querySelector`, doc `querySelectorAll` (count and order), element-scoped `querySelector`/`querySelectorAll`, `matches` on `.a` and `.b`, `closest` from `.b` and `.c` | `TestSelectorMatchingSection1Forms` | `test/js/selector_matching_test.go` | 1, 2, 3, 4, 5, 7 |
| BR-214 | combinators `>`, descendant, `+`, `~`; `*` as a compound tail; selector lists; document order; de-duplication (`p,p,p` = three, not nine); `:is(p, .s)` | `TestSelectorMatchingCombinatorsAndLists` | same | 4 |
| BR-215 | attribute presence, double/single/unquoted equality, `i` flag, `^=`, `$=`, `*=`, `\|=`, `~=` with a quoted value, attribute-name folding | `TestSelectorMatchingAttributes` | same | 3 |
| BR-216 | `:first-child`, `:last-child`, `:only-child`, `:empty`, `:first-of-type`, `:last-of-type`, `:nth-child(An+B)`, `:nth-child(An+B of S)`, `:nth-last-child`, `:nth-last-of-type`, `:not(list)`, `:is()`, four `:has()` forms | `TestSelectorMatchingPseudoClasses` | same | 5 |
| BR-217 | `:scope` on Document and on Element, in `querySelector`, `querySelectorAll`, `matches`, `closest` | `TestSelectorMatchingScopeSemantics` | same | 7 |
| BR-218 | `matches` never looks at descendants; `closest` includes self, stops at the first match, walks ancestors; `typeof document.matches === "undefined"` | `TestSelectorMatchingMatchesAndClosest` | same | 7 |
| BR-219 | tag and attribute names fold; id, class and attribute values do not | `TestSelectorMatchingCaseRules` | same | case rules |
| BR-220 | 21 invalid selectors × 6 entry points (doc QS/QSA, element QS/QSA, `matches`, `closest`) all raise `SyntaxError` | `TestSelectorMatchingSyntaxErrors` | same | 6 |
| BR-221 | selectors Chromium accepts but that match nothing (`:is()`, `:where()`, `::before`, `p::before`, `p:empty`, `p:has(> figure)`) answer empty and do not throw | `TestSelectorMatchingValidButEmptySelectors` | same | 6 |
| BR-222 | `[a=""]` is valid, `[a=]` is not — validity only; the count is GOOSIE-CSSATTR-EMPTY-001 | `TestSelectorMatchingEmptyQuotedAttributeIsAccepted` | same | 3, 6 |
| BR-223 | the exact `DOMException` wording on five receivers (empty-selector wording vs quoted-back wording) | `TestSelectorMatchingSyntaxErrorMessages` | same | 6 |
| BR-224 | `String()` argument coercion (`undefined`, `null`, array, object with `toString`, number) and the missing-argument `TypeError` with Chromium's wording | `TestSelectorMatchingArgumentCoercion` | same | 6 |
| BR-225 | the tightened fast-path predicate: only a whole-string tag, `.class` or `#id` may use the index; uppercase tag names and every compound/complex form decline | `TestSelectorFastPathPredicate` | `internal/js/selector_index_test.go` (`package js`) | keep the index path |
| BR-226 | a complex selector is declined by *both* index functions and answered by the walk with the measured counts; a simple `p` still uses the index and agrees with the walk | `TestSelectorComplexSelectorDeclinesIndex` | same | keep the index path |
| BR-227 | the compiled plan is reused for the same string, differs for different strings, and the cache never exceeds 256 entries | `TestSelectorPlanCacheReusesAndBounds` | same | cache |
| BR-228 | the compound fast-path answer on the id-index fixture: doc-level `p.x` = 3 and `p` = 3, element-scoped `#inner p` = 1, `#inner #inner` = null; the old expectation for `p.x` was 0 | `TestJSQueryContainment` (expectation corrected in place) | `test/dom/id_index_test.go` | keep the index path |

`BenchmarkSelectorMatchingQueryAll` is a benchmark, not a case: no BR id, and no test asserts a duration —
the numbers are recorded in §8.4 instead. The pre-existing `test/js/element_query_test.go:42
TestElementQuerySelectorAttr` (unquoted `[jsname=sIJmDf]`) was left untouched and still passes in §8.3.

### 8.2 Oracle: commands, numbers, exit codes

Chromium side, every script run as
`NODE_PATH=/Users/vyquocvu/Develop/Browser/goosie/node_modules node /tmp/t51/measure*.js`; every script below
exited **rc 0**, `navigator.userAgent` = `HeadlessChrome/153.0.8010.12` (Playwright 1.63.0 from the repo's
`node_modules`), standards mode (`<!DOCTYPE html>`), fixture exactly the §1 document.

| script | what it measured | result |
|--------|------------------|--------|
| `measure.js` | the fourteen rows × `docQS`, `docQSA` count+order, `oQS`/`oQSA`, `matches` on `.a`/`.b`, `closest` from `.b`/`.c`; plus `:scope` on Document and Element | the table below |
| `measure8.js` | invalid set, `SyntaxError` messages, zero-argument `TypeError` wording, `:scope` semantics, `null`/`undefined`/array coercion | encoded in BR-220/223/224/217 |
| `measure9.js` | `[a=1]`, `[a=#1]`, `[.]`, `[#]`, `[1a]`, `#1a`, `.-1` invalid; `#-a`, `.-a`, `[a="v"i]` valid; `[a=b s]`, `[a=b S]` invalid; browser version | BR-220, BR-215 |
| `measure10.js` | combinators, attribute operators, `nth-*` forms, `:is`, `:not` lists | BR-214/215/216 |
| `measure12.js` | `p:nth-child(1 of .b)` = `p.b`; `p:nth-last-of-type(1)` = `p.a,p.b,p.c`; `body > :first-child` = `div#o`; `div > *` = `p.a,span.s`; `:is()`/`:where()` valid and empty | BR-216, BR-214, BR-221 |
| `measure13.js` | `p:nth-child(2)` = `p.c`; `p:nth-child(odd)` = `p.a,p.b`; `p:has(> figure)` valid and empty; `typeof document.matches`/`closest` = `"undefined"` | BR-213/216/218 |
| `measure14.js` | the `test/dom/id_index_test.go` fixture: `p.x` = 3, `p` = 3, `#inner p` = 1 | BR-228 |
| `probe_gaps.js` | `P:FIRST-CHILD` = `p.a,p.b`; `P:NOT(.a)` = `p.b,p.c`; `[data-x=""]` = 0; `[data-x="y" x]` = `SyntaxError`; `*\|p` = `p.a,p.b,p.c`; `svg\|p` = `SyntaxError` | §8.6, §9 |
| `probe_gaps3.js` | `typeof textNode.querySelectorAll` = `"undefined"`; `#a\:b` = 1; `.\78` = 1 | §9 |

The fourteen rows as encoded by `TestSelectorMatchingSection1Forms` (field order:
`docQS | docQSA | oQS | oQSA | a.matches | b.matches | b.closest | c.closest`):

| selector | measured value |
|----------|----------------|
| `*` | `html \| html,head,body,div#o,p.a,span.s,p.b,p.c \| p.a \| p.a,span.s,p.b \| true \| true \| p.b \| p.c` |
| `div` | `div#o \| div#o \| null \| (empty) \| false \| false \| div#o \| null` |
| `#o` | `div#o \| div#o \| null \| (empty) \| false \| false \| div#o \| null` |
| `p.a` | `p.a \| p.a \| p.a \| p.a \| true \| false \| null \| null` |
| `div#o` | `div#o \| div#o \| null \| (empty) \| false \| false \| div#o \| null` |
| `[data-x]` | `div#o \| div#o \| null \| (empty) \| false \| false \| div#o \| null` |
| `[data-x="y"]` | `div#o \| div#o \| null \| (empty) \| false \| false \| div#o \| null` |
| `div > p` | `p.a \| p.a \| p.a \| p.a \| true \| false \| null \| null` |
| `.a, .c` | `p.a \| p.a,p.c \| p.a \| p.a \| true \| false \| null \| p.c` |
| `p:first-child` | `p.a \| p.a,p.b \| p.a \| p.a,p.b \| true \| true \| p.b \| null` |
| `div span` | `span.s \| span.s \| span.s \| span.s \| false \| false \| span.s \| null` |
| `.a` | `p.a \| p.a \| p.a \| p.a \| true \| false \| null \| null` |
| `p` | `p.a \| p.a,p.b,p.c \| p.a \| p.a,p.b \| true \| true \| p.b \| p.c` |
| `span > p` | `p.b \| p.b \| p.b \| p.b \| false \| true \| p.b \| null` |

The element-scoped column is empty (null / 0) wherever the match would be the root itself or outside it,
which is the containment rule `Element.querySelector` inherits from the spec: `#o.querySelectorAll("p")` is
2, not 3.

**Correction to §1:** the prose says "fourteen selector forms" but the table lists **thirteen** rows. The
fourteenth row the tests pin is `#o` (measured `div#o / 1 / null / 0 / false / false / div#o / null` in the
same `measure.js` run); without it the id branch of both the parser and the fast path would be untested.

### 8.3 Gates (this host: go1.26.10 darwin/arm64, Apple M1 Pro)

| gate | command | exit code | number |
|------|---------|-----------|--------|
| full suite | `go test -count=1 ./...` | **0** | 43 `ok` lines, 0 `FAIL`, wall `58.76s` (`/usr/bin/time -p`) |
| vet | `go vet -all ./...` | **0** | no output |
| archtest | `go test ./internal/archtest/` | **0** | `ok … 2.961s` (3.238s inside the full run) |
| gofmt | `gofmt -l` on the seven touched files | **0** | empty output |

Per-package lines from the same run: `internal/css 2.324s`, `internal/js 3.786s`, `test/js 7.367s`,
`test/dom 3.607s`, `internal/archtest 3.238s`, `test/qc 3.578s`, `test/entrypoints 51.287s`.

### 8.4 Mutations

Each mutation was applied alone, measured, then reverted; after every revert `cmp` showed the file
byte-identical to the pre-mutation backup, and the guard suite exited 0 again.

| # | mechanism restored | command | exit code | which tests died |
|---|--------------------|---------|-----------|------------------|
| M01 | the three-form matcher: a plan that declines the index gets `selectors = nil`, so only tag/class/id compare | `go test -count=1 ./test/js -run TestSelectorMatchingSection1Forms` | **1** | 10 of 14 subtests: `*`, `p.a`, `div#o`, `[data-x]`, `[data-x="y"]`, `div > p`, `.a, .c`, `p:first-child`, `div span`, `span > p`. Survivors: `div`, `.a`, `p`, `#o` — exactly the three forms §2.2 named |
| M02 | `isLowerTag` as the whole fast-path predicate, new matcher kept | `go test -count=1 ./test/js -run TestSelectorMatchingSection1Forms` | **1** | the same 10 subtests, with a different signature: `matches`/`closest` still answer correctly (`true\|false\|…\|div#o`), only `docQS`/`docQSA` are `null`/empty. That is the proof that §2.1 is a separate bug from §2.2 |
| M03 | no quote stripping in the cascade's attribute parser (`strings.Trim(valStr, "\"' ")` removed) | `go test -count=1 ./test/js -run 'TestSelectorMatching(Section1Forms\|Attributes)'` | **1** | `TestSelectorMatchingSection1Forms/[data-x="y"]` plus 8 subtests of `TestSelectorMatchingAttributes`: `[data-x="y"]`, `[data-x='y']`, `[data-x="Y" i]`, `^=`, `$=`, `*=`, `\|=`, `[class~="a"]`. Survivors: `[DATA-X]` and `[data-x]:not([data-x=z])`, which compare no quoted value |
| M04 | `closest` starts at `n.Parent` instead of the element | `go test -count=1 ./test/js -run 'TestSelectorMatching(Section1Forms\|MatchesAndClosest)'` | **1** | five §1 subtests (`*`, `.a, .c`, `p:first-child`, `p`, `span > p` — every row whose `closest` answer is the element itself) and `TestSelectorMatchingMatchesAndClosest` with `b.closest("p") = null`, `b.closest("p.a, p.b") = null`, `a.closest("p, div") = div#o` |
| M05 | no cache: `selectorPlanFor` compiles every call | `go test -count=1 ./test/js -run TestSelectorMatching` | **0** | none, as §5 requires — the cache is recorded, not asserted. `go test ./internal/js -run TestSelector` exits **1** only on `TestSelectorPlanCacheReusesAndBounds`, which observes the cache by design |

M05 is also the measurement §3.2 asks for. `BenchmarkSelectorMatchingQueryAll` (100 general-path queries per
op, `-benchmem -count=3`, same host) with the cache and without it:

| | ns/op (3 samples) | B/op | allocs/op |
|--|-------------------|------|-----------|
| cache present | 182708 / 180617 / 181794 | 161075 | 2839 |
| cache removed | 298085 / 297220 / 302782 | 300395 | 5039 |
| ratio | 1.64–1.67× slower | 1.87× | 1.78× |

An earlier sample on the same host measured 187323/187461/190059 ns/op with the cache against
449050/429355/490688 ns/op without it, i.e. 2.3–2.6×; the direction is stable, the ratio moves with machine
load. The final pair is the one quoted above. After the revert: behaviour `rc 0`, white-box `rc 0`, `cmp`
clean, `gofmt -l` empty.

### 8.5 Cache and caps

The cache is `Runtime.selectorPlans`, one per `Runtime`, and `internal/js` creates one `Runtime` per document,
so this is the per-document cache §3.2 asks for. It holds at most `selectorCacheCap = 256` plans; on the
257th distinct string the whole map is replaced, so retained memory is bounded by 256 plans and cannot grow
with page script. A plan holds the raw string, the parsed `[]css.Selector`, an optional pre-rendered error
message and the simple-selector shape — **no DOM pointers**. Scoped copies (`:scope` rewrites) are made per
call and are never cached, so no detached node can be retained through the cache.

Interaction with `internal/engine/limits.go:23-59`: none, and no new limit was added. Those caps
(`MaxDocumentBytes`, `MaxDocumentNodes`, `MaxDocumentDepth`, `MaxAttributes`, `MaxCSSBytes`, `MaxCSSTokens`,
`MaxCSSRules`, `MaxCSSSelectors`, `MaxCSSDeclarations`, `MaxSelectorParts`, `MaxSelectorConditions`, …) are
admitted-input caps for a document and its stylesheets; the selector string here is script input the page
already holds, and the cache stores only its parse. The parser in `internal/css` applies no part/condition
cap of its own, so a pathologically long selector is bounded only by the JS string; see §9.

### 8.6 §3.2 audit: which entry points existed, and the defect ids this found

- `document.querySelector`, `document.querySelectorAll`, `Element.querySelector`, `Element.querySelectorAll`
  existed but answered three forms (§2.2). Routed through `css.Selector` by this branch.
- **`Element.matches` and `Element.closest` did not exist at all** — proposed id
  **GOOSIE-DOMMATCHCLOSEST-001**, closed by this branch. Any page calling either got
  `TypeError: … is not a function`. Chromium measured `typeof a.matches === "function"`; on `Document` it is
  `"undefined"`, and that asymmetry is kept and pinned by BR-218. Both now go through the same compiled plan
  as the queries.
- **GOOSIE-CSSATTR-EMPTY-001** (open, cascade, out of scope per §6). `[a=""]` matches elements that do not
  have `a` at all: on the §1 document Chromium answers 0, Goosie answers **7** (throwaway `package js_test`
  probe, since deleted; reproducible by asserting the count in
  `TestSelectorMatchingEmptyQuotedAttributeIsAccepted`). Root cause is the cascade's `matchAttr` treating
  "attribute absent" as "value is the empty string". BR-222 deliberately pins validity only.
- **GOOSIE-CSSPSEUDOCASE-001** (open, cascade, out of scope per §6). `validPseudoClass` lowercases the name it
  validates, but `matchPseudoClass` switches on the raw `c.Value`, so `P:FIRST-CHILD` is *accepted* and never
  *matches*. Measured: Chromium `P:FIRST-CHILD` → `p.a,p.b` and `P:NOT(.a)` → `p.b,p.c`; Goosie → 0 and 0 on
  both Document and Element. Because the cascade and the query API call the same matcher they agree with each
  other — which is what §3.2 item 5 asks for — and both diverge from Chromium. No test pins this.

Predictions of mine that measurement disproved, corrected in the tests:

- I expected `p:nth-child(2)` to be `p.b`; Chromium answers **`p.c`** (`measure13.js`, rc 0). The test pins
  `p.c`.
- I expected `p:nth-child(2 of .b)` to be 1 match; Chromium answers **0** (`measure12.js`, rc 0). The test
  pins 0.
- I believed `p:not(.a, .c)` was invalid (Selectors 3 grammar); Chromium accepts it and answers `p.b`
  (`measure10.js`, rc 0). `validPseudoClass` was changed to accept a selector list in `:not`, and the test
  pins `p.b`.
- I assumed `document.matches`/`document.closest` existed; Chromium has neither (`typeof` is `"undefined"`,
  `measure13.js`). Goosie leaves both off `Document` too.
- The pre-existing `test/dom` case `TestJSQueryContainment` asserted that `#root.querySelectorAll("p.x")`
  answers `0` ("compound unsupported"). Chromium answers **3** (`measure14.js`, rc 0); the expectation was
  corrected to `3/3`, not loosened.

### 8.7 Files and the archtest edit

- New: `internal/js/selector.go`, `internal/js/selector_index_test.go` (`package js`, white-box, because
  `indexKind` and `querySelectorAllIndexed` are unexported), `test/js/selector_matching_test.go`
  (`package js_test`, black-box through `domtest.Parse` + `Runtime.Run`).
- Modified: `internal/css/selector.go` (validity grammar, `:scope` condition scope, `UsesScope`),
  `internal/js/js.go` (runtime cache field, `matches`/`closest` wiring, old three-form block deleted),
  `test/dom/id_index_test.go` (one expectation, see above), `internal/archtest/rules.go` (one line).
- `internal/archtest/rules.go`: `internal/js` is now allowed to import `internal/css` alongside `internal/dom`
  and `internal/media`. No other rule moved. `css` imports `dom` and nothing else, so `js → css → dom` adds no
  cycle. `go test ./internal/archtest/` rc 0 (§8.3).
- `PROJECT.md`: **not edited.** It mentions `internal/js` only in prose ("a per-document JS runtime bound to
  `internal/dom`"); it carries no per-package import inventory line, so there was no clause to change. The
  paragraph still reads true.
- The seven integrator-owned files were not touched: `docs/qa-test-catalog.md`,
  `docs/qa-open-defects.json`, `docs/qa-ratchet.json`, `test/qc/catalog.go`,
  `test/qc/traceability.golden.json`, `testdata/render-known-gaps.json`, `testdata/parity.py`.

## 9. What was not measured, and known divergences left in place

1. **No live-parity run and no new scored fixture.** `make live-parity` and the corpus numbers are the
   integrator's step (§7.4). No fixture was added, so there is no corpus delta from this branch.
2. **Namespace selectors.** Chromium accepts `*|p` (→ `p.a,p.b,p.c`) and rejects `svg|p` as `SyntaxError`;
   Goosie rejects both as `SyntaxError`, because the selector lexer has no `|` in type selectors. Not pinned.
3. **Escaped identifiers.** Chromium accepts `#a\:b` (1 match) and `.\78` (1 match); Goosie throws
   `SyntaxError` (the lexer has no escape support). Not pinned; this is a pre-existing `internal/css` gap that
   also affects the cascade.
4. **Uppercase pseudo-class names.** Chromium matches `P:FIRST-CHILD`; Goosie accepts and answers 0
   (GOOSIE-CSSPSEUDOCASE-001). Not pinned, deliberately: §3.2 item 5 asks the query API to agree with the
   cascade, which it does.
5. **`[a="v" x]`.** Chromium `SyntaxError`; Goosie accepts it and answers 0. The validator only recognises a
   trailing `i`/`s` flag and otherwise trusts a quoted value; no test covers the lax case.
6. **`[data-x=""]` count.** Validity is pinned; the count is not, because Goosie answers 7 where Chromium
   answers 0 (GOOSIE-CSSATTR-EMPTY-001, out of scope per §6).
7. **Non-element receivers.** `internal/js` installs these methods on `nodeProto`, which every node type
   shares, so `document.createTextNode("x").querySelectorAll` is a function in Goosie and `undefined` in
   Chromium (measured: `probe_gaps3.js`), and Text/Comment nodes now also get `matches`/`closest`. Fixing that
   means splitting the prototype per node type, a pre-existing API shape outside this task; not pinned.
8. **Selector length abuse.** No test feeds a selector with thousands of parts or a megabyte of text.
   `css.ParseSelectorList` applies no part/condition cap, so the only bound is the JS string the page already
   holds (§8.5).
9. **`i`/`s` flag semantics beyond the measured rows.** `[data-x="Y" i]` matches and `[a=b s]`, `[a="v" s]`
   throw, which is all that was measured; case folding of non-ASCII attribute values was not.
10. **`:has()` beyond the measured forms.** Only `div:has(>p)`, `div:has(span p)`, `div:has(+p)`,
    `span:has(p)` and the validity of `p:has(> figure)` were measured.
11. **`:scope`-heavy performance.** `WithScope` copies a plan per call and is not cached; only the
    general-path benchmark (no `:scope`) was measured, so the cost of scoped selectors is unknown.
12. **Durations and allocations are one host**: go1.26.10 darwin/arm64, Apple M1 Pro. The comparisons in §8.4
    are same-host pairs; absolute numbers are not portable.
13. **Shadow DOM and `DocumentFragment` scoping** beyond what `internal/js` already exposes: not exercised.
