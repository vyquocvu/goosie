# TASK-050 — `url()` payloads containing `)` are cut short (GOOSIE-CSSURLPAREN-001)

**Priority:** P1. **Method:** UNIT (computed-value oracles in `test/style/`, `test/css/`) + INT (per-site oracles for every call site in §2.3) + PARITY (one new scored fixture in `testdata/render/css-features/`).
**Owner:** agent tasked with TASK-050. **Defect:** `GOOSIE-CSSURLPAREN-001` (P1, open).
**Base:** `main` at the commit that registers BR-200/201/202 (parent of this doc; read `git log -1 --format=%H`).
**Language:** English — every number here has to be quotable verbatim into the defect registry.

## 0. Rules for this branch

- **Do not touch** `docs/qa-test-catalog.md`, `docs/qa-open-defects.json`, `docs/qa-ratchet.json`,
  `test/qc/catalog.go`, `test/qc/traceability.golden.json`, `testdata/render-known-gaps.json`, or
  `testdata/parity.py`. The integrator owns those. Propose BR ids and exact test names in §8 instead;
  adding a `// qc:` marker for an id that is not in the catalog yet would red `go test ./test/qc/`.
- **Add the scored fixture** to `testdata/render/css-features/` but leave `MIN_SCORED_FIXTURES` alone and
  report the corpus you measured; the integrator raises the floor in the same commit that registers the case.
- No `git push`, no merge, no PR. One worktree, commits only on your branch.
- Every claim must carry the command, the real number and the exit code measured on your host. If a
  prediction you wrote earlier turns out wrong, correct it in place and say what was wrong — do not quietly
  restate it.

## 1. Reproduction, measured by the integrator on `main` before this task was written

Two hosts, one page, four declarations. The page (`/tmp/bgimg-probe/variants5.html`) is four 100×20 divs,
each with a solid-blue SVG data URI as `background-image`, differing in two axes: whether the data URI
carries a `;utf8` mediatype, and whether the paint colour is written `%230000ff` (percent-encoded, no
parentheses) or `rgb(0,0,255)` (contains a `)`). All four values are quoted; see the correction below.

| id  | declaration shape                                   | Chromium non-blue-band px | Goosie non-blue-band px |
|-----|-----------------------------------------------------|---------------------------|-------------------------|
| k0  | quoted, `;utf8`, `%230000ff`                        | 2000                      | 2000                    |
| k1  | quoted, no mediatype, `%230000ff`                   | 2000                      | 2000                    |
| k2  | quoted, `;utf8`, `rgb(0,0,255)` — payload has `)`   | 2000                      | **0**                   |
| k3  | quoted, no mediatype, `rgb(0,0,255)` — payload has `)` | 2000                   | **0**                   |

- Chromium, measured 2026-10-26: `NODE_PATH=<repo>/node_modules node /tmp/t50/v5chrom.js` against
  `viewport 120×90, deviceScaleFactor 1` → `blue px per band k0..k3 = [2000, 2000, 2000, 2000]`, and the
  computed `background-image` string keeps the entire payload (`length` 147 / 142 / 150 / 145), e.g.
  `url("data:image/svg+xml;utf8,<svg … fill='rgb(0,0,255)'/></svg>")`.
- Goosie, same page and same geometry on `main` (`/tmp/t50/hl -in … -out … -width 120 -height 90 -dpr 1`,
  exit 0, `presents=2`) → `[2000, 2000, 0, 0]`.
- So the discriminator is **a `)` inside the payload**, not the mediatype: k1 proves `;utf8` is
  irrelevant, and k3 fails only because of the parenthesis.
