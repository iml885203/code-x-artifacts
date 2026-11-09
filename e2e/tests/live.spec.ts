import type { Page } from "@playwright/test";
import { expect, publish, signIn, test } from "./support";

// What only a real browser shows about the live socket (docs/realtime.md): the
// Origin header it sends from the site's own page and from a sandboxed one,
// and the close codes a page can read.

/** Asks for a ticket the way the page does: the cookie and the bridge header. */
async function ticketFor(page: Page, id: string): Promise<string> {
  return page.evaluate(async (id) => {
    const res = await fetch(`/api/artifacts/${id}/live`, { method: "POST", credentials: "same-origin", headers: { "X-Pictor": "1" } });
    return ((await res.json()) as { ticket: string }).ticket;
  }, id);
}

/** Opens the socket from the current page and reports what happened: the first message's first byte, or why it did not open. */
async function openSocket(page: Page, id: string, ticket: string): Promise<{ opened: boolean; firstByte?: number | undefined; closeCode?: number | undefined }> {
  return page.evaluate(
    ({ id, ticket }) =>
      new Promise<{ opened: boolean; firstByte?: number | undefined; closeCode?: number | undefined }>((resolve) => {
        const ws = new WebSocket(`${location.origin.replace(/^http/, "ws")}/ws/${id}?ticket=${ticket}`);
        ws.binaryType = "arraybuffer";
        let opened = false;
        ws.onopen = () => {
          opened = true;
        };
        ws.onmessage = (e) => {
          resolve({ opened, firstByte: new Uint8Array(e.data as ArrayBuffer)[0] });
          ws.close();
        };
        ws.onclose = (e) => resolve({ opened, closeCode: e.code });
      }),
    { id, ticket },
  );
}

test("the site's own page opens its socket with a ticket, once", async ({ page, expectConsoleErrors }) => {
  // The second attempt is refused at the upgrade, which the browser logs.
  expectConsoleErrors(/WebSocket connection to .* failed/);
  await signIn(page, "alice");
  const a = await publish(page, { title: "Live", content: "<p>live", visibility: "organisation" });
  await page.goto(`/a/${a.id}`);

  const ticket = await ticketFor(page, a.id);
  // The room starts the sync: its first message is a sync message (type 0).
  expect(await openSocket(page, a.id, ticket)).toMatchObject({ opened: true, firstByte: 0 });
  expect((await openSocket(page, a.id, ticket)).opened).toBe(false);
});

test("a page in the sandbox cannot open the socket even with a ticket", async ({ page, expectConsoleErrors }) => {
  // The browser sends Origin: null from the sandbox and the upgrade is refused.
  expectConsoleErrors(/WebSocket connection to .* failed/);
  await signIn(page, "alice");
  const a = await publish(page, { title: "Sandboxed", content: "<p>sandboxed", visibility: "organisation" });
  await page.goto(`/a/${a.id}`);
  const ticket = await ticketFor(page, a.id);

  await page.goto(`/raw/${a.id}`);
  expect(await page.evaluate(() => self.origin)).toBe("null");
  expect((await openSocket(page, a.id, ticket)).opened).toBe(false);
});
