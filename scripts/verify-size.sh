#!/bin/sh
# Fails when a source file grows past the line limit (harness principle 12).
# A big file is where parallel changes collide and where one more branch gets
# added instead of a split; the limit asks for the split while it is cheap.
#
# Usage: verify-size.sh PATHSPEC...   e.g. verify-size.sh '*.go' ':!*_test.go'
# The language decides which files count through the pathspecs it passes.
#
# A file already over the limit is listed in .size-allowlist as "<lines> <path>".
# The number is the file's exact size: growing past it fails, and shrinking
# below it fails until the entry is lowered, so the list only ever gets shorter.
set -u

[ "$#" -gt 0 ] || { echo "verify-size: pass the pathspecs of the source files to check"; exit 2; }
git rev-parse --git-dir >/dev/null 2>&1 || { echo "verify-size: not in a git repository"; exit 1; }

limit="${VERIFY_SIZE_LIMIT:-600}"
allow=.size-allowlist
counts="$(mktemp)"
trap 'rm -f "$counts"' EXIT

git ls-files --cached --others --exclude-standard -- "$@" | sort -u | while IFS= read -r f; do
	[ -f "$f" ] || continue
	printf '%s %s\n' "$(wc -l <"$f" | tr -d ' ')" "$f"
done >"$counts"

[ -f "$allow" ] || allow=/dev/null

awk -v limit="$limit" -v allow="$allow" '
	function path(line) { return substr(line, index(line, " ") + 1) }
	FILENAME == ARGV[1] {
		if ($0 ~ /^[[:space:]]*(#|$)/) next
		ceiling[path($0)] = $1
		next
	}
	{
		n = $1; p = path($0); seen[p] = 1
		if (p in ceiling) {
			if (n > ceiling[p]) { print p ": " n " lines, grown past its " ceiling[p] " in " allow ": split it"; bad = 1 }
			else if (n <= limit) { print p ": " n " lines, within the limit: remove it from " allow; bad = 1 }
			else if (n < ceiling[p]) { print p ": shrank to " n " lines: lower its entry in " allow " from " ceiling[p]; bad = 1 }
		} else if (n > limit) {
			print p ": " n " lines, over the " limit "-line limit: split it by responsibility"; bad = 1
		}
	}
	END {
		for (p in ceiling) if (!(p in seen)) { print allow ": " p " is not a checked file: remove the entry"; bad = 1 }
		exit bad
	}
' "$allow" "$counts"
