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

## Website

The public website is static HTML, CSS and JavaScript under `website/`:

```sh
python3 -m http.server 4176 --bind 127.0.0.1 --directory website
```

Open http://localhost:4176. Preserve `website/CNAME` when publishing. Product
screenshots use demonstration content and must match the actual interface.
