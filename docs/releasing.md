# Publish a container release

This is a manual maintainer workflow. It does not enable remote CI or update an
existing deployment. Requirements: Git, the repository's build/verification tools,
Docker with Buildx and AMD64/ARM64 support, Python 3, tar, and GitHub CLI authenticated
as the package owner with a classic token granting `write:packages`. Keep tokens
in the CLI/credential store; never put them in arguments, source or image layers.

## Prepare a reviewed source version

Use a clean checkout of the reviewed release commit. Update the concrete image
version in `compose.release.yaml` and the installation links in README/deployment
docs together. Complete the normal verification, independent code/PR review and
platform installation checks before announcing the version. Never publish from
an active/dirty development checkout.

Create and push the version tag on that exact commit, for example:

```sh
git tag -a v0.1.0 -m 'CodeXArtifacts v0.1.0'
git push origin v0.1.0
scripts/release-image.sh v0.1.0
```

The script verifies the clean checkout, local/remote tag agreement, owner identity
and matching Compose version; it queries all package tags and refuses an existing
version. Authentication/network failures do not count as an absent package. It
verifies a fresh checkout, exports only the tagged Git tree into a private build
context, authenticates Docker over stdin, and publishes
one multi-platform image with source/version/revision OCI labels. Keep release
tags immutable; use a new version for fixes. Coordinate maintainers so only one
publisher operates on a version at a time.

The output `.work/releases/v0.1.0/image.json` records the registry manifest digest,
source commit and platforms. Retain it as release evidence. A push failure may
have published the image: inspect the package/tag before retrying. Never overwrite
a possibly successful version just because the command did not return a receipt.

## Visibility and anonymous installation

A personal-account package starts private. In GitHub's package settings, link the
source repository if needed, change visibility to **Public**, and confirm the
package name. Public visibility cannot be made private again. The source OCI
label connects the package to this repository; verify the link in GitHub.
See [GitHub's registry guide](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry)
and [visibility controls](https://docs.github.com/en/packages/learn-github-packages/configuring-a-packages-access-control-and-visibility).

Use a temporary empty Docker configuration for anonymous pulls, separate from the
publishing credential store. For both `linux/amd64` and `linux/arm64`, explicitly
pull the version/digest, inspect its architecture and source-revision label, and
run a clean installation. On an ARM host, AMD64 under supported emulation proves
that tested environment, not performance on native AMD64 hardware.

For each platform, use a new Compose project and data volume, initialize once,
retrieve/remove the bootstrap password privately, then verify in a real browser:
localhost sign-in, artifact creation and an attachment upload. Recreate the
container against the same volume and prove the artifact, attachment bytes and
account access remain available. Confirm uid/gid 65532, loopback-only host binding,
healthcheck and image labels. Use synthetic fixtures and stop only those test
projects. Never mount a production SQLite volume into the smoke app.

Inspect the final image configuration/filesystem and build context for private
state: no credentials, accounts, databases or bootstrap files may ship. The final
filesystem includes the binary, distroless runtime trust files and licenses.
`.dockerignore` excludes repository metadata and local environment/test data.

## Publish the installation assets

After those checks pass, create the GitHub release for the existing immutable tag
and attach `compose.release.yaml` and the generated `image.json`:

```sh
gh release create v0.1.0 compose.release.yaml .work/releases/v0.1.0/image.json \
  --verify-tag --title 'CodeXArtifacts v0.1.0' --notes-file <reviewed-description-file>
```

Include the exact image reference/digest, source revision and verification limits
in the description. Download the public assets without authentication and repeat
the documented initialization flow from a directory containing only that Compose
file. No source checkout or Go/Node installation may be needed by an operator.
Existing deployments follow [backup and update guidance](deployment.md#updates)
separately; publishing a release never restarts them automatically.
