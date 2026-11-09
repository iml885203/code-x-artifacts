#!/bin/sh
# Keeps the AGENTS.md files true and the root one short (harness principles 8 and 14).
#
#   - Every test named on a "Tests:" line must exist. A renamed test otherwise
#     leaves the invariant pointing at nothing while the list still reads as a spec.
#   - The root AGENTS.md stays under VERIFY_AGENTS_LINES lines (project ceiling 120). It
#     loads into every session and every parallel agent edits it; detail for one
#     area belongs in that directory's own AGENTS.md.
#
# Go test names must be real top-level testing.T declarations; frontend titles
# must be literal test/it declarations. Other quoted
# identifiers in the prose are checked against source as before.
set -u

checker="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/verify-agents-tests.go"

git rev-parse --git-dir >/dev/null 2>&1 || { echo "verify-agents-md: not in a git repository"; exit 1; }

max="${VERIFY_AGENTS_LINES:-120}"
case "$max" in *[!0-9]*|'') echo "verify-agents-md: VERIFY_AGENTS_LINES must be a positive integer"; exit 2 ;; esac
[ "$max" -gt 0 ] || { echo "verify-agents-md: VERIFY_AGENTS_LINES must be positive"; exit 2; }
[ "$max" -le 120 ] || max=120
status=0

if [ -f AGENTS.md ]; then
	lines="$(wc -l <AGENTS.md | tr -d ' ')"
	if [ "$lines" -gt "$max" ]; then
		echo "verify-agents-md: AGENTS.md is $lines lines, over $max: move area detail into that directory's AGENTS.md"
		status=1
	fi
fi

# A "Tests:" entry may wrap; it runs until a blank line, a list item or a heading.
names="$(git ls-files --cached --others --exclude-standard -- 'AGENTS.md' '**/AGENTS.md' | sort -u | while IFS= read -r f; do
	[ -f "$f" ] && awk '
		/Tests:/ { on = 1; $0 = substr($0, index($0, "Tests:")) }
		on && (/^[[:space:]]*$/ || /^[[:space:]]*[-*+] / || /^#/) && !/Tests:/ { on = 0 }
		on { while (match($0, /`[^`]+`/)) { print substr($0, RSTART + 1, RLENGTH - 2); $0 = substr($0, RSTART + RLENGTH) } }
	' "$f"
done | sort -u)"

[ -n "$names" ] || exit "$status"

# Search prose identifiers in tests first; this keeps vendored bundles out of
# the common path. Go declarations are independently verified below.
wanted="$(mktemp)"
found="$(mktemp)"
rest="$(mktemp)"
declared="$(mktemp)"
trap 'rm -f "$wanted" "$found" "$rest" "$declared"' EXIT
printf '%s\n' "$names" | sort -u >"$wanted"

search() { # search PATTERNFILE PATHSPEC...: print the names found, once each
	list="$1"
	shift
	git grep --untracked --no-recurse-submodules -hoFw -f "$list" -- "$@" >"$found"
	rc=$?
	if [ "$rc" -gt 1 ]; then
		echo "verify-agents-md: git grep failed (exit $rc)"
		exit 2
	fi
	sort -u -o "$found" "$found"
}

# Parse declarations rather than accepting a comment, helper or production string.
go run "$checker" >"$declared" || exit 2
search "$wanted" '*_test.go' '*.test.ts' '*.test.tsx' '*.spec.ts'
comm -23 "$wanted" "$found" >"$rest"
if [ -s "$rest" ]; then
	search "$rest" ':!*.md'
	comm -23 "$rest" "$found" >"$wanted"
else
	: >"$wanted"
fi

# Named Go tests and quoted frontend titles must declare a test.
sort -u "$declared" >"$found"
# Add undeclared names to the unresolved prose identifiers.
printf '%s\n' "$names" | grep -E '^Test[A-Z0-9_]|[[:space:]]' | sort -u >"$rest"
comm -23 "$rest" "$found" >>"$wanted"
sort -u -o "$wanted" "$wanted"

while IFS= read -r name; do
	echo "verify-agents-md: invariant names test $name, which does not exist"
	status=1
done <"$wanted"

exit "$status"
