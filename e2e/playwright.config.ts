import { defineConfig } from "@playwright/test";

// The server under test is the smoke server (internal/web/smoke_test.go): the
// real handler with a fake directory (alice/pw-a, bob/pw-b). Locally Playwright
// starts it with `go test`; the pipeline starts a compiled test binary and says
// so in PICTOR_E2E_SERVER.
const port = process.env["PICTOR_E2E_PORT"] ?? "18080";
const addr = `127.0.0.1:${port}`;

export default defineConfig({
  testDir: "tests",
  // A test publishes its own artifacts and signs in as its own people, so tests
  // do not depend on each other and run side by side.
  fullyParallel: true,
  forbidOnly: !!process.env["CI"],
  retries: 0,
  // The pipeline's runner is small and Chrome is heavy: two workers keep each
  // test well inside its time instead of fourteen browsers starving each other.
  ...(process.env["CI"] ? { workers: 2, timeout: 60_000, expect: { timeout: 15_000 } } : {}),
  reporter: process.env["CI"] ? [["list"], ["html", { open: "never" }]] : "list",
  outputDir: "test-results",
  use: {
    baseURL: `http://${addr}`,
    // The system Chrome where the Playwright browsers are not installed.
    ...(process.env["PICTOR_E2E_CHANNEL"] ? { channel: process.env["PICTOR_E2E_CHANNEL"] } : {}),
    // A failure leaves what the pipeline needs to see it: a trace (with the
    // screenshots of every step) and a screenshot of the end.
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  webServer: {
    command: process.env["PICTOR_E2E_SERVER"] ?? "go -C .. test -count=1 -run Smoke -timeout 30m ./internal/web",
    // The two-minute pause before a room becomes a version, and the minute a person stays listed as viewing after leaving, are shortened so a test can wait for them.
    env: { PICTOR_SMOKE_ADDR: addr, PICTOR_SMOKE_ROOM_QUIET: "2s", PICTOR_SMOKE_VIEWER_STAYS: "3s" },
    url: `http://${addr}/healthz`,
    reuseExistingServer: false,
    timeout: 180_000,
  },
});
