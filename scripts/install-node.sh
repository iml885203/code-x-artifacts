#!/usr/bin/env bash
# Installs the pinned Node.js into <dest-dir>/node so the front-end checks run
# the same Node on a laptop and in CI, whatever the machine has.
#
# Same shape as install-golangci.sh: a release tarball from nodejs.org with its
# published checksum verified before anything is unpacked, never a piped
# install script and never "whatever apt has".
set -euo pipefail

version="${1:?usage: install-node.sh <version> <dest-dir>}"
dest="${2:?usage: install-node.sh <version> <dest-dir>}"

case "$(uname -s)" in
	Darwin) os=darwin ;;
	Linux) os=linux ;;
	*) echo "install-node: unsupported OS $(uname -s)" >&2; exit 1 ;;
esac
case "$(uname -m)" in
	arm64 | aarch64) arch=arm64 ;;
	x86_64 | amd64) arch=x64 ;;
	*) echo "install-node: unsupported architecture $(uname -m)" >&2; exit 1 ;;
esac

name="node-v${version}-${os}-${arch}"
archive="${name}.tar.gz"
base="https://nodejs.org/dist/v${version}"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

echo "install-node: fetching ${archive}"
curl -sSfL "${base}/${archive}" -o "${work}/${archive}"
curl -sSfL "${base}/SHASUMS256.txt" -o "${work}/checksums.txt"

(
	cd "$work"
	grep " ${archive}\$" checksums.txt > expected.txt
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum -c expected.txt
	else
		shasum -a 256 -c expected.txt
	fi
)

tar -xzf "${work}/${archive}" -C "$work"
rm -rf "${dest}/node"
mkdir -p "$dest"
mv "${work}/${name}" "${dest}/node"

echo "install-node: node $("${dest}/node/bin/node" --version), npm $(PATH="${dest}/node/bin:$PATH" npm --version)"
