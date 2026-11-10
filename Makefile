# `make verify` is the definition of done. Adapted from an independent local verification harness.
#
# Every quality property is enforced here or in the pipeline, never by asking
# for care: if a rule matters and no target checks it, it is not enforced.

GOLANGCI_VERSION := v2.14.0
GOLANGCI_BIN     := $(CURDIR)/.tools/golangci-lint

# The front end (web/) runs on the Node in .node-version, installed into
# .tools/node on first use; NODE_HOME points elsewhere for a Node installed
# some other way, which must be the same version (verify-web-tools checks).
NODE_VERSION := $(shell cat .node-version)
NODE_HOME    ?= $(CURDIR)/.tools/node
NPM          := PATH="$(NODE_HOME)/bin:$$PATH" npm --prefix web
# The embedded runtime (runtime/) is built with the same Node; its output is
# committed under internal/jsruntime/v1 (docs/runtime.md).
NPM_RUNTIME  := PATH="$(NODE_HOME)/bin:$$PATH" npm --prefix runtime
NODE         := $(NODE_HOME)/bin/node
# The browser tests (e2e/), also on that Node; see the e2e target.
NPM_E2E      := PATH="$(NODE_HOME)/bin:$$PATH" npm --prefix e2e
PLAYWRIGHT_INSTALL_ARGS ?=

# Runs per Go test. Tests run in a random order (-shuffle) so one that depends
# on another fails the first time, not on someone else's change. -count=2 would
# also catch state that survives a run, but TestReactRenderIsMemoisedByContentAndRuntime
# fails on the second pass (its compile counter reads a process-wide memo), so
# CI stays at 1 until that test owns its cache.
TEST_COUNT ?= 1

# Extra flags for `go test` (CI sets -p=1: see .gitlab-ci.yml).
GO_TEST_FLAGS ?=

# Environment for `go test` only (CI puts the tests' scratch files in memory: see .gitlab-ci.yml).
TEST_ENV ?=

# A command that wraps the Go tests and the front end's tests (CI prints the CPU throttling during them).
TEST_WRAP ?=

.PHONY: help
help:
	@echo "make web            build the front end into internal/web/dist (embedded by the Go build)"
	@echo "make web-dev        Vite dev server for the front end, in front of a Go server on :8080"
	@echo "make runtime        rebuild the embedded libraries (internal/jsruntime/v1) from runtime/; commit the result"
	@echo "make inspector       rebuild internal/kind/design_inspector.js from internal/kind/inspector/; commit the result"
	@echo "make e2e-tools      install the browser tests' dependencies (e2e/)"
	@echo "make e2e            run the browser tests against the smoke server (not part of verify)"
	@echo "make hooks          once per clone or worktree: point git at .githooks and install local Git hooks"
	@echo "make build          build bin/codexartifacts"
	@echo "make verify-fast    static checks + tests, no time limit"
	@echo "make verify         definition of done"

# Pinned tools install themselves: the first step that needs one fetches it,
# and every later run finds it in .tools/ or node_modules/. There is no setup
# step to forget; `make hooks` is the only manual one.
#
# npm ci installs exactly what package-lock.json says and fails when
# package.json disagrees with it. It leaves node_modules/.package-lock.json,
# which is how a directory's install is told to be current: $(call npm_ci,dir,npm command)
define npm_ci
@if [ -f $(1)/node_modules/.package-lock.json ] && ! [ $(1)/package-lock.json -nt $(1)/node_modules/.package-lock.json ]; then :; else $(2) ci; fi
endef

# The real yjs library, used only to produce and check the fixtures of the
# realtime hub's interop tests (docs/realtime.md).
INTEROP := internal/realtime/interop
NPM_INTEROP := PATH="$(NODE_HOME)/bin:$$PATH" npm --prefix $(INTEROP)

# The editor's document schema and the fixtures that tie it to the Go bridge of
# a doc in a room (docs/doc-room.md).
DOCROOM := internal/docroom/interop
NPM_DOCROOM := PATH="$(NODE_HOME)/bin:$$PATH" npm --prefix $(DOCROOM)

