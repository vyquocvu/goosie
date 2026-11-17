# Flaky-test disposition log

## 2026-11-17 — verification-layer bootstrap

No existing tests were skipped, removed, or weakened by this work. No flaky test was deleted. Gate implementation failures were reproduced with deterministic regressions and repaired; they are not labeled as flakes. Deliberate negative tests and temporary-overlay mutation failures are expected failure evidence, not flaky runs.

The complete Go suite was not run on the personal host because native clipboard tests affect shared desktop state. Existing optional-suite skips and previously reported failures remain unverified here; historical reports alone are not fresh reproductions and do not justify deleting tests.

## Required removal record

For each future unresolved flaky test, record:

- Test name and source; incident and proposed removal PR identifiers.
- Environment, exact command, original failing output, and evidence location.
- Root-cause investigation and each attempted fix, with a maximum of three unsuccessful rounds.
- Why deterministic repair was not possible; no sleeps, skips, or retry-until-green workaround.
- Coverage lost, replacement check or explicit gap, and responsible maintainer.
- Human approval reference and removal date.

A record is not authorization to alter protected checks. Remove the test only through the approved policy process described in [README.md](README.md).
