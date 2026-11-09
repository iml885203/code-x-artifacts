#!/bin/sh
# Runs the verify steps and holds them to two budgets (harness principle 4):
#
#   - VERIFY_UNIT_BUDGET (default 5s): no single test unit may take longer. This
#     is the design gate: a slow unit is really waiting instead of using a fake
#     clock or a short timeout.
#   - VERIFY_BUDGET (default 60s): the whole run. This measures the machine as
#     much as the code, so each environment sets its own, with the reason
#     written beside it.
#
# Either overrun names the slowest test units. verify-fast does not go through
# here and has no limit.
#
# Language-neutral: tasks run through the language's own task runner, named by
# VERIFY_RUNNER or the one-line file .verify-runner (make when neither is set). What counts as a test unit and how long it took is
# read by its verify-slowest task, which gets the captured output's path in the
# LOG environment variable and prints "<seconds> <unit>" lines, slowest first.
set -u

runner="${VERIFY_RUNNER:-$(cat .verify-runner 2>/dev/null || echo make --no-print-directory)}"
prepush_runner="${VERIFY_PREPUSH_RUNNER:-$runner}"

budget="${VERIFY_BUDGET:-60}"
unit_budget="${VERIFY_UNIT_BUDGET:-5}"
out="$(mktemp)"
trap 'rm -f "$out"' EXIT
start="$(date +%s)"

status=0
{ $runner verify-precommit && $prepush_runner verify-prepush; } >"$out" 2>&1 || status=1
cat "$out"
elapsed=$(($(date +%s) - start))

echo "verify: ${elapsed}s (budget ${budget}s, ${unit_budget}s per test unit)"
if [ "$status" -ne 0 ]; then
	exit 1
fi

slowest="$(LOG="$out" $runner verify-slowest)"
slow_units="$(echo "$slowest" | awk -v b="$unit_budget" '$1 + 0 > b + 0')"
if [ -n "$slow_units" ]; then
	echo "verify: test units over ${unit_budget}s:"
	echo "$slow_units"
	echo "verify: use a fake clock or a short timeout instead of waiting; wait for the event, never a fixed sleep."
	exit 1
fi
if [ "$elapsed" -gt "$budget" ]; then
	echo "verify: over the time budget. Slowest test units:"
	echo "$slowest"
	echo "verify: no unit is over its own budget, so look at the total: fewer serial tests, or an environment budget sized for this machine."
	exit 1
fi
