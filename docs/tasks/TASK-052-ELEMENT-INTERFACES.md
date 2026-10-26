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
| `in.value = 'zz'` → `getAttribute('value')` | `zz` (Goosie writes the live value through to the attribute, so `.defaultValue` also reads `zz`) | **`v0`**: Chromium keeps the property and the attribute apart (`.value` is `zz`, `.defaultValue` is `v0`; probe1/probe5). The original row said `zz` and was wrong; the Goosie write-through is the TASK-030/031/037 contract §3.3 protects, recorded as a measured divergence in §9 |
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
- Regression guards for §3.3: `go test ./test/js/ ./test/dom/ ./test/engine/ ./internal/dom/` (there is no
  `test/form*` package; TASK-030/031/037's form semantics live in `test/engine/{forms,form_submission,
  input_types,red_checked_value,option_value,select_keyboard}_test.go`) and `test/dom` if attribute
  semantics move.

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

## 8. Results: proposed BR rows, gates, mutations, measurements

Base: `5920d98` (TASK-051 merge). `docs/qa-test-catalog.md` at that commit holds **233 rows, BR-001…
BR-233**, so this task proposes **BR-234 … BR-247**. None of the QC-owned files in §0 were edited.

The new tests first carried `// qc:BR-234` … doc tags. `go test ./test/qc/` then failed every one with
`unknown QC ID … referenced by …`, because that gate resolves tags against the catalog this task is
forbidden to touch (base `test/qc` was green). The tags were therefore converted to prose
(`// Proposed BR-234 …`) and the names below are the hand-off: when QC accepts the rows, adding the tags
back is mechanical. Everything else in the catalog/ratchet stays untouched.

### 8.1 Proposed BR rows

