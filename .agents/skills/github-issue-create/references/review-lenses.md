# Independent review briefs

Use separate fresh reviewer contexts. Supply only the raw request, full draft,
repo/commit, product documents, applicable local instructions and relevant primary
protocol sources. Specify read-only access and no external mutations. Do not supply
an intended conclusion, the author's verdict or a leading summary.

Every reviewer returns: disposition, material findings with evidence, concrete
suggested corrections, and uncertainties or limits of their investigation. Code
claims need inspected file/line or commit permalinks; demand claims may cite the
request or product evidence. Absence of usage metrics is not itself a blocker for
an early-stage product or explicit owner request.

## Claim accuracy

Verify current-state statements against the current default branch, not assumed
architecture. Follow actual call sites and tests. Check examples, compatibility
claims, versions and external standards. Distinguish runtime reproduction from
source-only inspection. Look for already shipped or duplicate work. Flag unsupported
exclusivity claims such as "the only path" or "only one file changes".

For a routine item, also check the boundary and acceptance criteria below.

## Product need

Independently reconstruct this product's users, purpose and explicit constraints.
Identify the actual workflow friction and the benefit of resolving it. Compare the
proposal with existing features, configuration/documentation and smaller options.
Check whether a proposed solution merely follows a competitor or creates avoidable
maintenance/operational cost. Do not advocate for the feature by default.

An owner request is evidence of intent, not proof of usage frequency or measured
impact. Accept a clearly labelled qualitative value hypothesis when appropriate.
Respect explicit direction changes; flag rather than invent permanent roadmap bans.
Say whether the feature is justified, needs a specific answer, duplicates existing
value, or conflicts with the product goal. Explain why with evidence.

## Boundary and acceptance (separate for nontrivial items)

Trace adjacent assumptions across affected code, storage, API/transport, account
permissions, UI, deployment, docs and compatibility. For auth, data-loss or other
sensitive work, check relevant failure/recovery and existing trust boundaries.
Do not expand scope into hypothetical hardening unrelated to the requested outcome.

Ensure criteria are observable, cover the stated outcome and relevant negative
cases, preserve named compatibility behavior and do not contradict exclusions.
Identify oversized slices and decisions that materially change scope. Estimates,
priorities and invented architecture are not substitutes for missing decisions.

## Revision verdict

When asked to recheck findings, score each listed material finding resolved,
partially resolved or unresolved with evidence. Do not silently turn this bounded
verdict into a new broad review or claim whole-issue approval from one checked fix.
The author aggregates all required lens results before filing.
