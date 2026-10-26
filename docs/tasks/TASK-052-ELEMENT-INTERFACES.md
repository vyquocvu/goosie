# TASK-052 — element interfaces: IDL attribute reflection and `Image` (GOOSIE-IDLREFLECT-001, GOOSIE-IMAGE-001)

**Priority:** P1. **Method:** UNIT (`test/js/`, every expectation measured against Chromium first) + INT (prototype identity and write-through) + PARITY (live sweep delta after merge; a scored fixture only if one encodes a real idiom).
**Owner:** agent tasked with TASK-052. **Defects:** `GOOSIE-IDLREFLECT-001` (general), `GOOSIE-IMAGE-001` (the `Image` constructor face).
**Base:** `main` after TASK-047 **and** TASK-051 merge. Do not start earlier: all three edit `internal/js/js.go`, and TASK-051 changes the same prototype plumbing this task extends.
**Language:** English; every number must be quotable into the defect registry.

## 0. Rules

- **Do not touch** `docs/qa-test-catalog.md`, `docs/qa-open-defects.json`, `docs/qa-ratchet.json`,
  `test/qc/catalog.go`, `test/qc/traceability.golden.json`, `testdata/render-known-gaps.json`,
  `testdata/parity.py`. Propose BR ids and exact test names in §8.
- `internal/archtest/rules.go:40-64` may be edited **only** if URL reflection genuinely needs a package `js`
  cannot currently reach, and only with the justification in the commit message (TASK-051 sets the pattern).
  Prefer solving reflection with `internal/dom`, which `js` already imports.
- No `git push`, no merge, no PR. Measure before writing; correct anything in this doc your data disproves.

## 1. What the integrator measured on `main` (throwaway `package js_test` probes, run then deleted)

Document `<div id=d class="c0">`, `<a id=a href="/p?q=1">`, `<img id=i src="x.png" width="10">`,
`<input id=in value="v0">`, session URL `https://example.test/docs/page.html`.

| probe | Goosie | Chromium |
|-------|--------|----------|
| `d.className = 'k'` → `d.getAttribute('class')` | **`c0`** (write does not reach the attribute) | `k` |
| `setAttribute('class','z')` → `.className` | **`''`** (read does not come from the attribute) | `z` |
| `d.id = 'q2'` → `getAttribute('id')` / `getElementById('q2')` | `q2` / resolves | same |
| `in.value = 'zz'` → `getAttribute('value')` | `zz` | `zz` (and `value` differs from the attribute by design — check §3.3) |
| `a.href` / `a.hostname` / `a.target` / `a.rel` | **all `typeof undefined`** | absolute URL / host / string / string |
| `img` — `src`, `currentSrc`, `width`, `height`, `naturalWidth`, `naturalHeight`, `complete`, `loading`, `decoding`, `alt`, `srcset`, `sizes`, `x`, `y`, `hspace`, `vspace`, `border`, `name`, `useMap`, `isMap`, `longDesc`, `decode()` | **all `undefined`** (21 measured forms) | all present |
| `img.getAttribute('src')` / `('width')` | `x.png` / `10` | same |
| `typeof Image` / `typeof window.Image` | **`undefined`**; `new Image(3,4)` → `ReferenceError: Image is not defined` | function / constructible |
| `Object.getPrototypeOf(divElement)` | **`Object.prototype`** | `HTMLDivElement.prototype` → `HTMLElement.prototype` → `Element.prototype` |

So the engine has no element interfaces at all: properties were installed ad hoc wherever a task needed one
(`id`, form `value`/`checked`/`type`), which is why the coverage looks random rather than broken.

## 2. Cause, one sentence with its proof

Every element object handed to goja is a plain JS object whose prototype is `Object.prototype`
(`Object.getPrototypeOf` in §1), so nothing is inherited and each reflected attribute has to be installed by
hand per node — the pattern that produced `className` being writable as a dead own-property.

## 3. The fix

### 3.1 Prototypes, not per-node properties

Install a real prototype chain once per runtime: `HTMLElement.prototype` above `Element.prototype`, with
`HTMLAnchorElement`, `HTMLImageElement`, `HTMLInputElement`, … above it. Required properties: `tagName`,
`nodeName`, `id`, `className`, `classLIST`-backed `classList` write-through, `title`, `lang`, `dir`,
`hidden`, `dataset`, `style`, `attributes`, and the per-interface IDL table. Prove with an INT case that
`Object.getPrototypeOf(document.createElement('div')) !== Object.prototype`, that a **parsed** element (not
just a created one) carries the same prototype, and that `el instanceof HTMLElement` /
`instanceof HTMLAnchorElement` hold in the right places.

### 3.2 Reflection rules, per the WebIDL attribute types