| proposed | exact test | pins |
|----------|------------|------|
| BR-234 | `test/js/element_interfaces_test.go::TestElementPrototypeChainIsInherited` | §1 `Object.getPrototypeOf(divElement)` row and §2: chain `HTMLDivElement>HTMLElement>Element>Node>EventTarget>Object` for parsed and created elements, `instanceof` at every level, `Document`/`Text` behaviour, illegal constructors throw `TypeError`, `title` is an accessor on `HTMLElement.prototype`, `className` is inherited not own. |
| BR-235 | `TestStringAttributesReflectBothDirections` | §1 `className` write and read rows; §3.2 DOMString reflection write→attribute and attribute→property for `id`, `className`, `slot`, `title`, `lang`, `name`, `rel`, `target`, `alt`, etc., including `null`/number/`undefined` stringification. |
| BR-236 | `TestBooleanAttributesReflectPresence` | §3.2 boolean reflection (`hidden`, `isMap`, `disabled`, `required`, `multiple`, `readOnly`, `defaultChecked`, `autofocus`): absent false, any attribute value true, true stores `""`, `0` removes. |
| BR-237 | `TestUnsignedLongAttributesReflectWithTheirDefaults` | §3.2 unsigned/signed long reflection with defaults (`width`/`height`/`hspace`/`vspace` 0, `maxLength` -1, `size` 20, `tabIndex` -1), HTML integer parsing (`12.9`→12, `12px`→12, spaces, empty→default), setter truncation and `IndexSizeError` rules. |
| BR-238 | `TestEnumeratedAttributesReflectOnlyKnownValues` | §3.2 “limited to only known values”: `loading`, `decoding`, `crossOrigin` (nullable), `referrerPolicy`, `input.type`; getter lowercases a known value else the default, setter stores raw. |
| BR-239 | `TestClassListAttributesAndStyleAreLive` | §3.1 live `classList` (add/remove/toggle/contains/item/length/value, token validation) and live `attributes`/`style`; every write reaches the attribute and vice versa, including the trailing `;` Chromium serialises in `style`. |
| BR-240 | `TestAnchorURLReflectionResolvesAgainstTheDocumentBase` | §1 `a.href`/`hostname`/`target`/`rel` row; §3.2 URL reflection for `/p?q=1`, `./x`, `../up/y`, `#f`, `?z=2`, `data:`, `http://[::1` (unparseable), `//other.test/y`, `mailto:`, `javascript:`, uppercase scheme/host, empty and spaces-only, plus URL-part setters and `origin`/`protocol`/`host`/`port`/`pathname`/`search`/`hash`. |
| BR-241 | `TestInputValueStaysLiveAndDefaultValueReflectsTheAttribute` | §1 `in.value` row (corrected above) and §3.3: the live value survives `defaultValue`/`setAttribute` round-trips, checkbox/radio read `"on"` without a `value` attribute, `checked`/`defaultChecked` write the attribute. M05's killer. |
| BR-242 | `TestImageConstructorCreatesADetachedHTMLImageElement` | §1 `typeof Image` row and §3.4: constructor arity (`Image.length` 0, one/two/null args), prototype identity, detached `ownerDocument`, `tagName`, width/height attribute reflection with truncation and clamping. |
| BR-243 | `TestImageLoadStateAndEventsForADataSource` | §1 `img` row load half and §3.4: `complete` false during the loading turn and true after, `onload`/`onerror`, `currentSrc`, natural size, `decode()` resolve/reject (`EncodingError`), empty and unparseable sources. |
| BR-244 | `TestImageReflectionsBeyondTheSource` | §1 `img` row remaining attributes: `alt`, `srcset`, `sizes`, `name`, `useMap`, `border`, `longDesc` (URL), `hspace`, `vspace`, `isMap`, `x`/`y`, and the parsed image's absolute `src`/`currentSrc`, zero natural size before a load and `complete`. |
| BR-245 | `test/engine/script_image_test.go::TestScriptClassNameReachesTheCascade` | §4 cascade bullet: a script's `className = 'on'` is visible to the load-time cascade (`getComputedStyle(...).fontSize` becomes the `.on` rule's `18px`), to `getAttribute('class')` and to `classList.contains`. |
| BR-246 | `TestScriptImageLoopStopsAtTheAttemptCap` | §3.4 “an unbounded script loop … which cap stops it”: 1,000 × `new Image()` with a `data:` source yields exactly 256 load events and 744 error events, and all 744 rejected `decode()` promises name the attempt limit. The cap is `maxLoadedImages = 256` attempts, not successes. |
| BR-247 | `TestScriptImagesChargeTheDocumentPixelBudget` | §3.4 `imgReservation`: three 4096×4096 `data:` images (16,777,216 pixels each) — two decode and report 4096×4096 natural size, the third is refused with `engine: script image exceeds the document decoded-pixel budget`, because `MaxDocumentDecodedPixels = 33,554,432` is exactly two images. M04's killer. |

The `test/js` file has 11 test functions and 292 leaf cases (measured with `go test -v` → 292 `--- PASS`
lines below the 11 parents); the engine file has 3 tests with per-assertion failure messages.

### 8.2 Gates (run from the worktree, after the last restore)

| command | rc | result |
|---------|----|--------|
| `go test -count=1 ./...` | **0** | 43 packages `ok`, 53.1 s wall; `test/js` 6.879s, `test/engine` 7.600s, `test/qc` 3.528s, `internal/archtest` 3.202s |
| `go test ./internal/archtest/` | **0** | `ok … internal/archtest 3.151s`; no import-table edit was needed |
| `go vet -all ./...` | **0** | no output |
| `gofmt -l <touched files>` | **0** | no output |
| `go test -count=1 ./test/js/ ./test/dom/ ./test/engine/ ./internal/dom/` (TASK-030/031/037 guards) | **0** | 3.366s / 0.695s / 5.022s / 1.153s |

The first full run had `test/qc` red only because of the proposed tags described above; after the tags
became prose (no assertion changed) the same command was rc 0.

### 8.3 Mutations

