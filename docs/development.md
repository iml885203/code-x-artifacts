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

## Library image thumbnails

Gallery and Recently opened cards use account-scoped JPEG thumbnails at a fixed
1120×720 image size (a 560×360 CSS viewport, rendered at 2×). A cold card queues an invisible opaque-origin preview in the
viewer's browser (at most two at once). The preview uses bundled html-to-image to
rasterize settled content, including canvas and supported nested Design frames;
it never gets the viewer's API bridge or session credentials. Successful uploads
are decoded and reencoded as JPEG by Go, then the temporary iframe is removed.
A warm card loads only an image, without running the artifact or loading its fonts.
No server browser, Node runtime or external screenshot service is required.

Images persist in `thumbnails/` next to `PICTOR_DB`. The cache is disposable:
200 MiB by default (`PICTOR_THUMBNAIL_CACHE_MB=1..4096`), at most 10,000 files and
1 MiB per uploaded image. Least-recently-used files are evicted to maintain bounds;
old revisions are replaced for each account/artifact/theme/language variant.
There are at most four variants (light/dark × English/Traditional Chinese), with
no viewport or device-pixel-ratio variants. Capture times out after 20 seconds,
and the outer queue releases/removes a stuck document after 30 seconds.

Saved version, relevant asset hashes, visibility, render script and frontend build,
request origin, account and theme/language participate in identity. Hour boundaries
and expired embedded grants do not invalidate pixels. Generation/upload/delivery
recheck current access; image responses are `no-store`, not public CDN objects.
Already delivered pixels cannot be recalled. Persisted thumbnails contain no live
resource URLs. A cache miss or failure shows the card's lightweight placeholder;
opening the full artifact still works and retains normal scrolling/interactivity.
Video, shadow DOM, scrolled internal containers, oversized nested frames (>2000px),
busy or excessively complex content may be unsupported. Font/image CORS or browser
SVG/canvas restrictions can also prevent capture; failed captures are not saved.
Focus retries a missing preview after a one-minute cooldown; a reload retries too.

The previous process-local inert HTML preview endpoints remain available for older
clients, with their existing 128-entry / 32 MiB / next-hour expiry limits. Current
cards do not use them. Changes to server-only rendering that do not affect the
capture script or frontend must bump the thumbnail renderer namespace in
`thumbnail_routes.go` so persisted images are regenerated.
