#!/usr/bin/env bash
# Publish one immutable version from an already reviewed/tagged source commit.
set -euo pipefail
umask 077

fail() { echo "release-image: $*" >&2; exit 1; }
[[ $# -ge 1 && $# -le 2 ]] || fail 'usage: scripts/release-image.sh vMAJOR.MINOR.PATCH [evidence-directory]'
version="$1"
[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || fail 'use a version tag, never latest'
root="$(git rev-parse --show-toplevel)"
cd "$root"
[[ -z "$(git status --porcelain)" ]] || fail 'release checkout must be clean'
revision="$(git rev-parse 'HEAD^{commit}')"
[[ "$(git rev-parse "refs/tags/$version^{commit}")" == "$revision" ]] || fail 'version tag must name this reviewed commit'
remote_revision="$(git ls-remote origin "refs/tags/$version" "refs/tags/$version^{}" | awk '{ if ($2 ~ /\^\{\}$/) peeled=$1; else plain=$1 } END { print peeled ? peeled : plain }')"
[[ "$remote_revision" == "$revision" ]] || fail 'push the matching version tag before publishing'
image=ghcr.io/iml885203/code-x-artifacts
[[ "$(gh api user --jq .login)" == iml885203 ]] || fail 'the authenticated maintainer must own this package namespace'
check_compose() { grep -Fxq "    image: $image:$version" "$1" || fail 'release Compose must select this exact version'; }
check_compose compose.release.yaml

temporary="$(mktemp -d)"
trap 'rm -rf "$temporary"' EXIT
# Check every package version, including private tags. Only an authenticated
# 404 for the owner's package means first publication; transport/auth failures
# never mean permission to overwrite a possibly existing tag.
if gh api --paginate '/users/iml885203/packages/container/code-x-artifacts/versions?per_page=100' \
  --jq '.[].metadata.container.tags[]?' >"$temporary/tags" 2>"$temporary/query-error"; then
  if grep -Fxq "$version" "$temporary/tags"; then fail 'version already exists; choose a new version, never overwrite'; fi
else
  grep -Fq '(HTTP 404)' "$temporary/query-error" || fail 'cannot verify package tags; check authentication/connectivity and retry'
fi

# Verify in a fresh checkout, then export only the tagged tree. Ignored local
# files and later working-tree edits can never enter the Docker build context.
git clone --quiet --no-hardlinks --no-checkout "$root" "$temporary/verified"
git -C "$temporary/verified" checkout --quiet --detach "$revision"
(cd "$temporary/verified"; GO_TEST_FLAGS="${GO_TEST_FLAGS:--p=2}" VERIFY_BUDGET="${VERIFY_BUDGET:-240}" make verify)
[[ -z "$(git -C "$temporary/verified" status --porcelain)" ]] || fail 'verification changed the reviewed source tree'
mkdir "$temporary/context"
git -C "$temporary/verified" archive --format=tar "$revision" | tar -xf - -C "$temporary/context"
check_compose "$temporary/context/compose.release.yaml"
gh auth token --hostname github.com | docker login ghcr.io --username iml885203 --password-stdin
docker buildx build --platform linux/amd64,linux/arm64 \
  --build-arg "VERSION=$version" --build-arg "REVISION=$revision" \
  --tag "$image:$version" --metadata-file "$temporary/build.json" --push "$temporary/context"

output="${2:-$root/.work/releases/$version}"
mkdir -p "$output"
python3 - "$temporary/build.json" "$output/image.json" "$image" "$version" "$revision" <<'PY'
import json
from pathlib import Path
import re
import sys
metadata = json.loads(Path(sys.argv[1]).read_text())
digest = metadata['containerimage.digest']
if not re.fullmatch(r'sha256:[a-f0-9]{64}', digest):
    raise SystemExit('Invalid published image digest; inspect the registry before retrying')
record = dict(image=sys.argv[3], version=sys.argv[4], revision=sys.argv[5], digest=digest,
              platforms=['linux/amd64', 'linux/arm64'])
Path(sys.argv[2]).write_text(json.dumps(record, indent=2) + '\n')
print('Published ' + record['image'] + '@' + digest)
print('Saved source/digest record in ' + sys.argv[2])
PY
echo 'Verify public visibility, anonymous platform pulls and installation before announcing the release.'
