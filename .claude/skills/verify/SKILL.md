---
name: verify
description: Use when verifying a Goosie change, before claiming it works, opening a PR, or reporting task completion.
---

# Verify Goosie

Use the approved base-branch version of this skill and its runner. Never change the checks, expected pixels, tests, or thresholds to make the candidate pass. Policy changes require a separate human-approved PR. During initial installation, results are bootstrap evidence, not protected-gate approval.

Use Go from go.mod and a persistent Python environment with Pillow 11.3.0. The gate runs Python in isolated mode, so user-site-only packages do not suffice. Confirm that interpreter can run `python3 -I -c 'from PIL import Image; print(Image.__version__)'`.

After human-approved installation, from inside the repository run the pinned launcher, not candidate-owned gate code:

```sh
"$(git rev-parse --git-path hooks/goosie-verify)" check --suite smoke
```

Before adoption only, from the repository root use the Pillow-capable interpreter for explicitly untrusted development evidence:

```sh
python3 -I scripts/verification_gate.py check --suite smoke --bootstrap
```

The runner builds a fresh Goosie binary, starts an ephemeral loopback HTTP server, and opens a 320×200 document through the actual headless browser. It requires exact red/static, green/JavaScript-mutated, and white/background pixels. It also executes the real event-loop journeys for navigation/back/forward/reload/form feedback, settings/search/download, file selection, in-page find, and tab isolation. Every required journey must execute and pass; skips, missing tests, process errors, timeouts, and wrong pixels fail.

Read the printed report.json and logs; open document.png. Report the command, exit status, evidence path, tested revision and scope. Synthetic frame counts, compilation, old screenshots, and a passing test name without actual execution are not substitutes.

The runner shuts down the server and browser, deletes its temporary profile/binary, and retains only evidence in the printed unique directory. It never uses the user's normal profile. Delete that evidence only after review.

On failure, inspect the named step's log and screenshot; fix the cause, not the oracle. Do not skip flaky tests, add sleeps, or rerun until green. Stop after three unsuccessful self-fix rounds. Unresolved flaky-test removal requires a reason in docs/verification/flaky-log.md and human review when a protected test is affected.

This is smoke evidence, not full conformance, native macOS UI proof, or a statement that every test passed. Run diff-triggered checks from the base REVIEW.md as well.