- **Correction, 2026-10-26 (integrator, finishing pass).** The first version of the table above labelled
  k0/k2 as "quoted" and k1/k3 as "unquoted". Re-running the same file showed all four are quoted and the
  varied axis is the `;utf8` mediatype; the pixel counts and computed lengths are unchanged. Quoting was
  measured separately (`/tmp/t50/variants6.html`, same geometry): Chromium paints `[2000, 0, 2000, 0]` for
  (quoted `%23`, unquoted `%23` with literal quotes, quoted `rgb`, unquoted `rgb` with literal quotes).
  A literal quote or `(` inside an unquoted `url()` is a bad-url in Chromium
  (`getComputedStyle().backgroundImage` reads `none`), so those two forms are a Goosie leniency, not a
  reference behaviour; Goosie paints `[2000, 2000, 2000, 2000]` there after the fix. The leniency is pinned
  by unit tests, not by the parity fixture.

Corpus context, so nobody "fixes" this by pointing at a blank page: `testdata/render/css-features/`
already contains fixtures whose background colour paints (`colors/03-opacity`), and `data:` URI backgrounds
paint on `main` for the two parenthesis-free forms above. This is a parsing defect, not a missing feature.

## 2. Cause, from reading every site that extracts a `url()` payload

### 2.1 The CSS contract

CSS Syntax Level 3 §4.3.9: inside `url(...)` a quote character is *not* the start of a string token, but
when the payload **is** quoted the tokenizer consumes up to the matching quote, and the closing `)` is then
found by balanced-paren scanning from the token start. Practically (and this is what Chromium does, per the
computed values in §1): the payload may contain `(`, `)`, `;`, `,`, spaces and `'` — an SVG data URI is full
of them — and the URL ends at the first `)` **that is not inside the quoted string**.

### 2.2 What Goosie does

The engine's tokenizer is already correct: `internal/css/parser.go:1311` and `:1891` handle 4.3.9 quotes.
The defect is in the *second* pass — the handful of places that re-extract a URL from an already-parsed
declaration string with a flat `strings.IndexByte(rest, ')')`, which cannot see whether the parenthesis is
inside quotes:

```go
// internal/style/style.go:4655 extractBackgroundURL
rest := v[i+len("url("):]
end := strings.IndexByte(rest, ')')   // :4662 — first ')', quote-blind
target := strings.TrimSpace(rest[:end])
target = strings.Trim(target, "\"'")  // :4667 — strips quotes *after* the cut
```

Stripping the quotes *after* cutting at the first `)` is the whole bug in three lines: for
`url("…fill='rgb(0,0,255)'/></svg>")` the payload is returned as `"data:…fill='rgb(0,0,255` , which
`internal/engine/limits.go:944 decodeDataImageURL` then rejects (mediatype parses, but the SVG is truncated →
decode error, or worse: a syntactically valid but wrong image).

### 2.3 Every site to audit — five, found by searching for the pattern

Search: `strings.IndexByte(<x>, ')')` over `internal/`. Do **not** fix only the one that reproduced.

| site | function | state today | what a fix must prove |
|------|----------|-------------|-----------------------|
| `internal/style/style.go:4662` | `extractBackgroundURL` | **broken**, proven by §1 | quoted + unquoted payloads containing `)`, `(`, `;`, `,` all reach `Style.BackgroundImage` whole |
| `internal/css/border_image.go:86` | `borderImageURL` | **same shape, unproven** | must be measured (a `border-image-source` with a `)` in the payload) and either fixed by the shared helper or reported as an independent defect |
| `internal/engine/limits.go:1013` | `absolutizeCSSURLs` | **same shape, unproven, and worse** | it cuts at the first `)` *then* re-emits the tail as new text, so a fetched stylesheet's quoted data URI is both truncated **and** rewritten; needs a `file://` `<link>`-stylesheet case measured end to end |
| `internal/engine/imports.go:122` | `importRef` | quote-naive: `unquote(rest[4:close])` | `@import url("…)")` behaviour; low blast radius, must be stated not assumed |
| `internal/engine/engine.go:706` | font URL extraction | **already quote-aware**: `:698-705` handles a leading quote before falling back to `)` | use as the reference for what the shared helper must do, and prove with a test that the font path keeps a `)` payload |

