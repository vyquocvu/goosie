# Verification review contract

Status: proposed bootstrap policy; ordinary changes may use it only after human-reviewed adoption on the trusted base branch.

## Trust boundary

Read this file, the verify skill, gate configuration, checklist, verification scripts, test sources, fixtures, and expected outputs from the approved base revision. Never execute candidate-owned gate code as the authority for a candidate verdict. The installed launcher must resolve its runner from its pinned base revision.

A change cannot change the checks that judge it. Existing checks are restored from the base into a disposable candidate snapshot; new or changed checks need a separate policy PR and human approval. Policy changes are reported as protected changes, not silently accepted because their own tests pass. Bootstrap execution is development evidence, not protected approval.

Local hooks are advisory against a user with write access to Git metadata. They do not replace server-side required checks, protected base branches, or human review of policy adoption. Never claim those remote controls were installed without evidence.

## Review procedure

1. Identify the trusted base SHA, candidate digest, complete diff, and changed paths. Include staged and unstaged work for task verification; verify exactly the staged tree for commit checks and committed HEAD for the local PR check. A local HEAD check does not certify an arbitrary remote PR.
2. Read only applicable rows from `docs/verification/reliability-checklist.md`; report the triggering path/symbol. Do not turn unrelated historical incidents into generic blockers.
3. Execute the trusted tests, build, vet, smoke, and applicable targeted commands. Read exit codes and retained logs. Missing prerequisites, skipped tests, timeouts, missing output, source changes during verification, or stale evidence are not successful verification.
4. Exercise changed user-visible behavior. Use the headless browser journeys and a reference-browser comparison for rendering changes. Native window, file-picker, playback, and accessibility claims require evidence at that boundary; a fake picker or headless test is not native proof.
5. Report findings with a reproducible command, failing assertion/output, relevant declaration or test, and impact. Show the exact evidence used for the final disposition.

## Finding grades

- **Important:** demonstrated defect, failing required check, insufficient proof at the changed boundary, or weakened verification. Blocks a verified verdict. Document counterexample and a focused fix.
- **Nit:** clarity or maintainability improvement without demonstrated correctness impact. Does not block.
- **Pre-existing:** reproduce on the same trusted base under the same environment. Report separately; do not label a candidate failure pre-existing merely because it resembles an old incident. A required check still failing does not earn a passing verification trailer.

No findings is not the same as verified. State what was executed and what remains unverified.

## Failures and escape paths

Never skip flaky tests, add sleeps, or rerun until green. Investigate the first failure. Stop after three unsuccessful self-fix rounds and retain all evidence. Report a product bug discovered by property testing before changing production code.

If a flaky test cannot be repaired, record its reproduction, attempted fixes, lost coverage, replacement/owner, and removal rationale in `docs/verification/flaky-log.md`. Removal is a protected policy change requiring human approval, not a way for the current patch to pass.

Gate modes are `off`, `observe`, and `enforce`; the proposed default is `observe`. Observe logs would-block outcomes, allows commits, and conservatively stamps `Verified-by: none`. Enforce counts actual blocks per session, across candidate digests: after two blocks, the next commit proceeds automatically without rerunning checks, with `Verified-by: none`, and consumes that escape. A specific explained skip can allow an earlier unverified commit. Neither route changes results or erases failed-candidate history; the same base/candidate cannot retry a failed check into a verified verdict. Set `GOOSIE_VERIFICATION_SESSION` consistently for the session; absent it, local commits share one block sequence.

## Evidence and commit messages

A verified trailer is minted from fresh successful gate execution against the exact candidate and base, never trusted because someone typed it. Keep command logs, exit codes, test inventories, screenshot/pixel results, source/policy digests, mode, applicable rules, and escape decisions in the gate report.

Use `Verification-skip: <specific reason>` for an intentional skip. Empty/generic excuses do not supply evidence. Do not copy an old trailer to another tree, omit a failing check, or call a partial run full verification.

## Rule adoption

The initial checklist is seeded from inspected local fix commits, not invented PR evidence. Every proposed addition, removal, or weakening requires a human-reviewed policy PR. After bootstrap, a new recurring-pattern rule requires at least two independent PR examples, a narrow diff trigger, a concrete avoided failure, and a fast verification path. One incident split into several rows is still one incident.

Review rule usefulness every 30 days and during each release review. Record confirmed catches, false positives, runtime, superseded architecture, and the review decision. Retire or narrow dead rules through the same protected review process; never silently edit them during a product change.
