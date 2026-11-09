#!/bin/sh
# The CI cache is a local file on the node, and two pipelines on one branch can
# restore and save it at the same moment ("stale file handle"). The restore then
# leaves a browser that is the right version but cut short: Playwright's install
# finds it complete and the browser crashes with SIGSEGV when launched. After an
# install, run each cached browser binary; delete one that does not start, so
# the install that follows downloads it again.
# Usage: e2e-browser-heal.sh BINARY_GLOB...   (exit 0 when nothing was broken, 3 when something was removed)
set -u
removed=0
for bin in "$@"; do
	[ -e "$bin" ] || continue
	if ! "$bin" --version >/dev/null 2>&1; then
		dir="$(dirname "$(dirname "$bin")")"
		echo "e2e-browser-heal: $bin does not start; removing $dir"
		rm -rf "$dir"
		removed=3
	fi
done
exit "$removed"
