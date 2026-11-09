#!/bin/sh
# Fails when the browser tests grow past the budget in .e2e-budget (docs/test-strategy.md).
# An e2e runs in two browsers in CI and on every agent's machine, so each one has to earn
# its place; the budget makes adding one a visible one-line diff that review must justify.
#
# The count is the test( declarations in e2e/tests/*.spec.ts, including test.only/skip/fixme
# and tests inside test.describe. It reads the source instead of running
# `playwright test --list` so the gate needs neither npm packages nor a browser.
# A test declared inside a loop counts once; write each case out instead of generating it.
#
# Raising .e2e-budget is the deliberate act. Lowering it is always allowed, and the gate
# tells you when the budget has slack so the next person cannot grow into it unnoticed.
set -u
cd "$(dirname "$0")/.." || exit 1

budget_file=.e2e-budget
budget="$(grep -E '^[0-9]+$' "$budget_file" 2>/dev/null | head -n 1)"
[ -n "$budget" ] || { echo "verify-e2e-budget: $budget_file must hold the allowed number of e2e tests on a line of its own"; exit 1; }

count="$(cat e2e/tests/*.spec.ts | grep -cE '^[[:space:]]*test(\.(only|skip|fixme))?\(')"

if [ "$count" -gt "$budget" ]; then
	echo "verify-e2e-budget: $count e2e tests, over the budget of $budget in $budget_file."
	echo "An e2e is only for a genuinely valuable happy path across browser and server; errors, refusals, edge cases and regressions go to Go or vitest tests."
	echo "See docs/test-strategy.md. If the new test passes its three conditions, raise $budget_file in the same commit and justify it in review."
	exit 1
fi
if [ "$count" -lt "$budget" ]; then
	echo "verify-e2e-budget: $count e2e tests, under the budget of $budget: lower $budget_file to $count so the freed room is not spent unnoticed."
fi
exit 0
