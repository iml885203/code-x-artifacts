# Development

CodeXArtifacts uses Go, React and TypeScript, SQLite, and embedded artifact runtimes.
The supported first release runs as one server; a TypeScript backend and
Cloudflare-native hosting are outside its current scope.

## Build and run

Follow the [quick start](../README.md#quick-start). Go 1.27.1, Git and Make are required.
`make build` installs pinned Node 24.22.0 under `.tools/`, verifies its checksum,
and installs locked npm dependencies. Node is a build dependency, not a server runtime.

For optional frontend hot reload in front of the running Go server:

```sh
make web-dev
```

Keep `.local/` private. Existing lowercase `pictor` API names and `PICTOR_`
environment variables remain compatible; do not rename them as a branding change.

## Checks

Install the repository hooks once per clone, then run the full verification:

```sh
make hooks
GO_TEST_FLAGS=-p=2 VERIFY_BUDGET=240 make verify
```

The explicit budget accommodates local tool setup and race compilation. Bounded
Go package concurrency suits a shared development host; individual test-unit limits
remain enabled. Verification covers formatting, lint, contracts, frontend tests,
Go tests and build checks. Keep the normal commit and push hooks enabled.

Browser checks are separate:

```sh
make e2e-browser
make e2e
```

See [test strategy](test-strategy.md) and the scoped `AGENTS.md` files before
changing artifact contracts, authorization or sandbox behavior.

## Agent workflows

Repository skills are available under `.agents/skills` after cloning:

- [`$github-issue-create`](../.agents/skills/github-issue-create/SKILL.md) checks
  product fit and source evidence, then drafts/reviews a Scrum-style GitHub item.
- [`$github-issue-implement`](../.agents/skills/github-issue-implement/SKILL.md)
  implements a referenced issue using the project's checks and independent review.
  It adapts Orbit's workflow; publishing, merging or updating a runtime still
  requires authorization in the request/session.

For example: `Use $github-issue-implement to implement issue #<number> and open a PR.`
`make hooks` also creates `.claude/skills -> ../.agents/skills` for Claude Code,
without replacing an existing file/link. Codex can discover `.agents/skills`
directly; clients without skill discovery can read the linked `SKILL.md` files.

## Website

The public website is static HTML, CSS and JavaScript under `website/`:

```sh
python3 -m http.server 4176 --bind 127.0.0.1 --directory website
```

Open http://localhost:4176. Preserve `website/CNAME` when publishing. Product
screenshots use demonstration content and must match the actual interface.

## Website sharing and search metadata

`website/index.html` contains the canonical HTTPS URL, page summary, Open Graph,
X card metadata, and WebSite/WebPage JSON-LD. Keep these URLs, `website/CNAME`,
`robots.txt` and `sitemap.xml` aligned when moving domains. The sitemap lists the
single public landing page; section anchors are not separate pages.

The sharing card is `website/social/og.png` (1200×630 PNG). Its editable source
is `scripts/website-og.html`, using the existing brand mark, local font and actual
library screenshot. With the browser-test dependencies and Chrome available:

```sh
node scripts/render-website-og.mjs
```

The renderer also records the asset source in `website/social/og.png.json`.
Review the card before publishing; use demonstration content only.
Metadata and crawl files improve how the site is described; search engines and
sharing platforms choose when to refresh their cached copies.