Each mutation was applied to the file, the targeted tests run, then the file restored from a saved copy and
`shasum -a 256 -c` verified byte-clean (`OK` for `js.go`, `element_interfaces.go`, `script_image.go`), rc 0.

| mutation | change | killed | restore |
|----------|--------|--------|---------|
| M01 | `className` installed as an own data property in `wrapNode` (the pre-TASK-052 shape) | 3 `test/js` functions (8 failing units: inherited-not-own, className→attribute, attribute→className, number stringifies, classList remove/toggle/spaces) + the engine cascade test = **4 functions / 8 units** | rc 0 |
| M02 | `className` accessor with no setter | write direction dies: className→attribute, number stringifies, spaces/classList, engine cascade = **3 functions / 4 units**; read direction (`attribute → className`) and the prototype chain stayed green | rc 0 |
| M03 | `startImageLoad` returns after setting the loading state (stub load path, nothing settles) | `TestImageLoadStateAndEventsForADataSource` = **1 function / 6 units** (load event, currentSrc, decode resolve, unparseable error×2, empty error); constructor and reflection tests stayed green | rc 0 |
| M04 | `decodeScriptImage` creates a fresh `MaxDocumentDecodedPixels` reservation per image instead of charging `s.imgReservation` | `TestScriptImagesChargeTheDocumentPixelBudget` = **1 function**; the attempt-cap test stayed green, so the two caps are separately covered | rc 0 |
| M05 | per-instance `value` accessor replaced by plain attribute reflection (`getAttribute`/`setAttribute`) | `TestInputValueStaysLiveAndDefaultValueReflectsTheAttribute/checkbox_without_a_value_reads_on` = **1 function / 1 unit** (reads `""`, wants `on`); the TASK-030/031/037 engine form suites stayed green because they drive editing through engine APIs, not through the binding's `value` property | rc 0 |

M05 had to be applied inside `defineValueAccessor`: a prototype-level override is a no-op, because
`wrapNode` installs the `value` accessor per input instance and that own accessor shadows the prototype.
That shadowing is pre-existing TASK-030/031/037 plumbing and is left alone.

### 8.4 Measurements

**Allocations (§3.5).** Probe: a fresh runtime over a fixed tree of 200 `<div id class lang dir>` elements,
then 1,400 property reads (7 per element: `id`, `className`, `lang`, `dir`, `tagName`, `nodeName`,
`nodeType`). `testing.AllocsPerRun`, measured in the `5920d98` snapshot and in this worktree, probe deleted
afterwards:

| measurement | base `5920d98` | after TASK-052 | delta |
|-------------|----------------|----------------|-------|
| cold: `js.New` + first wrap of 200 elements + 1,400 reads | 579,855 allocs/run | **84,322** | −85.5% |
| warm: 1,400 reads on cached wrappers | 5,220 allocs/run | **5,018** | −3.9% |

The cold number is the per-node property churn §3.5 predicted (the base installed every accessor and method
on every wrapper); the warm number is unchanged within ~4%, which is the point: reads stay O(1) and now come
from one prototype per interface.

**Caps for the `Image` path.** A script image calls the same `fetchImageBytes` → `sub.take` → `imgdec.Probe`
→ `imgReservation.reserve` → `imgdec.Decode` chain a parsed `<img>` uses:
`MaxEncodedImageBytes = 8 MiB`, `MaxImageDimension = 8192`, `MaxDecodedImagePixels = 16,777,216` per image,
`MaxDocumentDecodedPixels = 33,554,432` per document, `ImageFetchBudget = 6 s`, and
`maxLoadedImages = 256` attempts. The unbounded loop is stopped by the **attempt** cap (256 loads + 744
errors from 1,000 attempts); the document budget is the second wall (two 4096² images, third refused).

## 9. What was not measured, what is absent, and the divergences

### 9.1 Not measured

- **Network script images.** Every test and probe drives `data:` sources; the engine has no fetcher in
  `test/js`/`test/engine` fixtures and `cmd/goosie-headless` installs none (§3.4 caveat). The goroutine path
  in `LoadScriptImage` (fetch on a worker goroutine, settle on a microtask) is therefore exercised only by
  inspection, not by an oracle.