# The formatted text of a design in a room, as yjs writes it and as Go writes it
# (docs/design-room.md).
DESIGNROOM := internal/designroom/interop
NPM_DESIGNROOM := PATH="$(NODE_HOME)/bin:$$PATH" npm --prefix $(DESIGNROOM)

# The browser tests (e2e/) are their own job in the pipeline, not part of
# verify: Chrome and the dependencies stay out of the 60 second budget.
.PHONY: e2e-tools
e2e-tools: node-tools
	$(call npm_ci,e2e,$(NPM_E2E))

# Downloads the Chrome the tests drive; the pipeline adds --with-deps for the
# system libraries it needs. A machine that has Chrome can set
# PICTOR_E2E_CHANNEL=chrome instead and skip this.
.PHONY: e2e-browser
e2e-browser: e2e-tools
	cd e2e && PATH="$(NODE_HOME)/bin:$$PATH" npx playwright install $(PLAYWRIGHT_INSTALL_ARGS) chromium

# Installs Google Chrome (the stable channel people use) for the Chrome run of
# the tests; PLAYWRIGHT_INSTALL_ARGS=--with-deps adds its system libraries.
.PHONY: e2e-chrome-browser
e2e-chrome-browser: e2e-tools
	cd e2e && PATH="$(NODE_HOME)/bin:$$PATH" npx playwright install $(PLAYWRIGHT_INSTALL_ARGS) chrome

# The same tests in the installed Google Chrome instead of the bundled Chromium.
.PHONY: e2e-chrome
e2e-chrome:
	$(MAKE) e2e PICTOR_E2E_CHANNEL=chrome

# The tests start the smoke server themselves (e2e/playwright.config.ts) on
# PICTOR_E2E_PORT (default 18080) and fail on any console error that is not
# tracked by an issue.
.PHONY: e2e
e2e: web e2e-tools
	cd e2e && PATH="$(NODE_HOME)/bin:$$PATH" npx tsc --noEmit
	cd e2e && PATH="$(NODE_HOME)/bin:$$PATH" npx playwright test

# The one manual step after a clone, and after creating a worktree: .git/hooks
# is per clone, so a hook kept there would exist only on whichever machine
# installed it. Every checkout opts into the same committed hook scripts.
.PHONY: hooks
hooks:
	@if [ ! -e .claude/skills ] && [ ! -L .claude/skills ]; then mkdir -p .claude; ln -s ../.agents/skills .claude/skills; fi
	@git config core.hooksPath .githooks
	@echo "hooks installed: pre-commit runs verify-precommit, pre-push runs verify-prepush"

# The front end is built into internal/web/dist, which the Go build embeds.
# Nothing built is committed, so every Go build and test goes through here.
.PHONY: web
web: verify-web-tools
	$(NPM) run build

.PHONY: web-dev
web-dev: verify-web-tools
	$(NPM) run dev

# Rebuilds the libraries pages import. The result is committed, so a change to
# runtime/ or its lockfile shows up as a diff of the files that ship.
.PHONY: runtime
runtime: verify-runtime-tools
	$(NPM_RUNTIME) run build

.PHONY: build
build: web
	go build -o bin/codexartifacts ./cmd/codexartifacts

# golangci-lint is not a module dependency, so the first step that needs it
# fetches the pinned, checksum-verified release.
.PHONY: verify-tools
verify-tools:
	@if ! { [ -x "$(GOLANGCI_BIN)" ] && \
	    "$(GOLANGCI_BIN)" --version 2>/dev/null | grep -q "$(patsubst v%,%,$(GOLANGCI_VERSION))"; }; then \
		scripts/install-golangci.sh $(GOLANGCI_VERSION) "$(CURDIR)/.tools"; \
	fi

# Node at the version in .node-version, checksum-verified by install-node.sh.
.PHONY: node-tools
node-tools:
	@if [ "$(NODE_HOME)" != "$(CURDIR)/.tools/node" ]; then \
		[ "$$("$(NODE_HOME)/bin/node" --version 2>/dev/null)" = "v$(NODE_VERSION)" ] || { echo "node-tools: $(NODE_HOME) is not node v$(NODE_VERSION), the version in .node-version"; exit 1; }; \
	elif ! { [ -x "$(NODE_HOME)/bin/node" ] && [ "$$("$(NODE_HOME)/bin/node" --version)" = "v$(NODE_VERSION)" ]; }; then \
		scripts/install-node.sh $(NODE_VERSION) "$(dir $(NODE_HOME))"; \
	fi

