---
name: github-issue-implement
description: "Implement a CodeXArtifacts GitHub feature or bug issue with isolated changes, behavioral verification and independent review. Use when asked to implement an issue, including a clearly referenced issue from the conversation. Follow the authorized endpoint: code only, draft PR, or review through merge and post-merge verification. Does not create backlog issues or authorize publication by itself."
---

# GitHub Issue Implement

Adapted from [Orbit's feature implementation workflow](https://github.com/iml885203/orbit/blob/main/.claude/skills/orbit-feature-implement/SKILL.md)
for this repository's Go + React architecture, checks and authorization boundaries.
Use the current checkout's remote; do not assume the contributor's fork has the
same owner as upstream. Resolve repository-relative paths from its root.

## Intake and authorized endpoint

Resolve the issue number/URL from the request or unambiguous conversation context.
Ask only if the target is genuinely ambiguous. Read the live issue's body, state
and related PRs; confirm it is open, not already implemented, and has a coherent
user outcome with observable acceptance criteria. Do not duplicate an active PR.
A material product or acceptance ambiguity needs a decision before dependent work;
ordinary implementation choices do not.

Summarize the goal, feature/bug classification, acceptance checks, assumptions and
planned verification. Establish the authorized endpoint from the request/session.
Code-only or draft-only work stops there. A request to implement is not by itself
permission to merge, deploy publicly or operate unrelated services. When the session
already authorizes PR creation/merge or a local service update, complete those
phases without asking again. Do not infer external-write authority from this skill.

## Isolate and establish evidence

Read `AGENTS.md`, `PRODUCT.md`, `docs/development.md`, `docs/test-strategy.md` and
applicable scoped `AGENTS.md` files; read `DESIGN.md` for UI changes. These define
the completion standard, not Orbit-specific files or commands.

Inspect status, fetch the remote default branch and preserve other work. Use a
fresh issue branch such as `codex/issue-<number>-<slug>` in an isolated worktree or
checkout based on that branch. Use the installed Orca skill for Orca-managed state
when available; contributors do not need Orca or Orbit installed to use this skill.
Do not reuse or clean a dirty checkout. If another agent edits overlapping files,
coordinate before writing, building shared output or updating its checkout.

Run the relevant baseline checks. New checkouts need the built embedded frontend:
`make build` prepares it before Go HTTP/browser tests. Attribute baseline failures
accurately; resolve setup problems within scope, and stop if reliable verification
is genuinely blocked. For a bug, reproduce the failure first, or record concrete
source/runtime evidence and why direct reproduction is unavailable.

## Implement and verify

Trace existing behavior and nearby contracts; make the smallest cohesive change
that satisfies the issue. Keep Go + React/SQLite and the single-server model.
Preserve shared HTTP/MCP permissions, opaque-origin sandboxing, compatible `PICTOR_`
and `window.pictor` identifiers, and `website/CNAME`. Do not introduce a backend
migration, unrelated cleanup, hosted dependency or remote CI as incidental work.

Add behavioral coverage at the appropriate layer, including important refusal and
recovery cases. Follow the test strategy rather than mirroring implementation.
Browser tests are separate from `make verify`; UI/sign-in changes require real
browser verification. A raised e2e budget needs the documented three-part reason.
When Go JSON/boot contracts change, update their frontend types and tests together;
do not assume an unavailable generator target exists.

Update product/deployment/user documentation and applicable scoped invariants.
Keep synthetic fixtures distinct from real user data. Do not publish credentials,
private hosts, provider fixtures, reviewer scratch or downloaded test binaries.

For the final candidate and material review fixes, run:

```sh
make hooks
GO_TEST_FLAGS=-p=2 VERIFY_BUDGET=240 make verify
```

Run focused checks during iteration and additional browser/interop checks required
by the issue. The documented total budget does not disable individual test limits.
Do not skip hooks, weaken tests or raise limits merely to hide failures. Avoid
concurrent Vite builds and Go tests that embed its output; a build replaces that
folder, which can cause unrelated transient missing-asset failures.

## Independent review and commit

Use fresh read-only reviewer subagents with distinct lenses: simple design,
domain cohesion, and reuse/regression risk; include relevant security expertise
for authentication or data-loss boundaries. Supply the raw issue, diff, relevant
source and scoped instructions, not the desired verdict. If independent agents
are unavailable, disclose that limit and perform separate skeptical passes instead
of claiming independent approval.

Resolve Critical and Should Fix findings or demonstrate why they are invalid.
Recheck affected behavior and the completion gate after material fixes; obtain a
bounded reviewer recheck. Inspect the staged diff for secrets, scope and accidental
user changes. Commit only the issue's changes using Conventional Commits and the
issue reference. Never stage unrelated changes or use `--no-verify`.

## PR and merge, when authorized

Synchronize with the latest default branch according to repo policy; resolve
conflicts without discarding work and rerun affected gates. Push without force.
Use connected GitHub tools when sufficient, otherwise `gh` with a UTF-8 body file.
Describe the user-visible outcome, `Closes #<number>`, exact verification and limits.
Honor an explicitly requested draft PR; do not assign reviewers or milestones
without authorization.

Have a fresh reviewer inspect the actual PR diff/context. Resolve actionable
feedback and material findings, rerun affected verification and push fixes. Check
review threads, mergeability and checks on the current head. Never merge with
pending/failing/stale required checks or unresolved blocking review. This repo
currently prohibits enabling remote CI: an intentionally unconfigured check set
is not a green CI run. Record local evidence and do not invent remote results.

When merge is authorized and all gates pass, use the normal merge method with an
expected head SHA. Confirm the resulting commit and linked issue closure. If a
create/merge request times out, read back its state before retrying.

## Post-merge and completion

Fast-forward the original local default branch when safe. If active edits prevent
it, coordinate with their owner; do not reset, stash or overwrite another agent's
work without agreement. Report an unresolved condition instead of claiming sync.

Exercise the merged behavior: primary path, meaningful refusal/boundary, and the
nearest existing behavior. If authorized to update a local environment, use the
installed Orbit skill when applicable and restart only the intended service after
coordinating shared builds. Provider fixtures belong in an isolated test environment,
not the user's real identity system. A healthy port alone is not feature evidence.

Clean only task-created test services and a clean disposable checkout after
successful verification; preserve user work, data and unrelated services. Remove a
merged local issue branch only when safe. Keep concise evidence outside publication.

Report the issue/PR, final branch/commit, acceptance evidence, local checks, review
and CI status, exploratory results, issue closure/sync, and remaining limits. An
open PR or partly passing checks is not completion of an authorized merge workflow.