- **Paint of a script-created image.** A detached `HTMLImageElement` has no layout box, so no paint path is
  asserted; only the load state is.
- **Event/decode timing against Chromium.** Goosie settles in a microtask at the end of the script turn
  (deterministic); Chromium completes asynchronously on its own task queue. The tests assert the end state,
  not cross-browser scheduling, and no timer/`await` interleaving was compared.
- **`setAttribute('src', …)` load behaviour.** Only the `src` property setter starts a load; setting the
  attribute from a script does not. Chromium's behaviour for the attribute path was not measured, so this is
  recorded as a limitation rather than a parity claim.
- **Memory in bytes/RSS.** The §3.5 numbers are Go allocation counts, not resident memory; the 4096² PNGs
  are paletted to keep the budget test small (~17 MiB decoded each) and that fixture choice was not compared
  against other encodings.
- **DOMException identity.** `decode()` rejects with an `Error` whose `name` is `EncodingError` and whose
  message carries the engine reason; the browser-wide DOMException/prototype identity was not compared.

### 9.2 Interfaces still absent

- SVG interfaces: `document.createElementNS`/`<svg>` elements answer `HTMLElement`, not `SVGSVGElement`
  (or any `SVGElement`), so `instanceof SVGSVGElement` cannot hold.
- `textarea.value`, `select.selectedIndex` and the rest of §3.3's live-value family: TASK-030/031/037's
  engine-side editing remains reachable through the engine and existing bindings, but this task added no IDL
  accessors for them (only `HTMLInputElement` was extended).
- `autocomplete`, `validity`/`validationMessage`, `files`, `formAction`/`formMethod`, `labels`, `dataset`
  camelCase name conversion beyond the pre-existing proxy, `Element.attachShadow` beyond the existing
  `shadowRoot` binding, and the full `HTMLFormElement`/`HTMLSelectElement`/`HTMLTextAreaElement` method sets.
- `Image` does not implement `srcset`/`sizes` selection or `picture`; those attributes are reflected only.
- `document.createElement` remains exposed on every node wrapper (the pre-TASK-052 permissive receiver that
  `TestLibrarySetupGatePassesTheDocument` needs); `Document` also installs its own copy.

### 9.3 Measured divergences from Chromium (kept deliberately)

- **`input.value` writes through to the `value` attribute**, so `.defaultValue` follows the live value.
  Chromium keeps the property and the attribute apart (`.value = 'zz'` leaves `getAttribute('value')` at
  `v0`, `.defaultValue` `v0`). This is the TASK-030/031/037 contract §3.3 protects; §1's original cell said
  Chromium returned `zz` and was wrong, corrected in place above. It is **not** plain attribute reflection
  (M05 dies on the checkbox/radio `on` fallback).
- **`checked` writes the `checked` attribute** (`checked = true` sets it, `defaultChecked` reads it),
  whereas Chromium's `.checked` is the live checkedness, independent of the content attribute.
- **`img.width`/`height` do not fall back to the laid-out box.** Chromium answers the used box (e.g. 16 for
  a broken 404 image); Goosie answers the attribute, else the intrinsic size once loaded, else 0. Layout
  geometry does not reach `internal/js`.
- **`img.x`/`y` always read 0**; the document position of a parsed image is layout geometry, which
  `internal/js` cannot see. Detached images are 0 in Chromium too, which is the only case the binding can
  answer.
- **`complete` is false for the whole loading turn** for a `data:` source and flips in the microtask; this
  matches the measured Chromium probe (`complete:false` immediately after setting `src`, then `true` with
  the load event), but the settlement vehicle differs (microtask here, task there).
- **`Image` cannot be constructed by `new HTMLImageElement()`** (throws `TypeError`), same as Chromium; the
  `Image` function is the only path.