.PHONY: verify-web-tools
verify-web-tools: node-tools
	$(call npm_ci,web,$(NPM))
	@grep -q "^FROM .*node:$(NODE_VERSION)-" Dockerfile || { echo "verify-web-tools: the Dockerfile's node stage is not node:$(NODE_VERSION), the version in .node-version"; exit 1; }

.PHONY: verify-runtime-tools
verify-runtime-tools: node-tools
	$(call npm_ci,runtime,$(NPM_RUNTIME))
	@grep -q "^FROM .*node:$(NODE_VERSION)-.* AS runtime" Dockerfile || { echo "verify-runtime-tools: the Dockerfile's runtime stage is not node:$(NODE_VERSION), the version in .node-version"; exit 1; }

# The embedded runtime is exactly what runtime/ builds from its lockfile: a
# fresh build (under half a second, so it is never skipped) must equal the
# committed files byte for byte. A changed dependency, a hand-edited file or a
# stale build fails here.
.PHONY: verify-runtime
verify-runtime: verify-runtime-tools
	$(NPM_RUNTIME) run typecheck
	$(NPM_RUNTIME) run check

# The inspector in every design frame is built from the modules in
# internal/kind/inspector (comments stay in the modules); the built file is
# committed because Go embeds it. It is built with the esbuild runtime/ pins.
.PHONY: inspector
inspector: verify-runtime-tools
	$(NODE) internal/kind/inspector/build.mjs

# The committed inspector is exactly what its modules build.
.PHONY: verify-inspector
verify-inspector: verify-runtime-tools
	$(NODE) internal/kind/inspector/build.mjs --check

# The Go room and the real yjs library agree in both directions (and, for a doc in
# a room, the Go bridge and the editor's schema): the fixtures
# the Go tests apply are exactly what yjs produces from the lockfile (a fresh
# run must equal the committed files), and every update the Go room produced
# for the committed Go fixtures applies in yjs and builds what Go says it does.
.PHONY: verify-interop
verify-interop: verify-runtime-tools
	$(call npm_ci,$(INTEROP),$(NPM_INTEROP))
	@rm -rf .cache/interop && mkdir -p .cache/interop
	$(NODE) $(INTEROP)/generate.mjs .cache/interop
	@diff -r .cache/interop internal/realtime/testdata/yjs || { echo "verify-interop: the committed yjs fixtures differ from what yjs produces; run '$(NODE) $(INTEROP)/generate.mjs internal/realtime/testdata/yjs' and commit"; exit 1; }
	$(NODE) $(INTEROP)/check.mjs internal/realtime/testdata/go
	$(call npm_ci,$(DOCROOM),$(NPM_DOCROOM))
	@rm -rf .cache/docroom-interop && mkdir -p .cache/docroom-interop
	$(NODE) $(DOCROOM)/generate.mjs .cache/docroom-interop
	@diff -r .cache/docroom-interop internal/docroom/testdata/yjs || { echo "verify-interop: the committed editor fixtures differ from what the editor schema produces; run '$(NODE) $(DOCROOM)/generate.mjs internal/docroom/testdata/yjs' and commit"; exit 1; }
	$(NODE) $(DOCROOM)/check.mjs internal/docroom/testdata/go
	$(call npm_ci,$(DESIGNROOM),$(NPM_DESIGNROOM))
	@rm -rf .cache/designroom-interop && mkdir -p .cache/designroom-interop
	$(NODE) $(DESIGNROOM)/generate.mjs .cache/designroom-interop
	@diff -r .cache/designroom-interop internal/designroom/testdata/yjs || { echo "verify-interop: the committed design fixtures differ from what yjs produces; run '$(NODE) $(DESIGNROOM)/generate.mjs internal/designroom/testdata/yjs' and commit"; exit 1; }
	$(NODE) $(DESIGNROOM)/check.mjs internal/designroom/testdata/go

