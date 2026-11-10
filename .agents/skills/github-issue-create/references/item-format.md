# Scrum-style backlog item

Use the repository's issue form where it covers these needs; combine short sections
and omit genuinely inapplicable fields. The [Scrum Guide's Product Backlog](https://scrumguides.org/scrum-guide.html#product-backlog)
provides the value/refinement basis, not mandatory GitHub headings or a user-story syntax.

## Title

Name the outcome or observable defect in the repo's convention. Avoid a title that
commits to a mechanism before its value and constraints have been examined.

## Body shape

```markdown
## Goal and value

Who needs this, in which workflow, and what improves for them?
As a [specific user], I want [outcome], so that [benefit].

## Current and expected behavior

- Today: observed behavior, workaround and supporting evidence.
- Expected: the smallest coherent successful outcome.

## Scope

- Cases the item covers and compatibility behavior it must preserve.

## Out of scope

- Deliberate exclusions and why; unresolved owner choices stay open questions.

## Key example

A concrete before/after workflow, reproduction or realistic user steps.
Use neutral identifiers and placeholder credentials.

## Acceptance criteria

- [ ] Given [relevant context], when [action], then [observable result].
- [ ] Existing [specific supported behavior] remains available.
- [ ] Relevant refusal, error or recovery behavior is verifiable.

## Verification

How to demonstrate the criteria: suitable automated checks, manual scenarios and
any compatibility matrix needed. Distinguish checks already run from future work.

## Dependencies and open questions

Only material dependencies, risks or decisions, with the decision owner when known.
Do not invent dates, estimates, urgency or assignees.

## Product-fit and evidence review

- Need: request/evidence connecting this item to the product goal.
- Alternatives: why existing behavior or a smaller change is insufficient.
- Evidence: inspected commit, public source permalinks and probe results; label assumptions.
- Review: Ready / Needs discovery / Duplicate / Already implemented / Not recommended,
  with concise reasons and remaining nonblocking questions.
```

## Adapt the shape to the item

- **Feature:** user outcome and value hypothesis; use a user story when it clarifies
  the actor and benefit. Treat INVEST as a refinement heuristic, not a numeric score.
- **Bug:** environment/version, reproducible steps, actual versus expected behavior,
  impact and regression criteria. An unconfirmed report stays explicitly unconfirmed.
  Name a missing coverage mechanism only when the evidence supports it.
- **Enabler:** the product outcome or concrete operational risk it enables; do not
  manufacture an end-user story for an internal implementation preference.
- **Spike:** a precise uncertainty, bounded investigation and deliverable/decision
  that ends it. Do not disguise an unspecified implementation as research or silently
  turn the requested feature into a Spike.

A backlog-ready item should be discussable and testable without forcing developers
into a speculative design. Preserve the team's existing Definition of Done; do not
claim Scrum requires a separate Definition of Ready. Order/priority belongs to the
product owner and sizing to the developers unless the user provides those values.
