#!/bin/sh
# Proves verify-agents-md.sh keeps its guarantee, in a scratch repo: a test
# named on a "Tests:" line that exists passes, the same test renamed fails,
# restored passes again, and the same holds for a front end test file.
set -eu

# A git hook runs this with GIT_DIR and GIT_INDEX_FILE set; left in place, the
# scratch repo's git commands would write to the real repository.
for v in $(git rev-parse --local-env-vars); do unset "$v"; done

here="$(cd "$(dirname "$0")" && pwd)"
top="$(git -C "$here" rev-parse --show-toplevel)"
mkdir -p "$top/.tmp"
dir="$(mktemp -d "$top/.tmp/agents-md-selftest.XXXXXX")"
repo="$dir/repo"
mkdir "$repo"
trap 'rm -rf "$dir"' EXIT

cd "$repo"
git init -q .
mkdir -p internal/a web/src
cat >internal/a/AGENTS.md <<'M'
# a

- **An invariant.**
  Tests: `TestItHoldsTheLine`, `TestAnotherOne`; web/src/b.test.ts (`b renders the title`).
M
cat >internal/a/a_test.go <<'G'
package a

import "testing"

func TestItHoldsTheLine(t *testing.T) {}
func TestAnotherOne(t *testing.T)     {}
G
printf 'it("b renders the title", () => {})\n' >web/src/b.test.ts
git add -A

check() { # check WANT_EXIT LABEL
	set +e
	out="$(sh "$here/verify-agents-md.sh" 2>&1)"
	rc=$?
	set -e
	if [ "$rc" -ne "$1" ]; then
		echo "verify-agents-md-selftest: $2: want exit $1, got $rc"
		echo "$out"
		exit 1
	fi
}

check 0 "all named tests exist"

cp internal/a/a_test.go "$dir/a_test.go" # the copy is outside the repo: a stray file in it would still name the test
sed -i.bak 's/TestItHoldsTheLine/TestItHoldsTheLineRenamed/' internal/a/a_test.go
rm internal/a/a_test.go.bak
check 1 "a renamed Go test must fail"
printf 'package a\n// TestItHoldsTheLine\nvar name = "TestItHoldsTheLine"\n' >internal/a/a.go
printf '\n/* func TestItHoldsTheLine(t *testing.T) {} */\n' >>internal/a/a_test.go
check 1 "production strings and commented declarations cannot satisfy a test reference"
case "$out" in *TestItHoldsTheLine*) ;; *) echo "verify-agents-md-selftest: the failure does not name the test"; exit 1 ;; esac
cp "$dir/a_test.go" internal/a/a_test.go
check 0 "the Go test restored"
sed -i.bak 's/TestItHoldsTheLine(t \*testing.T)/TestItHoldsTheLine(t string)/' internal/a/a_test.go
rm internal/a/a_test.go.bak
check 1 "a helper with the same name is not a Go test"
cp "$dir/a_test.go" internal/a/a_test.go
check 0 "the declaration restored"

cp web/src/b.test.ts "$dir/b.test.ts"
sed -i.bak 's/renders the title/renders the heading/' web/src/b.test.ts
rm web/src/b.test.ts.bak
check 1 "a renamed front end test must fail"
cat >>web/src/b.test.ts <<'T'
// it("b renders the title", () => {})
const example = 'b renders the title';
T
check 1 "frontend comments and unrelated strings cannot satisfy a test reference"
cp "$dir/b.test.ts" web/src/b.test.ts
check 0 "the front end test restored"

awk 'BEGIN { for (i=0; i<120; i++) print "index" }' >AGENTS.md
check 0 "the project index may contain 120 lines"
echo index >>AGENTS.md
check 1 "the project index cannot contain 121 lines"
export VERIFY_AGENTS_LINES=150
check 1 "a larger harness default cannot raise the project ceiling"
unset VERIFY_AGENTS_LINES

echo "verify-agents-md-selftest: ok"
