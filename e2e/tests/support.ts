import { expect, test as base, type Locator, type Page } from "@playwright/test";

// Console errors the pages are known to log, each tracked by an issue. A new
// error is a failure; an entry leaves this list in the commit that fixes its issue.
const knownConsoleErrors: { issue: string; matches: RegExp }[] = [];

function isKnown(text: string): boolean {
  return knownConsoleErrors.some((k) => k.matches.test(text));
}

// Tests run in parallel against one server, so a card on the overview can be for
// an artifact another test has just made private or deleted: its preview then
// answers 404, as it would for any reader. Only /raw/ previews on the overview.
// A preview's message can reach the listener after the page has moved on to an artifact (a slow machine), and the
// artifact it opened is one the overview had a card for: so once a page has been on the overview, a /raw/ message
// is let through whichever artifact it names, and so is the overview's list request.
function isOverviewPreview(text: string, resource: string, page: string, wasOnOverview: boolean): boolean {
  const resourcePath = new URL(resource || page).pathname;
  // The overview's own list request, cut off by the navigation away from it, is logged as ERR_FAILED the same way.
  const lateOverview = wasOnOverview && (resourcePath.startsWith("/raw/") || resourcePath === "/api/artifacts");
  if (new URL(page).pathname !== "/" && !lateOverview) return false;
  // A card runs the artifact's own script, so what that script logs (a page that fetches the API from its opaque origin is refused by CORS) is its author's,
  // not the site's. The test that is about the cards reads their frames itself.
  if (/\/raw\/[^/?]+\?preview$/.test(new URL(resource || page).pathname + new URL(resource || page).search)) return true;
  // The browser's second line for that refusal names the refused address, not the frame.
  if (text.startsWith("Failed to load resource: net::ERR_FAILED") && new URL(resource || page).pathname.startsWith("/api/")) return true;
  return text.startsWith("Failed to load resource: the server responded with a status of 404") && /\/raw\/[^/]+$/.test(new URL(resource || page).pathname);
}

interface Fixtures {
  /** A test that provokes a refusal on purpose says so: the browser logs the refused request as a console error. */
  expectConsoleErrors: (pattern: RegExp) => void;
  /** A page the test opens itself (another person's browser) is held to the same rule as `page`. */
  watch: (other: Page) => Page;
}

/**
 * Every test fails on a console error or an uncaught exception from its page or
 * any frame in it, unless it is one of knownConsoleErrors.
 */
interface Collector {
  seen: string[];
  expected: RegExp[];
  watch: (page: Page) => void;
}

export const test = base.extend<Fixtures & { consoleErrors: Collector; consoleRule: void }>({
  consoleErrors: async ({}, use) => {
    const seen: string[] = [];
    await use({
      seen,
      expected: [],
      watch: (page) => {
        let wasOnOverview = false;
        page.on("framenavigated", (f) => {
          if (f === page.mainFrame() && new URL(f.url()).pathname === "/") wasOnOverview = true;
        });
        page.on("console", (m) => {
          if (m.type() !== "error") return;
          const resource = m.location().url;
          if (!isOverviewPreview(m.text(), resource, page.url(), wasOnOverview || new URL(page.url()).pathname === "/")) seen.push(resource ? `${m.text()} (${resource})` : m.text());
        });
        page.on("pageerror", (e) => {
          seen.push(`uncaught: ${e.message}`);
        });
      },
    });
  },
  expectConsoleErrors: async ({ consoleErrors }, use) => {
    await use((pattern) => {
      consoleErrors.expected.push(pattern);
    });
  },
  watch: async ({ consoleErrors }, use) => {
    await use((other) => {
      consoleErrors.watch(other);
      return other;
    });
  },
  consoleRule: [
    async ({ page, consoleErrors }, use) => {
      consoleErrors.watch(page);
      await use();
      const unexpected = consoleErrors.seen.filter((e) => !isKnown(e) && !consoleErrors.expected.some((p) => p.test(e)));
      expect(unexpected, "console errors").toEqual([]);
    },
    // Not asked for by name, so no test can forget the check.
    { auto: true },
  ],
});

export { expect };

export const people = { alice: "pw-a", bob: "pw-b", carol: "pw-c" } as const;
export type Person = keyof typeof people;

/** Signs in through the sign-in form, as a person does. */
export async function signIn(page: Page, who: Person): Promise<void> {
  await page.goto("/login");
  await page.getByLabel("Username").fill(who);
  await page.getByLabel("Password").fill(people[who]);
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("heading", { name: "Artifacts" })).toBeVisible();
}

/** The files that come with a page, by relative path; null removes one. */
export type Files = Record<string, string | null>;

export interface Published {
  id: string;
  version: number;
}

/**
 * The calls go from inside the page, the way the front end makes them: the
 * session cookie is Secure, so only the browser (which treats localhost as a
 * secure origin) sends it over the smoke server's plain http.
 */
