<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **goosie** (12362 symbols, 43730 relationships, 300 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

> Index stale? Run `node .gitnexus/run.cjs analyze` from the project root — it auto-selects an available runner. No `.gitnexus/run.cjs` yet? `npx gitnexus analyze` (npm 11 crash → `npm i -g gitnexus`; #1939).

## Always Do

- **MUST run impact analysis before editing any symbol.** Before modifying a function, class, or method, run `impact({target: "symbolName", direction: "upstream"})` and report the blast radius (direct callers, affected processes, risk level) to the user.
- **MUST run `detect_changes()` before committing** to verify your changes only affect expected symbols and execution flows. For regression review, compare against the default branch: `detect_changes({scope: "compare", base_ref: "main"})`.
- **MUST warn the user** if impact analysis returns HIGH or CRITICAL risk before proceeding with edits.
- When exploring unfamiliar code, use `query({search_query: "concept"})` to find execution flows instead of grepping. It returns process-grouped results ranked by relevance.
- When you need full context on a specific symbol — callers, callees, which execution flows it participates in — use `context({name: "symbolName"})`.
- For security review, `explain({target: "fileOrSymbol"})` lists taint findings (source→sink flows; needs `analyze --pdg`).

## Never Do

- NEVER edit a function, class, or method without first running `impact` on it.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis.
- NEVER rename symbols with find-and-replace — use `rename` which understands the call graph.
- NEVER commit changes without running `detect_changes()` to check affected scope.

## Resources

| Resource | Use for |
|----------|---------|
| `gitnexus://repo/goosie/context` | Codebase overview, check index freshness |
| `gitnexus://repo/goosie/clusters` | All functional areas |
| `gitnexus://repo/goosie/processes` | All execution flows |
| `gitnexus://repo/goosie/process/{name}` | Step-by-step execution trace |

## CLI

| Task | Read this skill file |
|------|---------------------|
| Understand architecture / "How does X work?" | `.claude/skills/gitnexus/gitnexus-exploring/SKILL.md` |
| Blast radius / "What breaks if I change X?" | `.claude/skills/gitnexus/gitnexus-impact-analysis/SKILL.md` |
| Trace bugs / "Why is X failing?" | `.claude/skills/gitnexus/gitnexus-debugging/SKILL.md` |
| Rename / extract / split / refactor | `.claude/skills/gitnexus/gitnexus-refactoring/SKILL.md` |
| Tools, resources, schema reference | `.claude/skills/gitnexus/gitnexus-guide/SKILL.md` |
| Index, status, clean, wiki CLI commands | `.claude/skills/gitnexus/gitnexus-cli/SKILL.md` |

<!-- gitnexus:end -->

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