**Sixth reader, found in the finishing pass.** `ParseBorderImageShorthand`
(`internal/css/border_image.go`) splits the value on whitespace and then re-joins the source token with a
quote-blind `strings.Contains(tok, ")")`. A quoted payload with a space after an interior `)` was cut there
too — measured: `url("…fill='rgb(0,0,255)' /></svg>") 7 fill` parsed the source as
`…fill='rgb(0,0,255)'` and the slice as `/></svg>") 7 fill`. The branch's own shorthand oracle passed only
because its payload glued the interior `)` to the url close. Fixed with the same scanner; both shapes are
pinned by `TestBorderImageShorthandURLParen`.

## 3. The fix

- Add **one** scanner that returns the payload of the first `url(` in a declaration string plus the index
  just past its closing `)`, honouring an optional leading quote and balanced parentheses, and use it at all
  sites in §2.3 that need it. Where a site also needs the *remainder* after the `url()` (the shorthand
  parsers, `absolutizeCSSURLs`, `importRef`) the scanner must return enough to slice exactly, so no caller
  re-implements the cut.
- Placement is the implementer's decision, but state it and justify it against
  `internal/archtest/rules.go:40-64`: `internal/css` is the only package both `internal/style` and
  `internal/engine` already import, so a helper there needs no new table entry. Do not add a new package.