async function call(page: Page, method: string, path: string, body?: unknown): Promise<{ status: number; text: string }> {
  return page.evaluate(
    async ({ method, path, body }) => {
      const isBytes = Array.isArray(body);
      const res = await fetch(path, {
        method,
        credentials: "same-origin",
        headers: { "X-Pictor": "1", ...(body === undefined || isBytes ? {} : { "Content-Type": "application/json" }) },
        ...(body === undefined ? {} : { body: isBytes ? new Uint8Array(body as number[]) : JSON.stringify(body) }),
      });
      return { status: res.status, text: await res.text() };
    },
    { method, path, body },
  );
}

async function expectStatus(res: { status: number; text: string }, ...ok: number[]): Promise<void> {
  expect(ok, res.text).toContain(res.status);
}

/** Publishes through the same JSON API the viewer uses, with the page's session. */
export async function publish(page: Page, body: { title: string; kind?: string; content: string; visibility?: string; files?: Files }): Promise<Published> {
  const res = await call(page, "POST", "/api/artifacts", body);
  await expectStatus(res, 201);
  return JSON.parse(res.text) as Published;
}

export async function republish(page: Page, id: string, body: { title: string; kind?: string; content: string; baseVersion: number; sourceBases?: Record<string, string>; files?: Files }): Promise<Published> {
  const res = await call(page, "PUT", `/api/artifacts/${id}`, body);
  await expectStatus(res, 200);
  return JSON.parse(res.text) as Published;
}

export async function upload(page: Page, id: string, name: string, bytes: Buffer): Promise<void> {
  await expectStatus(await call(page, "PUT", `/api/artifacts/${id}/assets/${name}`, [...bytes]), 200, 201);
}

export async function visibilityOf(page: Page, id: string): Promise<string> {
  const res = await call(page, "GET", `/api/artifacts/${id}`);
  await expectStatus(res, 200);
  return (JSON.parse(res.text) as { visibility: string }).visibility;
}

/** A 1x1 PNG. */
export const png = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4//8/AAX+Av6nNYGEAAAAAElFTkSuQmCC", "base64");

/** The author names editors, as the viewer's Editors panel does. */
export async function nameEditors(page: Page, id: string, add: string[]): Promise<void> {
  await expectStatus(await call(page, "PATCH", `/api/artifacts/${id}/editors`, { add }), 200);
}

/** A personal token for the signed-in person, as the Connect page makes one. */
export async function personalToken(page: Page): Promise<string> {
  const res = await call(page, "POST", "/api/tokens", { name: "e2e" });
  await expectStatus(res, 201);
  return (JSON.parse(res.text) as { token: string }).token;
}

/** Calls an MCP tool the way an agent does: JSON-RPC over HTTP with a personal token. */
export async function mcpTool(origin: string, token: string, name: string, args: Record<string, unknown>, client?: string): Promise<{ isError: boolean; structured: Record<string, unknown>; text: string }> {
  const res = await fetch(`${origin}/api/mcp`, {
    method: "POST",
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json", Accept: "application/json, text/event-stream" },
    body: JSON.stringify({ jsonrpc: "2.0", id: 1, method: "tools/call", params: { name, arguments: args, ...(client === undefined ? {} : { _meta: { "io.modelcontextprotocol/clientInfo": { name: client, version: "1" } } }) } }),
  });
  expect(res.status).toBe(200);
  const body = (await res.json()) as { result: { isError?: boolean; structuredContent?: Record<string, unknown>; content?: { text?: string }[] } };
  return { isError: body.result.isError === true, structured: body.result.structuredContent ?? {}, text: (body.result.content ?? []).map((c) => c.text ?? "").join("") };
}

/**
 * Where an element of a frame is on screen: the frame's own box on the page, and the element's box inside
 * the frame scaled by how much smaller the frame is drawn than it lays out. Playwright's own
 * `boundingBox()` of an element in a scaled frame is the on-screen box in Chromium 141 and the
 * unscaled one in Chrome 155, so the tests do not rely on it.
 */
export async function onScreen(host: Locator, inner: Locator): Promise<{ x: number; y: number; width: number; height: number }> {
  const outer = await host.boundingBox();
  if (!outer) throw new Error("the frame is not on screen");
  const m = await inner.evaluate((el) => {
    const r = el.getBoundingClientRect();
    return { x: r.x, y: r.y, width: r.width, height: r.height, layout: el.ownerDocument.defaultView?.innerWidth ?? 1 };
  });
  const k = outer.width / m.layout;
  return { x: outer.x + m.x * k, y: outer.y + m.y * k, width: m.width * k, height: m.height * k };
}

/**
 * Waits until a modal has been presented: its entrance animation has finished,
 * so the browser has drawn it. A click before that can be routed to a sandboxed
 * frame behind it (a frame is its own process, and until the dialog's frame is
 * drawn the browser sends the click there), and the dialog never sees it.
 */
export async function presented(modal: Locator): Promise<void> {
  await modal.evaluate((d) => Promise.all(d.getAnimations().map((a) => a.finished)));
}

/** Picks a version in the version panel, opening the group it is folded into when it is not the newest of its group. */
export async function pickVersion(panel: Locator, version: number): Promise<void> {
  await panel.getByRole("button", { name: /^v\d/ }).first().waitFor();
  const row = panel.getByRole("button", { name: new RegExp(`^v${String(version)}`) });
  if (!(await row.isVisible())) await panel.getByRole("button", { name: /earlier$/ }).click();
  await row.click();
}
