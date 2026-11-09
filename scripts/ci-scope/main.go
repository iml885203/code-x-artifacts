// Command ci-scope selects conservative pipeline gates and cache writes.
package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Only reviewed prose paths may omit product gates. New paths default to full.
var documentation = map[string]struct{}{
	"DESIGN.md": {},
	".agents/skills/implement-issue/SKILL.md": {},
	"AGENTS.md":                           {},
	"PRODUCT.md":                          {},
	"README.md":                           {},
	"docs/README.md":                      {},
	"docs/acceptance-beta.md":             {},
	"docs/acceptance-ime.md":              {},
	"docs/adding-a-kind.md":               {},
	"docs/agent-activity.md":              {},
	"docs/artifact-types.md":              {},
	"docs/design-format.md":               {},
	"docs/design-research.md":             {},
	"docs/design-room.md":                 {},
	"docs/design-source-edit.md":          {},
	"docs/doc-docx.md":                    {},
	"docs/doc-edit.md":                    {},
	"docs/doc-format.md":                  {},
	"docs/doc-room.md":                    {},
	"docs/editor.md":                      {},
	"docs/goal.md":                        {},
	"docs/large-publish.md":               {},
	"docs/multi-file.md":                  {},
	"docs/page-downloads.md":              {},
	"docs/realtime.md":                    {},
	"docs/research/claude-artifacts.md":   {},
	"docs/runtime.md":                     {},
	"docs/script-src.md":                  {},
	"docs/slides-pptx.md":                 {},
	"docs/spikes/design-editor.md":        {},
	"docs/spikes/react-kind.md":           {},
	"docs/spikes/realtime.md":             {},
	"docs/spikes/slides-format.md":        {},
	"docs/test-strategy.md":               {},
	"docs/viewers.md":                     {},
	"e2e/AGENTS.md":                       {},
	"internal/artifactguard/AGENTS.md":    {},
	"internal/assets/AGENTS.md":           {},
	"internal/auth/AGENTS.md":             {},
	"internal/claudeimport/AGENTS.md":     {},
	"internal/designfmt/AGENTS.md":        {},
	"internal/designroom/AGENTS.md":       {},
	"internal/designsourceedit/AGENTS.md": {},
	"internal/docedit/AGENTS.md":          {},
	"internal/docfmt/AGENTS.md":           {},
	"internal/docroom/AGENTS.md":          {},
	"internal/docx/AGENTS.md":             {},
	"internal/files/AGENTS.md":            {},
	"internal/i18n/AGENTS.md":             {},
	"internal/kind/AGENTS.md":             {},
	"internal/mcptools/AGENTS.md":         {},
	"internal/pptx/AGENTS.md":             {},
	"internal/realtime/AGENTS.md":         {},
	"internal/roomkind/AGENTS.md":         {},
	"internal/service/AGENTS.md":          {},
	"internal/store/AGENTS.md":            {},
	"internal/web/AGENTS.md":              {},
	"runtime/AGENTS.md":                   {},
	"web/AGENTS.md":                       {},
	"web/src/design/AGENTS.md":            {},
	"web/src/design/comments/AGENTS.md":   {},
	"web/src/editor/AGENTS.md":            {},
	"web/src/gallery/AGENTS.md":           {},
	"docs/ci.md":                          {},
}

func classify(source string, paths []string) string {
	if source != "push" || len(paths) == 0 {
		return "full"
	}
	for _, path := range paths {
		if _, ok := documentation[path]; !ok {
			return "full"
		}
	}
	return "docs"
}

func cachePolicy(source, branch, defaultBranch string, paths []string) string {
	if branch != defaultBranch || branch == "" {
		return "pull"
	}
	if source == "schedule" {
		return "pull-push"
	}
	if source != "push" {
		return "pull"
	}
	for _, path := range paths {
		switch path {
		case "go.mod", "go.sum", ".node-version", "Makefile", "scripts/install-node.sh", "scripts/install-golangci.sh",
			"web/package-lock.json", "runtime/package-lock.json", "e2e/package-lock.json", "internal/realtime/interop/package-lock.json",
			"internal/docroom/interop/package-lock.json", "internal/designroom/interop/package-lock.json":
			return "pull-push"
		}
	}
	return "pull"
}

func changedPaths(base, head string) []string {
	if len(base) != 40 || strings.Trim(base, "0") == "" || head == "" {
		return nil
	}
	if _, err := hex.DecodeString(base); err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// --no-renames includes both sides; moving prose into runtime cannot hide it.
	//nolint:gosec // Fixed Git command, a validated base hash, and a CI commit revision; no shell is invoked.
	out, err := exec.CommandContext(ctx, "git", "diff", "--no-renames", "--name-only", "-z", base, head, "--").Output()
	if err != nil {
		return nil
	} // A shallow clone without the base must run full.
	if len(out) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
}

// Refresh at the first main push across a UTC commit-day boundary, without a scheduler.
func firstPushOfDay(base, head string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	//nolint:gosec // Git reads CI commit revisions as argv, without a shell.
	out, err := exec.CommandContext(ctx, "git", "show", "-s", "--format=%ct", base, head, "--").Output()
	if err != nil {
		return false
	}
	stamps := strings.Fields(string(out))
	if len(stamps) != 2 {
		return false
	}
	previous, err := strconv.ParseInt(stamps[0], 10, 64)
	if err != nil {
		return false
	}
	current, err := strconv.ParseInt(stamps[1], 10, 64)
	return err == nil && current/86400 > previous/86400
}

func main() {
	paths := changedPaths(os.Getenv("CI_COMMIT_BEFORE_SHA"), os.Getenv("CI_COMMIT_SHA"))
	scope := classify(os.Getenv("CI_PIPELINE_SOURCE"), paths)
	policy := cachePolicy(os.Getenv("CI_PIPELINE_SOURCE"), os.Getenv("CI_COMMIT_BRANCH"), os.Getenv("CI_DEFAULT_BRANCH"), paths)
	if policy == "pull" && os.Getenv("CI_PIPELINE_SOURCE") == "push" && os.Getenv("CI_COMMIT_BRANCH") != "" && os.Getenv("CI_COMMIT_BRANCH") == os.Getenv("CI_DEFAULT_BRANCH") && firstPushOfDay(os.Getenv("CI_COMMIT_BEFORE_SHA"), os.Getenv("CI_COMMIT_SHA")) {
		policy = "pull-push"
	}
	slog.Info("selected CI gates", "scope", scope, "cache", policy, "changed_paths", len(paths))
	if _, err := fmt.Fprintf(os.Stdout, "include:\n  - local: .gitlab-ci.yml\nvariables:\n  PICTOR_SCOPE: %s\n  PICTOR_CACHE_POLICY: %s\n", scope, policy); err != nil {
		slog.Error("write child configuration", "error", err)
		os.Exit(1)
	}
}
