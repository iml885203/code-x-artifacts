package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOnlyKnownDocumentationChangesCanSkipProductGates(t *testing.T) {
	tests := []struct{ name, source, paths, want string }{
		{"docs", "push", "docs/goal.md\x00README.md\x00", "docs"},
		{"project harness", "push", "AGENTS.md\x00docs/ci.md\x00.agents/skills/implement-issue/SKILL.md\x00", "docs"},
		{"mixed", "push", "docs/goal.md\x00cmd/codexartifacts/main.go\x00", "full"},
		{"frontend", "push", "web/src/App.tsx\x00", "full"},
		{"backend", "push", "internal/service/access.go\x00", "full"},
		{"embedded markdown", "push", "internal/kind/doc_guide.md\x00", "full"},
		{"fixture", "push", "docs/fixtures/example.md\x00", "full"},
		{"template", "push", "internal/web/templates/viewer.html\x00", "full"},
		{"CI", "push", ".gitlab-ci.yml\x00", "full"},
		{"dependency", "push", "web/package-lock.json\x00", "full"},
		{"unknown markdown", "push", "docs/new-runtime.md\x00", "full"},
		{"empty", "push", "", "full"},
		{"initial", "push", "", "full"},
		{"web", "web", "README.md\x00", "full"},
		{"schedule", "schedule", "README.md\x00", "full"},
		{"MR", "merge_request_event", "README.md\x00", "full"},
		{"rename out", "push", "README.md\x00internal/kind/new.md\x00", "full"},
		{"newline", "push", "docs/goal.md\nunknown\x00", "full"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classify(tt.source, strings.Split(strings.TrimSuffix(tt.paths, "\x00"), "\x00"))
			if got != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestCacheWritesAreBoundedToDependenciesAndRefresh(t *testing.T) {
	for _, tt := range []struct{ source, branch, paths, want string }{
		{"push", "main", "internal/service/access.go", "pull"},
		{"push", "main", "go.sum", "pull-push"},
		{"push", "main", ".node-version", "pull-push"},
		{"push", "main", "e2e/package-lock.json", "pull-push"},
		{"push", "feature", "go.sum", "pull"},
		{"schedule", "main", "", "pull-push"},
		{"web", "main", "", "pull"},
	} {
		if got := cachePolicy(tt.source, tt.branch, "main", []string{tt.paths}); got != tt.want {
			t.Errorf("%+v: got %s", tt, got)
		}
	}
}

func TestMissingPushRangeRunsFullWithoutARefreshWrite(t *testing.T) {
	for _, base := range []string{"", strings.Repeat("0", 40), strings.Repeat("f", 40)} {
		paths := changedPaths(base, "HEAD")
		if classify("push", paths) != "full" || cachePolicy("push", "main", "main", paths) != "pull" {
			t.Fatal("an unavailable range must select full verification and read-only cache")
		}
	}
}

func TestSourceCachesRefreshOnTheFirstPushOfANewUTCDay(t *testing.T) {
	root, err := filepath.Abs(".tmp")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", root)
	repo := t.TempDir()
	git := func(date string, args ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		//nolint:gosec // Test-only Git argv in an owned temporary repository, without a shell.
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
		for _, variable := range os.Environ() {
			if !strings.HasPrefix(variable, "GIT_") {
				cmd.Env = append(cmd.Env, variable)
			}
		}
		cmd.Env = append(cmd.Env, "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
		out, runErr := cmd.CombinedOutput()
		if runErr != nil {
			t.Fatalf("git %v: %v: %s", args, runErr, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("2026-10-20T01:00:00Z", "init", "-q")
	commit := func(date string) string {
		git(date, "-c", "user.name=CI scope test", "-c", "user.email=scope@example.invalid", "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-qm", "fixture")
		return git(date, "rev-parse", "HEAD")
	}
	base := commit("2026-10-20T01:00:00Z")
	sameDay := commit("2026-10-20T23:59:00Z")
	nextDay := commit("2026-10-21T00:01:00Z")
	for _, variable := range os.Environ() {
		key, _, _ := strings.Cut(variable, "=")
		if strings.HasPrefix(key, "GIT_") {
			t.Setenv(key, "")
			if err := os.Unsetenv(key); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Chdir(repo)
	if firstPushOfDay(base, sameDay) {
		t.Fatal("same UTC day must not repeat an archive")
	}
	if !firstPushOfDay(sameDay, nextDay) {
		t.Fatal("first push of the next UTC day must refresh compiled source")
	}
	if firstPushOfDay(strings.Repeat("0", 40), nextDay) {
		t.Fatal("unknown history must not claim a daily boundary")
	}
	if got := classify("push", changedPaths(base, nextDay)); got != "full" {
		t.Fatalf("empty commit range must run full, got %s", got)
	}
}