`reflect` (DOMString), `reflect boolean`, `reflect long` with a default and non-negative parsing,
`reflect limited to only known values` (`loading`, `decoding`), `reflect DOMString with the URL parsing
mechanism` (`a.href`, `img.src`, `link.href`). State for each property you implement: the attribute name,
the IDL type, the getter's fallback, the setter's serialisation, and what Chromium answers — **measure the
Chromium column**, do not write it from memory, and keep the measured table in §4 as a table-driven oracle.

### 3.3 The cases where the property is deliberately not the attribute

`input.value` (value sanitisation algorithm; the *property* is the live value while `getAttribute('value')`
is the default), `textarea.value`, `select.selectedIndex`. TASK-030/031/037 built the form semantics here, so
changing write-through can regress them: their suites must stay green and this task must **not** make
`input.value` a plain reflection.

### 3.4 `Image` and load state

`window.Image` constructor (`new Image()`, `new Image(w,h)`), returning an `HTMLImageElement` that is *not*
in the document, with `src` triggering the same decode path a parsed `<img>` uses, `complete` /
`naturalWidth` / `naturalHeight` / `currentSrc` reflecting real load state, `onload` / `onerror` firing
through the existing event machinery, and `decode()` returning a promise. Two hard constraints:
- **Admission.** A script-created image must pass the same caps as a parsed one:
  `internal/image/decode.go:14-18` (probe encoded size, dimensions and pixel count before decoding) and
  `internal/engine/limits.go:23-59` for the per-document budget. A new `Image` must not be able to bypass the
  reservation in `internal/engine/engine.go` (`imgReservation`), and an unbounded script loop calling
  `new Image()` must stay inside the caps — say which cap stops it and test the answer.
- **The harness caveat.** `cmd/goosie-headless` installs no image fetcher, so script-driven loads cannot
  complete there; fixtures must assert the *absence of thrown errors* or the paint of a `data:` URI, never
  network pixels (this distinction is already recorded in `GOOSIE-CSSURLPAREN-001` and BR-201).

### 3.5 Performance

Installing accessors per prototype should be cheaper than today's per-node property churn, but say so with a
number: report allocations for a fixed tree size and a fixed number of property reads, before and after. The
gate suite asserts counters, not durations — keep it that way.

## 4. Oracles

- Table-driven `test/js` cases holding the §1 probes and their measured Chromium answers, asserting
  write-**and**-read direction for every reflected property (`prop → attribute` *and* `attribute → prop`).
- A `className` case that proves the cascade sees the change: set `className`, then show the computed style
  or the render picks the class rule — the point of the defect is silent no-op writes, and a DOM-only test
  would pass today's broken behaviour on the read side.
- `a.href` absolute-URL cases against the document base (`/p?q=1`, `./x`, `#f`, `data:…`, an unparseable
  value) — measure Chromium for each.
- `Image` cases: constructor arity, `complete` before/after a `data:` load, `onerror` for a rejected URI,
  the §3.4 caps with a loop that would exceed them.
- Regression guards for §3.3: `go test ./test/js/ ./test/form* ./test/engine/` (name the exact packages you
  ran) and `test/dom` if attribute semantics move.

## 5. Mutations

Each must kill at least one oracle; report counts and the restore rc 0.
- M01 install `className` as an own data property instead of an accessor: the write-through case must die.
- M02 make the accessor read-only (no setter): the write direction dies, the read direction stays green —
  proving the two directions are separately covered.
- M03 give `Image` a stub object with no load path: `complete`/`onload` cases must die.
- M04 let a script-created image skip `imgReservation`: the §3.4 cap case must die (this is a security-
  adjacent mutation: report it even if the surrounding code also blocks it).
- M05 reflect `input.value` straight onto the attribute: the §3.3 case must die.

## 6. Out of scope

- Selector matching (`TASK-051`) and `url()` parsing (`TASK-050`).
- Layout/paint changes: if a reflected property needs render work, report it as its own defect.
- Interfaces nobody reaches for yet. Pick the order from §1's evidence plus what the live sweep shows, and
  list in §9 which interfaces remain unimplemented rather than implying completeness.

## 7. Acceptance

1. `go test -count=1 ./...`, `go test ./internal/archtest/`, `go vet -all ./...`, `gofmt -l` clean, with
   timings and exit codes.
2. Every §1 row matches Chromium in Goosie, and the prototype claim in §2 is falsified by test.
3. Form semantics from TASK-030/031/037 unchanged (named suites green).
4. All §5 mutations behave as described; restore rc 0.
5. §8 proposes BR ids continuing from the catalog total you read (report it), lists exact test names, and
   states which §1 row each case pins.
6. §9 states what you did not measure, which interfaces remain absent, and which cap your `Image` path
   depends on — with the number, not the assertion.
