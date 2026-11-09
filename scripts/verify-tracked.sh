#!/bin/sh
# Fails on tracked or staged files that must never be committed (harness
# principle 13): symlinks, and machine-local directories such as pinned tools,
# scratch files and build output.
#
# A symlink breaks on Windows and on cherry-pick: a committed .tools link once
# turned into a link to itself on main. A .gitignore entry written as ".tools/"
# matches directories only, so it does not stop the link; this gate does.
#
# Usage: verify-tracked.sh [DIR...]   directory names to refuse in addition to
# .tools and .tmp, e.g. verify-tracked.sh bin node_modules
set -u

git rev-parse --git-dir >/dev/null 2>&1 || { echo "verify-tracked: not in a git repository"; exit 1; }

status=0

links="$(git ls-files --stage | awk '$1 == "120000" { sub(/^[^\t]*\t/, ""); print }')"
if [ -n "$links" ]; then
	echo "$links"
	echo "verify-tracked: symlinks are not committed; use an import, a copy, or a Makefile step"
	status=1
fi

dirs=".tools .tmp $*"
pattern="$(printf '%s\n' $dirs | sed 's/[.]/\\./g' | paste -sd'|' -)"
local_files="$(git ls-files | grep -E "(^|/)($pattern)(/|$)")"
if [ -n "$local_files" ]; then
	echo "$local_files"
	echo "verify-tracked: machine-local files are committed ($dirs); git rm --cached them and ignore them without a trailing slash"
	status=1
fi

# A pattern written as "bin/" matches directories only, so a file or a link of
# that name slips through; every ignore entry is written without the slash.
slashed="$(git ls-files --cached --others --exclude-standard -- '.gitignore' '**/.gitignore' | sort -u | while IFS= read -r f; do
	[ -f "$f" ] && grep -nE '^[^#].*/[[:space:]]*$' "$f" | sed "s|^|$f:|"
done)"
if [ -n "$slashed" ]; then
	echo "$slashed"
	echo "verify-tracked: drop the trailing slash from these .gitignore entries; it matches directories only, not a link or file of that name"
	status=1
fi

exit "$status"