# Type check and lint the front end: what a reviewer would catch, caught here.
.PHONY: verify-web
verify-web: verify-web-tools
	$(NPM) run typecheck
	$(NODE) --test scripts/lint-web.test.mjs
	$(NODE) scripts/lint-web.mjs

.PHONY: verify-fmt
verify-fmt: verify-tools
	@out="$$($(GOLANGCI_BIN) fmt --diff)"; \
	if [ -n "$$out" ]; then echo "$$out"; echo "verify-fmt: run '$(GOLANGCI_BIN) fmt'"; exit 1; fi

.PHONY: verify-lint
verify-lint: verify-tools
	go vet ./...
	$(GOLANGCI_BIN) run

.PHONY: verify-docs
verify-docs:
	@scripts/verify-docs.sh

# A source file over 600 lines is where parallel changes collide. Existing ones
# are listed, with their exact size, in .size-allowlist; the list only shrinks.
.PHONY: verify-size
verify-size:
	@scripts/verify-size.sh '*.go' '*.ts' '*.tsx' ':!*_test.go' ':!*.test.ts' ':!*.test.tsx' ':!*.spec.ts'

# Every test named on a "Tests:" line of an AGENTS.md exists, and the root
# AGENTS.md stays within 120 lines (area detail goes in that directory's own).
# The self-test first proves a renamed test makes the check fail.
.PHONY: verify-agents-md
verify-agents-md:
	@scripts/verify-agents-md-selftest.sh
	@scripts/verify-agents-md.sh

# The browser tests only grow through a one-line, reviewed raise of .e2e-budget
# (docs/test-strategy.md); lowering it is always allowed.
.PHONY: verify-e2e-budget
verify-e2e-budget:
	@scripts/verify-e2e-budget.sh

# No committed symlinks, pinned tools, scratch files or dependencies.
.PHONY: verify-e2e-boxes
verify-e2e-boxes:
	@scripts/verify-e2e-boxes.sh

.PHONY: verify-tracked
verify-tracked:
	@scripts/verify-tracked.sh bin node_modules

.PHONY: verify-mod
verify-mod:
	@go mod tidy -diff || { echo "verify-mod: run 'go mod tidy'"; exit 1; }

# The race detector sleeps a second when each test binary exits; there is
# nothing to flush here, so skip it (it would be 1s on every package).
# verify-lint runs the full go vet suite, including test sources. Running the
# smaller default vet suite again with race builds duplicates that analysis;
# test-unit owns test execution, while verify-lint owns static analysis.
.PHONY: test-unit
test-unit: web
	@mkdir -p .cache/go-tmp
	$(TEST_WRAP) env GORACE=atexit_sleep_ms=0 $(TEST_ENV) go test $(GO_TEST_FLAGS) -vet=off -v -race -shuffle=on -count=$(TEST_COUNT) ./...

# The front end's component and unit tests.
.PHONY: test-web
test-web: verify-web-tools
	$(TEST_WRAP) sh -c '$(NPM) run test'

# Local hooks run these checks; keep them enabled. `make verify` is the
# final completion gate, including when remote CI is not configured.
.PHONY: verify-precommit
verify-precommit: verify-fmt verify-lint verify-mod verify-docs verify-size verify-tracked verify-e2e-boxes verify-e2e-budget verify-agents-md verify-web verify-runtime verify-inspector verify-interop

.PHONY: verify-prepush
verify-prepush: build test-web test-unit

.PHONY: verify-fast
verify-fast: verify-precommit verify-prepush

# The definition of done: the same steps as verify-fast, held to the time
# budget in scripts/verify-budget.sh.

.PHONY: verify
verify:
	@scripts/verify-budget.sh

# Called by verify-budget.sh after every run: the slowest single tests in the
# captured output LOG, read from the "--- PASS: TestX (1.23s)" lines (go test -v).
# A test's time excludes waiting for a parallel slot and its subtests report their
# own, so a package of many parallel tests is never one slow unit.
.PHONY: verify-slowest
verify-slowest:
	@awk '$$1 == "---" && $$NF ~ /^\([0-9.]+s\)$$/ { t = $$NF; gsub(/[()s]/, "", t); print t, $$3 }' "$(LOG)" | sort -rn | head -5
