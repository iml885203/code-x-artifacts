---
name: github-issue-create
description: Draft, review and create GitHub issues as Scrum-style backlog items grounded in the target product and current code. Use for 開 issue, feature requests, bug reports, backlog refinement, or substantial issue rewrites. Check actual product need, evidence, duplicates, scope and acceptance criteria before filing. Does not implement the feature or serve as a general code-review skill.
---

# GitHub Issue Create

Produce a useful backlog item, verify its claims, and decide whether it belongs in
this product before creating it. A polished issue is not evidence that its feature
is needed. Reporting an existing issue or explaining why nothing should be filed
is a valid outcome.

Adapted from [Orbit's issue-creation workflow](https://github.com/iml885203/orbit/blob/main/.claude/skills/orbit-issue-create/SKILL.md),
with product-fit review and a Scrum-style item format. The headings are a practical
convention, not a prescribed Scrum template. Keep priority with the product owner
and sizing with the developers; do not invent story points, deadlines or sprint placement.

## Establish the target and evidence

- Resolve the intended product from the request and conversation, not cwd alone.
  State the local repo and GitHub `owner/name`. Ask only when the target is ambiguous.
- Read applicable `AGENTS.md`, product purpose/constraints, contribution rules and
  issue templates when present. Use README, current user flows and source as
  evidence when a formal product document is absent; do not invent a product goal.
- Inspect local status, fetch the current default branch when possible, and check
  the requested behavior there. Do not reset, checkout over, or merge into the
  user's work to investigate. Distinguish a local implementation from a shipped one.
- Search open and closed issues and relevant PRs for an existing implementation,
  duplicate or earlier decision. Explain unavailable access rather than claiming
  the search was complete. Avoid speculative file/line citations.
- Separate requester reports, observed/reproduced behavior, source inspection,
  external protocol requirements, and assumptions. Pin source evidence to the
  examined commit; verify time-sensitive technical requirements in primary docs.

## Assess product need, then draft

Read [the item format](references/item-format.md). Start with the problem and
outcome, then choose Feature, Bug, Enabler or Spike as appropriate.

For a feature, establish the user, concrete workflow friction and intended value.
Compare the proposed capability with existing functionality, a documentation or
configuration fix, and a smaller viable change. An explicit owner request can be
valid qualitative demand; do not fabricate usage research or demand metrics.

Evaluate fit against this product's purpose and current commitments. Record the
cost and assumptions behind the recommendation. A capability being absent is not
proof it is needed; another product offering it is not enough by itself. Conversely,
"not implemented" or a prior roadmap exclusion is not a permanent veto when the
owner explicitly requests a new direction. Surface real conflicts for their decision.

Keep the item to a coherent, independently verifiable outcome. Suggest an epic or
slices when necessary; do not create extra tickets without authorization. Mark
implementation ideas as proposals, not source facts or mandatory architecture.
Put unresolved design choices in open questions without converting them into
unapproved exclusions. Draft-only requests remain draft-only.

## Review before filing

First re-read the draft as a whole: verify current/expected behavior, examples,
scope and acceptance criteria agree. Recheck cited evidence on the examined tree,
remove public secrets/private identifiers, and ensure every acceptance criterion
is observable rather than a vague implementation task.

Then run independent reviewer subagents with the prompts in
[review lenses](references/review-lenses.md). Use fresh contexts and parallel,
read-only review where available. Always cover both claim accuracy and product
need; add a separate boundary/acceptance reviewer for security-sensitive,
architectural, cross-module or otherwise nontrivial requests. For a small item,
combine boundary coverage with claim review rather than inventing ceremony.
Give reviewers the raw request, draft, target repo/commit and relevant product
sources—not the author's intended verdict. Reviewers may read more relevant source.
They must not create issues, change source or communicate externally.

Resolve material findings with evidence or revise the draft. Return changed claims
or criteria to their reviewer for a bounded verdict check. Allow up to two revision
rounds; unresolved product decisions or evidence gaps stay visible and stop filing.
If subagents are genuinely unavailable, perform separate skeptical passes, disclose
that independence was unavailable, and do not claim independent approval.

Use one disposition:

- **Ready:** a grounded need, verified present-state claims, coherent scope and
  testable acceptance criteria; no unresolved blocker.
- **Needs discovery:** a material fact, value assumption or owner decision is
  missing. Keep a draft and ask the smallest question that changes the outcome.
- **Duplicate / already implemented:** report the existing URL or source evidence;
  do not file another request for the same outcome.
- **Not recommended:** explain the product mismatch or lower-cost alternative and
  the evidence. The owner can revise the goal; do not silently overrule them.

Readiness for backlog refinement is not a promise of sprint commitment or delivery.
Minor implementation questions may remain open; questions that change the problem,
solution boundary or acceptance criteria are blockers.

## Publish within the authorized scope

An explicit request to create the issue authorizes filing a **Ready** item; do not
ask again for routine wording. If you materially changed the requested outcome or
scope, show that concrete difference and obtain the missing decision first. A
review or skill-creation request alone does not authorize publishing test issues.

Use the connected GitHub tools when sufficient, otherwise `gh`. Respect the repo's
language, template and existing labels. Do not create labels, assign people,
commit to milestones or edit other issues without authorization. With `gh`, write
the exact Markdown to a UTF-8 file and use `--body-file`.

Before creating, recheck duplicates if drafting/review took long enough for that
state to matter. After creation, read back title/body/repo and report the URL plus
review disposition. A timed-out create may have succeeded: search/read back before
retrying, never blindly create again. For a substantial rewrite, verify the diff
against the existing issue and use the same review gate.

Keep review scratch files outside the publication set. A later correction belongs
in the existing issue when authorized; preserve the reason for material scope
corrections without publishing an abandoned conversation transcript.
