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
each with a solid-blue SVG data URI as `background-image`, differing only in two axes: whether the value is
quoted, and whether the paint colour is written `%230000ff` (percent-encoded, no parentheses) or
`rgb(0,0,255)` (contains a `)`):

| id  | declaration shape                                   | Chromium non-blue-band px | Goosie non-blue-band px |
|-----|-----------------------------------------------------|---------------------------|-------------------------|
| k0  | quoted, `%230000ff`                                 | 2000                      | 2000                    |
| k1  | unquoted, `%230000ff`                               | 2000                      | 2000                    |
| k2  | quoted, `rgb(0,0,255)` — payload contains `)`       | 2000                      | **0**                   |
| k3  | unquoted, `rgb(0,0,255)` — payload contains `)`     | 2000                      | **0**                   |

- Chromium, measured 2026-10-26: `NODE_PATH=<repo>/node_modules node /tmp/t50/v5chrom.js` against
  `viewport 120×90, deviceScaleFactor 1` → `blue px per band k0..k3 = [2000, 2000, 2000, 2000]`, and the
  computed `background-image` string keeps the entire payload (`length` 147 / 142 / 150 / 145), e.g.
  `url("data:image/svg+xml;utf8,<svg … fill='rgb(0,0,255)'/></svg>")`.
- Goosie, same page and same geometry on `main` (`/tmp/t50/hl -in … -out … -width 120 -height 90 -dpr 1`,
  exit 0, `presents=2`) → `[2000, 2000, 0, 0]`.
- So the discriminator is **a `)` inside the payload**, not `;utf8`, not quoting: k1 proves unquoted works,
  k3 fails only because of the parenthesis, k2 fails even though it is quoted.

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

## 8. Proposed BR ids and oracle list

(to be filled by the implementer)

## 9. Unresolved / limits, stated plainly

(to be filled by the implementer)
