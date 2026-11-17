# Verification layer

Status: bootstrap proposal, not an activated trust boundary. A human must approve these protected files on the base branch before installing the gate. Local evidence does not install remote required checks or branch protection.

## Agent entry points

[AGENTS.md](../../AGENTS.md#verification-layer--all-agents) contains the shared requirements for all agents. The gate scripts and installed Git commit hook are agent-independent; agents without Claude skills or event hooks invoke the launcher commands below directly. [CLAUDE.md](../../CLAUDE.md) and `/verify` are Claude-specific adapters to that shared policy, not its only entry point. Automatic task/PR hooks for other hosts are not installed by this proposal.

## Daily workflow

1. Read the **approved base** versions of [REVIEW.md](../../REVIEW.md), the [feature map](feature-map.md), and the [reliability checklist](reliability-checklist.md). Select only checklist rows whose behavior trigger matches the diff; the gate's path triggers are a conservative automated approximation.
2. Run the shared launcher's `check --suite smoke` command below, or use `/verify` through the approved [Claude skill](../../.claude/skills/verify/SKILL.md). It builds a fresh browser, drives five event-loop journeys, serves a local document, and checks its rendered pixels. Read the logs and open the screenshot.
3. Run full checks before completing a task or opening a PR on a disposable runner. The full suite includes smoke, build, uncached Go tests, vet, Python contracts, and matching incident checks. A zero exit from a build alone is not feature verification.
4. Report Important / Nit / Pre-existing findings, exact commands and exit codes, candidate/base identity, evidence paths, and unverified boundaries. Never call a required failure pre-existing without reproducing it on the base.

After approved installation, from anywhere inside the repository:

```sh
"$(git rev-parse --git-path hooks/goosie-verify)" check --suite smoke
"$(git rev-parse --git-path hooks/goosie-verify)" task
"$(git rev-parse --git-path hooks/goosie-verify)" pr
"$(git rev-parse --git-path hooks/goosie-verify)" audit
```

`task` checks the working tree, including nonignored untracked files; `commit-msg` checks only staged bytes; `pr` checks committed HEAD, not an unstaged fix. Evidence is a unique temporary directory containing JSON, command stdout/stderr, and nested smoke logs/PNG. Snapshots, runtime binaries, profiles, and servers are cleaned up; evidence is made read-only and retained until explicitly removed after review.

The Claude Stop and PreToolUse hooks call the installed launcher. The shell adapter recognizes standalone direct `gh pr create` commands, including leading environment assignments. It rejects compound shell commands, `gh pr ready`, and explicit repository/head targets rather than certify the wrong candidate. Run any commit, push, or branch switch separately before requesting the PR precheck. It is not a parser for aliases, shell expansion, or arbitrary scripts, and local HEAD evidence does not verify a remote PR. Use remote required checks for that boundary. Recursive Stop events are ignored to avoid a hook loop.

## Prerequisites and activation

Use the Go version declared in go.mod, Git, and Python 3.9+ with Pillow 11.3.0 installed in that interpreter's environment. The gate uses Python isolated mode: a user-site-only Pillow installation is insufficient. A persistent virtual environment is suitable; confirm with `python3 -I -c 'from PIL import Image; print(Image.__version__)'` using its interpreter. Additional full-check dependencies include those in [testdata/requirements.txt](../../testdata/requirements.txt), native build prerequisites, fonts, fixture assets, and `govulncheck` when dependency changes trigger it.

Full checks refuse to run unless `GOOSIE_VERIFICATION_DISPOSABLE_HOST=1` is explicitly declared **inside a disposable CI runner or VM**. Native tests write the system clipboard; a temporary HOME does not isolate that clipboard. Never set the declaration on a personal desktop merely to bypass the guard. Missing prerequisites and Go test skips remain failures, not permission to skip tests or enable golden recording. Existing WPT optional-suite skips and error-accounting gaps mean the current full suite has not been certified by this bootstrap work.

Activation is a separate human-controlled step:

1. Review and merge a policy-only PR containing the gate, checks, fixtures, skill, and review rules. Bootstrap test success cannot approve itself.
2. From that approved base checkout, invoke its runner with the persistent Python environment: `python3 -I scripts/verification_gate.py install --base main`. This pins the base SHA, runner blob, and Python interpreter in the actual Git hooks directory. It refuses an existing commit-msg/verification hook or a conflicting hooksPath; it never changes Git configuration. Arrange any existing hook integration through human review rather than overwriting it.
3. Confirm installation in a disposable clone using real successful and failing commits before enabling it for work. Set `GOOSIE_VERIFICATION_SESSION` to a stable session identifier for all commit commands in that session. The event adapter passes Claude's session ID to its own launcher calls; it cannot export that ID into a separate shell's commit environment.
4. After approved policy updates, explicitly retire the old pinned hooks and reinstall from the newly approved base through the same review process. Do not silently follow candidate policy changes.

For development of this initial proposal only:

```sh
python3 -I scripts/verification_gate.py check --suite smoke --bootstrap
python3 scripts/verification_gate_test.py
python3 scripts/verification_event_test.py
python3 scripts/verify_test.py
go test -mod=readonly -count=1 -v ./test/verification
```

Use the Pillow-capable interpreter for those Python commands. The contract suite uses temporary Git repositories and real fixture commits/hooks; it does not install hooks or create commits in this repository. `--bootstrap` executes candidate checks and reports `trusted: false`; normal invocation refuses when the base lacks approved policy.

## Commit policy and failures

The base [gate configuration](gate.json) defaults to **observe**:

| Mode | Commit behavior | Trailer |
| --- | --- | --- |
| off | Does not execute commit checks | `Verified-by: none` |
| observe | Executes checks, logs what would block, allows commit | `Verified-by: none` |
| enforce | Requires all checks, unchanged staged digest, no protected edits, and no previous failure for that base/candidate | Fresh gate/base/digest identity |

Explicit `check`, `task`, and `pr` commands still return failure in off/observe mode when checks fail. Forged verification trailers are replaced. Trailers are local evidence pointers, not cryptographic attestations or human-review approvals.

After **two actual enforce blocks in a session**, the next commit is automatically allowed without rerunning checks and stamped `Verified-by: none`. That consumes the two-block escape; the following attempt starts a new block sequence. Candidate changes do not reset session blocks. If no session ID is provided, local commits share a block sequence. A final-paragraph `Verification-skip: <specific explanation>` can allow an earlier unverified commit; empty or generic reasons do not qualify. Neither escape erases failed-candidate history.

A failed check is investigated, not retried until green. The same base/candidate digest cannot earn verification just because a later execution passes. Fix the cause, preserve the initial evidence, and stop after three unsuccessful self-fix rounds. Do not reset state or make meaningless source edits to evade that history. Preflight refusals that execute no checks are not recorded as test failures. If a flaky test cannot be repaired, document its reproduction, attempted fixes, lost coverage, replacement/owner, and removal rationale in the [flaky log](flaky-log.md); removal of a protected test needs human approval. Property-discovered production bugs must be reported before fixing them.

## Turn misses into rules

For a revert, hotfix, or repeated review comment:

1. Save the failing behavior, a deterministic reproduction, the missed verification boundary, and the real incident/PR identifiers. One incident is not enough for a general recurring-pattern rule.
2. Find **at least two independent PRs** exhibiting the same pattern. Do not invent PR numbers, count one PR twice, or treat two checklist rows from one fix as independent evidence. Until the threshold is met, keep the proposal in the incident/PR record; do not add a mandatory rule.
3. Draft a narrow trigger recognizable from a diff, the concrete check, expected evidence, runtime, failure example, and known false positives. Prefer a fast deterministic test over a broad subjective prohibition.
4. Open a separate protected-policy PR for **human approval**. Run both current base checks and clearly labeled proposal-development tests; the proposal cannot replace its own judging rules. Update checklist, gate path triggers when applicable, and tests together only after approval.
5. Record the approving PR and adoption date. New rule additions, weakening, removals, and test removals all use this same process.

The initial 14-row checklist is explicitly a bootstrap proposal mined from real local fix commits. It is not claimed to meet the two-PR threshold for later recurring rules.

## Periodic cleanup

The existing [nightly workflow](../../.github/workflows/nightly.yml) includes a read-only verification-policy audit. It checks out main, runs the base audit, and retains its report. The audit becomes due at 30 days since `last_reviewed`; due returns nonzero so maintenance is visible. It creates no issues/PRs and deletes nothing. The schedule is not active until the workflow is merged; no hosted execution is claimed here.

At every due audit and release review, a human maintainer evaluates each rule/test for confirmed catches, false positives, runtime, duplicate coverage, and superseded architecture. Keep, narrow, consolidate, or retire it with evidence in a policy PR. Preserve important lost-coverage notes in the flaky log when relevant. Advance `last_reviewed` only in the approved cleanup PR, never just to turn the audit green. The gate tests cover the exact 29/30-day boundary and prove that a candidate cannot change the base audit date.

## Bootstrap evidence and limits (2026-11-17)

- Five critical E2E journeys each passed five consecutive fresh runs; smoke passed three consecutive runs with real screenshot inspection.
- Nine deterministic Go property tests cover 10,256 generated cases. Seven deliberate temporary-overlay faults were caught; production source was not changed by the probes.
- Build, vet, architecture, focused race checks, and incident-linked checks passed. The full Go suite and native desktop UI were not certified.
- Gate contracts exercise real temporary Git commits/hooks, protected base restoration, exact candidate identity, failure/skip rejection, timeout/cancellation cleanup, and unverified escapes. Event adapter contracts cover routing and launcher behavior.
- Differential migration testing was not applicable: there is no active old/new implementation migration; Chromium parity is a separate conformance oracle.
- No repository commit, push, PR, hook installation, or remote required-check configuration was performed. This layer is implemented for human review, not yet activated enforcement.
