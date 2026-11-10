package releaseimage

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseEntryPointRefusesAmbiguousOrPreviouslyPublishedVersions(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"duplicate", "unavailable", "dirty", "wrong-tag", "invalid-version", "new", "older-version", "compose-mismatch", "ignored-file", "mutated-tracked"} {
		t.Run(scenario, func(t *testing.T) { t.Parallel(); exerciseRelease(t, scenario) })
	}
}

func exerciseRelease(t *testing.T, scenario string) {
	t.Helper()
	parent := t.TempDir()
	root := filepath.Join(parent, "source")
	bin := filepath.Join(parent, "tools")
	for _, dir := range []string{root, bin} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	env := append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
	git := func(args ...string) {
		t.Helper()
		command := exec.CommandContext(t.Context(), "git", args...) //nolint:gosec // G204: fixed git executable, only test-owned arguments/paths.
		command.Dir = root
		command.Env = env
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git: %s %v", out, err)
		}
	}
	write := func(path, text string, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(path, []byte(text), mode); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q")
	imageVersion := "v0.1.0"
	if scenario == "compose-mismatch" {
		imageVersion += "-rc.1"
	}
	write(filepath.Join(root, "compose.release.yaml"), "services:\n  app:\n    image: ghcr.io/iml885203/code-x-artifacts:"+imageVersion+"\n", 0o600)
	write(filepath.Join(root, ".gitignore"), "*.private\n", 0o600)
	git("add", "compose.release.yaml", ".gitignore")
	git("-c", "user.name=Synthetic", "-c", "user.email=test@example.org", "commit", "-qm", "release fixture")
	git("tag", "v0.1.0")
	remote := filepath.Join(parent, "remote.git")
	git("init", "--bare", "-q", remote)
	git("remote", "add", "origin", remote)
	git("push", "-q", "origin", "v0.1.0")
	if scenario == "ignored-file" {
		write(filepath.Join(root, "input.private"), "must not be published", 0o600)
	}
	if scenario == "dirty" {
		write(filepath.Join(root, "unreviewed"), "change", 0o600)
	}
	if scenario == "wrong-tag" {
		git("-c", "user.name=Synthetic", "-c", "user.email=test@example.org", "commit", "--allow-empty", "-qm", "different commit")
	}
	write(filepath.Join(bin, "gh"), `#!/bin/sh
if [ "$1" = auth ]; then printf 'synthetic-secret-must-not-leak\n'; exit 0; fi
if [ "$2" = user ]; then echo iml885203; exit 0; fi
case "$RELEASE_SCENARIO" in
 duplicate) echo v0.1.0;;
 unavailable) echo 'query failed (HTTP 403)' >&2; exit 1;;
 new) echo 'not found (HTTP 404)' >&2; exit 1;;
 *) echo v0.0.1;;
esac
`, 0o700)
	write(filepath.Join(bin, "make"), `#!/bin/sh
printf 'ignored verification output' > generated.private
if [ "$RELEASE_SCENARIO" = mutated-tracked ]; then echo mutation >> compose.release.yaml; fi
exit 0
`, 0o700)
	write(filepath.Join(bin, "docker"), `#!/bin/sh
printf '%s\n' "$*" >> "$RELEASE_DOCKER_CALLS"
if [ "$1" = login ]; then cat >/dev/null; exit 0; fi
while [ "$#" -gt 0 ]; do
 if [ "$1" = --metadata-file ]; then shift; metadata="$1"; fi
 context="$1"
 shift
done
if [ -e "$context/input.private" ] || [ -e "$context/generated.private" ]; then echo "private file entered build context" >&2; exit 3; fi
printf '{"containerimage.digest":"sha256:0000000000000000000000000000000000000000000000000000000000000000"}' > "$metadata"
`, 0o700)
	script, err := filepath.Abs("../release-image.sh")
	if err != nil {
		t.Fatal(err)
	}
	version := "v0.1.0"
	if scenario == "invalid-version" {
		version = "latest"
	}
	calls := filepath.Join(parent, "docker-calls")
	output := filepath.Join(parent, "evidence")
	command := exec.CommandContext(t.Context(), "bash", script, version, output) //nolint:gosec // G204: this repository script and test-owned paths/version.
	command.Dir = root
	env = append(env, "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "RELEASE_SCENARIO="+scenario, "RELEASE_DOCKER_CALLS="+calls)
	command.Env = env
	data, err := command.CombinedOutput()
	checkReleaseResult(t, scenario, version, output, calls, data, err)
}

func checkReleaseResult(t *testing.T, scenario, version, output, calls string, data []byte, err error) {
	t.Helper()
	if strings.Contains(string(data), "synthetic-secret-must-not-leak") {
		t.Fatal("credential leaked")
	}
	successful := scenario == "new" || scenario == "older-version" || scenario == "ignored-file"
	if !successful {
		if err == nil {
			t.Fatal("unsafe publication was accepted")
		}
		if _, err := os.Stat(calls); !os.IsNotExist(err) {
			t.Fatal("Docker invoked before refusal")
		}
		return
	}
	if err != nil {
		t.Fatalf("release failed: %s %v", data, err)
	}
	checkReleaseEvidence(t, version, output, calls)
}

func checkReleaseEvidence(t *testing.T, version, output, calls string) {
	t.Helper()
	record, err := os.ReadFile(filepath.Join(output, "image.json")) //nolint:gosec // G304: output is inside this test's TempDir.
	if err != nil {
		t.Fatal(err)
	}
	var metadata struct {
		Version, Revision, Digest string
		Platforms                 []string
	}
	if err := json.Unmarshal(record, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.Version != version || len(metadata.Revision) != 40 || !strings.HasPrefix(metadata.Digest, "sha256:") || len(metadata.Platforms) != 2 {
		t.Fatalf("incomplete publication record: %s", record)
	}
	invocation, err := os.ReadFile(calls) //nolint:gosec // G304: calls is inside this test's TempDir.
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(invocation), "--platform linux/amd64,linux/arm64") || !strings.Contains(string(invocation), "--push") {
		t.Fatal("missing dual-platform publication")
	}
}
