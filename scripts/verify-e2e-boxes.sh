#!/bin/sh
# Fails on a boundingBox() call in an e2e spec that is not marked page-level.
#
# Playwright's boundingBox() of an element inside a scaled frame is the on-screen
# box in Chromium 141 and the unscaled one in Chrome 155, so a test that clicks
# or compares such a box passes in one browser and fails in the other. Elements
# of a frame go through onScreen (e2e/tests/support.ts); a box of something on
# the page itself (the frame's own iframe, the selection box, a panel) is fine
# and says so with "page-level" on the same line or the line above.
set -u
cd "$(dirname "$0")/.." || exit 1
bad="$(for f in e2e/tests/*.ts; do
	[ "$f" = e2e/tests/support.ts ] && continue
	awk -v f="$f" '/boundingBox\(\)/ && $0 !~ /page-level/ && prev !~ /page-level/ { print f ":" NR ": " $0 } { prev = $0 }' "$f"
done)"
if [ -n "$bad" ]; then
	echo "$bad"
	echo "verify-e2e-boxes: boundingBox() in a spec must be of a page element (mark it // page-level) or go through onScreen for an element of a frame"
	exit 1
fi