- Behaviour rules, each of which needs a test:
  1. `url("a)b")` → `a)b`; `url('a)b')` → `a)b` (quotes stripped, payload kept whole).
  2. `url(a(b)c)` → `a(b)c` (unquoted with balanced parens — CSS escapes spaces but real pages write
     `url(data:image/svg+xml,<svg …(100%)…>)` constantly).
  3. A payload with an escaped quote, `url("a\"b")`, is a parse error per 4.3.9 — decide fail-closed
     (`""`) and document the choice rather than emitting half a URI.
  4. Unterminated `url("abc` → `""`, never the whole rest of the declaration (this is the class of bug that
     made TASK-041's bad-string recovery necessary; keep it fail-closed).
  5. Existing behaviour that must **not** change: `url()` payload-free → `""`, `none` → `""`,
     multiple `url()` in one value → first one only, and the `%23`-encoded forms in §1 (k0/k1) which paint
     today. Any regression here is a blocker, not a footnote.
- Cap discipline: the scanner runs on values that already passed admission; do not add a second size limit
  and do not let a quote-aware scan grow the work beyond one linear pass per value. Say which limit, if any,
  your change touches (`internal/engine/limits.go:23-59`).

## 4. Oracles

- **Unit, computed value (the discriminator):** a table-driven test over the forms of §1 asserting the
  *exact* payload string that reaches `Style.BackgroundImage` / `BorderImage.Source` / the rewritten sheet,
  not just that "something painted". Name it so the row in §8 is unambiguous.
- **Per-site INT:** one test per §2.3 row that exercises that site through its real entry (cascade for
  `style.go`, `ParseBorderImageSource` for `border_image.go`, a fetched `file://` stylesheet for
  `absolutizeCSSURLs`, `@import` for `importRef`, `@font-face` for `engine.go`).
- **Parity fixture:** a new `testdata/render/css-features/16-url-paren-data-image.html` with at least the
  four §1 forms as 100×20 bands, plus one band whose payload contains a `(`. Score it with
  `python3 testdata/parity.py` (no `--render` needed if you render into the harness paths yourself; state
  which you did) and report the fixture %, corpus scored count, average match %, known-gap counts, and rc.
  Chromium reference must be regenerated on your host, not copied from a log.
- Do not add duration assertions to the gate suite; it asserts counters only.

## 5. Mutations

Behaviour-off proofs, each measured and reported with the number of test failures it produces and restored
rc 0 afterwards. At minimum:

- M01 revert `extractBackgroundURL` to `IndexByte` (i.e. the status quo): every §1-derived oracle must die.
- M02 make the scanner quote-blind but paren-balanced: the quoted-payload cases must die.
- M03 make the scanner paren-blind but quote-aware: the unquoted `url(a(b)c)` case must die.
- M04 fail-open on an unterminated quote (return the rest of the declaration): the fail-closed cases die.
- M05–M08 one per site in §2.3 left unfixed (border-image, `absolutizeCSSURLs`, `importRef`, font): each
  must be shown to be either covered by a dying oracle or genuinely unaffected — "unaffected" needs the
  measurement, not the assertion.

## 6. Out of scope

- Network-loaded images in `cmd/goosie-headless`: it installs **no** image fetcher, so only `data:` URIs can
  paint through that binary. That is a harness limitation, not this defect.
- `paintBoxShadow` gating and the rest of `GOOSIE-SHADOWRUN-001`; `GOOSIE-ANONBG-001` is closed.
- Anything in the frame path (`internal/paint`, `internal/raster`, `internal/surface`): this is a
  front-end parsing fix and must not need one.

## 7. Acceptance

1. `go test -count=1 ./...` rc 0 and `go test ./internal/archtest/` rc 0 on your host.
2. `go vet -all ./...` clean, `gofmt -l` empty for every file you touched.
3. §1's four bands paint 2000 blue px each in Goosie at 120×90 dpr 1, and the parity fixture passes ≥ 90 %
   while the rest of the corpus does not lose a fixture or a known-gap band.
4. Every mutation in §5 kills at least one test and the suite is green after restore.
5. §8 proposes BR ids from the number the integrator left free (check `docs/qa-test-catalog.md` head for the
   current total — BR-200/201/202 are already taken), lists the exact test names, and states which §2.3 sites
   each case pins.
6. §9 states, plainly, what you did **not** measure.

## 8. BR ids and oracle list (filled by the integrator, finishing pass 2026-10-26)

BR-200..202 belong to TASK-049 and BR-203..210 to TASK-047, so this task lands on **BR-211** and **BR-212**.
Catalog total goes 210 → 212; ratchet P0+P1 tagged 92 → 94.

### BR-211 (P1, UNIT) — the shared scanner

`internal/css/url.go`: `ScanURL`, `ScanURLAt`, `IndexURL`.

Oracles in `internal/css/url_test.go`:
- `TestScanURLPayload` — 28 sub-cases covering §3 rules 1–5 (quoted/unquoted, `)`/`(`/`;`/`,`/space,
  stray quotes, unterminated, backslash, empty `url()`, `none`, first-of-many, uppercase).
- `TestScanURLOffsets` — Start/AfterOpen/Close/End map back into the original string.
- `TestIndexURL` — case-insensitive without a byte-length-changing lowering.
- `TestScanURLBigPayload`, `TestScanURLBalancedParensBigPayload` — 256 KiB inputs.
- `BenchmarkScanURLDataURI`, `BenchmarkIndexURLSheetOccurrences`.

### BR-212 (P1, INT + PARITY) — every reader, end to end

| §2.3 row | site | oracle |
|----------|------|--------|
| style | `extractBackgroundURL` | `test/style/url_paren_test.go`: `TestBackgroundImageURLParenDataImage` (k0–k7), `TestBackgroundShorthandURLParen`, `TestMaskImageURLParen`, `TestBackgroundImageURLParenRegression` |
| border-image longhand | `borderImageURL` | `test/css/url_paren_test.go`: `TestParseBorderImageSourceURLParen` (18 sub-cases) |
| border-image shorthand (sixth) | `ParseBorderImageShorthand` | `TestBorderImageShorthandURLParen` (2 payload shapes) |
| absolutize | `absolutizeCSSURLs` | `internal/engine/url_paren_test.go`: `TestAbsolutizeCSSURLsKeepsParenPayload` (10 sub-cases), `TestAbsolutizeCSSURLsNoTailGarbage`, `TestExpandCSSImportsURLKeepsParenPayload` |
| @import | `importRef` | `TestImportRefURLParen` (11 sub-cases) |
| font | `extractFontURL` | `TestExtractFontURLKeepsParenPayload` (11 sub-cases), `TestCollectFontJobsParenPayload` |
| pipeline | style → decode → arena attach | `test/engine/url_paren_test.go`: `TestBackgroundDataURLParenAttaches` (6 sub-cases), `TestStyleSheetImportDataURLParenAttaches` |
| parity | fixture 16 | `testdata/render/css-features/16-url-paren-data-image.html` |

### Measurements (2026-10-26, this host, Chromium 153.0.8010.12)

- Four bands, 120×90 DPR 1, exact `(0,0,255)` px per band: Goosie before `[2000,2000,0,0]`, after
  `[2000,2000,2000,2000]`; Chromium `[2000,2000,2000,2000]` on the doc's page.
- Fixture 16 (eight bands, 230,400 `(0,0,255)` px in the reference): after **98.69% PASS** (IoU 1.0,
  coverage 0.9972950782093334, blank 0/0); before **74.69% FAIL** (IoU 0.6835312636497746, coverage
  0.517161040790554, blank 0/0). Blue pixels: Chromium 230,400, after 230,400, before 115,200 — only k0, k1,
  k4 and the control band painted.
- Corpus with the same regenerated references: after **91/91 (100.0%)**, average **96.21%**, known gaps
  **5 fixtures / 10 bands** unchanged, rc **0**; before **90/91 (98.9%)**, average **95.95%**, rc **1**.
- Gates on this tree: `go test -count=1 ./...` rc 0; `go test ./internal/archtest/` rc 0;
  `go vet -all ./...` rc 0; `gofmt -l` empty on every touched file.
- Mutations, all applied and restored by the integrator here (each mutant rc 1, restored rc 0):
  M01 revert `extractBackgroundURL` — 5 tests; M02 quote-blind scanner — 8; M03 paren-blind scanner — 6;
  M04 fail-open unterminated quote — 7; M05 border-image longhand — 2; M06 `absolutizeCSSURLs` — 1;
  M07 `importRef` — 1; M08 font — 1.

## 9. Unresolved / limits, stated plainly (filled by the integrator, finishing pass 2026-10-26)

1. **A backslash inside a quoted payload still drops the declaration.** `url("data:…\"…")` renders in
   Chromium and not in Goosie. Measured with `/tmp/t50/esc.html` (40×20 div, DPR 1, exact `(0,0,255)`):
   Chromium 800 px; Goosie **0 before and 0 after** this change. The old readers also failed on the form, so
   this is a pre-existing gap, not a regression, and the branch's scanner comment that called it "a bad
   string per §4.3.9" was wrong — §4.3.9 makes the backslash an escape. Corrected in `internal/css/url.go`
   and registered as **GOOSIE-CSSURLESCAPE-001 (P2, open)**. Unescaping a URL payload is a separate task.
2. **Unquoted payloads with a literal quote or `(` are lenient beyond Chromium.** Measured: Chromium reports
   `background-image: none` for `url(data:…<svg … fill='rgb(0,0,255)'/>…)`; Goosie paints it. This is the
   engine's existing first-paint behaviour, kept deliberately and pinned by unit tests; the parity fixture
   uses only Chromium-valid forms.
3. **The `file://` premise in §2.3 row 3 is not reachable on main.** `resolveSubresourceURL`
   (`internal/engine/limits.go:829`) accepts only http/https for a subresource, and both callers of
   `absolutizeCSSURLs` are linked-sheet fetches. The branch's file:// test therefore never exercised the
   function; it was rewritten to drive an https sheet with a fake linker
   (`TestExpandCSSImportsURLKeepsParenPayload`), and the author-sheet pipeline is covered separately by
   `TestStyleSheetImportDataURLParenAttaches`.
4. **One extra reader was fixed beyond §2.3's five** (the border-image shorthand, §2.3). Its branch oracle
   passed by fixture luck; the spaced shape now fails without the fix.
5. **Not measured here:** network-loaded images (headless installs no image fetcher, so every pixel count
   above uses `data:` URIs); a live-site sweep for this defect (sweep #9 runs separately after merge); the
   branch's own mutants (it produced none before it stopped — M01–M08 above were written and run by the
   integrator on this tree).
