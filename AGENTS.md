<!-- CODEGRAPH_START -->
## CodeGraph

In repositories indexed by CodeGraph (a `.codegraph/` directory exists at the repo root), reach for it BEFORE grep/find or reading files when you need to understand or locate code:

- **MCP tool** (when available): `codegraph_explore` answers most code questions in one call — the relevant symbols' verbatim source plus the call paths between them, including dynamic-dispatch hops grep can't follow. Name a file or symbol in the query to read its current line-numbered source. If it's listed but deferred, load it by name via tool search.
- **Shell** (always works): `codegraph explore "<symbol names or question>"` prints the same output.

If there is no `.codegraph/` directory, skip CodeGraph entirely — indexing is the user's decision.
<!-- CODEGRAPH_END -->


## Visual Verification — Required for UI Features

For every change with user-visible output — HTML, CSS, layout, rendering,
JavaScript behaviour, or UI — visual verification is mandatory before the
work can be considered complete.

- Add or update a focused HTML fixture and E2E coverage for the feature.
- Render that fixture in both Goosie and Chromium through the existing
  Playwright E2E helper `CompareGoosieVsBrowser`; a Goosie screenshot or a
  Chromium screenshot alone is not sufficient.
- Run the relevant comparison, normally:

  ```bash
  make generate-test-data
  go test -tags=e2e ./test/e2e -run TestComprehensiveSuite
  ```

  If Playwright browsers are unavailable, install them first with
  `make install-playwright`.
- Inspect the generated Goosie, Chromium, and diff artifacts under
  `test/e2e/testdata/results/`. Any non-zero diff that exceeds the applicable
  fixture threshold must be fixed or explicitly documented as an accepted,
  reviewed limitation.
- In the final report and commit/PR description, record the command run, the
  threshold, the comparison result, and artifact paths.
- Never use `UPDATE_SNAPSHOTS=true` or update a baseline merely to hide a
  regression. Baseline changes require an intentional expected-output change
  and review of the resulting diff artifacts.

Pure backend changes with no user-visible output still require the relevant
automated tests, but do not require a synthetic Playwright comparison.
