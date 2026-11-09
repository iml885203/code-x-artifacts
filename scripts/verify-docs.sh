#!/bin/sh
# Fails on the shapes of history and of untracked short-term fixes that have no
# legitimate reading (harness principles 9 and 10):
#
#   - a work-marker comment without an issue reference: a shortcut nobody tracks
#     (write it as TODO(#12) or TODO(https://...));
#   - a changelog or history heading in a Markdown file;
#   - a Markdown heading or list item that starts with a date: a dated change note.
#
# Prose history ("used to", "changed to") is not checked: no pattern tells it
# apart from ordinary text without false alarms, and a gate that cries wolf gets
# bypassed. That part stays a written rule.
set -u

git rev-parse --git-dir >/dev/null 2>&1 || { echo "verify-docs: not in a git repository"; exit 1; }

hits="$(mktemp)"
trap 'rm -f "$hits"' EXIT
status=0

# search PATTERN-ARGS... writes the matches to $hits. git grep exits 1 for no
# match and above 1 when it could not search; that must fail the gate, never
# read as clean. --untracked checks files not yet added; it cannot be combined
# with submodule.recurse, so recursion is turned off here.
search() {
	git grep --untracked --no-recurse-submodules "$@" >"$hits"
	rc=$?
	if [ "$rc" -gt 1 ]; then
		echo "verify-docs: git grep failed (exit $rc)"
		exit 2
	fi
}

# POSIX ERE (git grep -E) has no \b, so the word boundary is spelled out.
search -nE '(//|#|/\*|<!--|--)[[:space:]]*(TODO|FIXME|HACK|XXX)([^A-Za-z0-9_]|$)' -- ':!*.md'
markers="$(grep -vE '(TODO|FIXME|HACK|XXX)\([^)]*(#[0-9]+|https?://)' "$hits")"
if [ -n "$markers" ]; then
	echo "$markers"
	echo "verify-docs: a work marker needs the issue that tracks it, e.g. TODO(#12)"
	status=1
fi

search -niE '^#{1,6}[[:space:]]+(changelog|change log|history|version history|release notes|變更紀錄|版本紀錄|更新紀錄|修改紀錄)[[:space:]]*$' -- '*.md'
if [ -s "$hits" ]; then
	cat "$hits"
	echo "verify-docs: docs describe what is true now; history lives in git log and the issues"
	status=1
fi

search -nE '^[[:space:]]*(#{1,6}|[-*+]|[0-9]+\.)[[:space:]]+(\*\*|\[)?[0-9]{4}-[0-9]{2}-[0-9]{2}' -- '*.md'
if [ -s "$hits" ]; then
	cat "$hits"
	echo "verify-docs: a dated change note is history; rewrite the doc to the current state"
	status=1
fi

exit "$status"
