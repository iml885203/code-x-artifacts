import type { Page } from "@playwright/test";
import { expect, nameEditors, publish, republish, signIn, test } from "./support";

// An open viewer follows the artifact: it polls for the newest version (every
// ten seconds) and shows it without anyone reloading.
const pollSlack = 25_000;

const frameOf = (page: Page) => page.frameLocator("iframe[data-artifact-frame]");
const origin = (page: Page) => page.url().split("/").slice(0, 3).join("/");

/** A page tall enough to scroll, saying which version it is. */
const tall = (word: string) => `<!doctype html><html><head><title>${word}</title></head><body style="margin:0"><h1>${word}</h1><div style="height:4000px"></div><p>bottom of ${word}</p></body></html>`;

test("bob's open viewer shows alice's new version without a reload, and stays where he was reading", async ({ page, browser, watch }) => {
  test.setTimeout(60_000);
  await signIn(page, "alice");
  const a = await publish(page, { title: "Weekly notes", content: tall("one"), visibility: "organisation" });

  const bobContext = await browser.newContext({ baseURL: origin(page) });
  const bob = watch(await bobContext.newPage());
  await signIn(bob, "bob");
  await bob.goto(`/a/${a.id}`);
  await expect(frameOf(bob).locator("h1")).toHaveText("one");
  await frameOf(bob).locator("body").evaluate(() => {
    window.scrollTo(0, 900);
  });
  await expect.poll(() => frameOf(bob).locator("body").evaluate(() => window.scrollY)).toBe(900);

  const frameBefore = await bob.locator("iframe[data-artifact-frame]").elementHandle();
  await republish(page, a.id, { title: "Weekly notes", content: tall("two"), baseVersion: a.version });

  // No navigation, no reload: the same document, a new frame.
  await expect(frameOf(bob).locator("h1")).toHaveText("two", { timeout: pollSlack });
  expect(await frameBefore?.evaluate((el) => el.isConnected)).toBe(false);
  await expect(bob.getByText("v2")).toBeVisible();
  await expect.poll(() => frameOf(bob).locator("body").evaluate(() => window.scrollY)).toBe(900);
  await bobContext.close();
});

test("an editor's new version reaches the author's open viewer", async ({ page, browser, watch }) => {
  test.setTimeout(60_000);
  await signIn(page, "alice");
  const a = await publish(page, { title: "Shared notes", content: tall("one") });
  await nameEditors(page, a.id, ["bob"]);
  await page.goto(`/a/${a.id}`);
  await expect(frameOf(page).locator("h1")).toHaveText("one");

  const bobContext = await browser.newContext({ baseURL: origin(page) });
  const bob = watch(await bobContext.newPage());
  await signIn(bob, "bob");
  await republish(bob, a.id, { title: "Shared notes", content: tall("by bob"), baseVersion: a.version });

  await expect(frameOf(page).locator("h1")).toHaveText("by bob", { timeout: pollSlack });
  await expect(page.getByText("v2")).toBeVisible();
  await bobContext.close();
});

// Who has the page open is shown at the top right: each sees the other, and a person who closes the
// tab is gone after the server's expiry (three seconds in the smoke server, a minute in production).
const goneSlack = 20_000;

test("alice and bob see each other at the top right of an html page, and bob is gone soon after he closes it", async ({ page, browser, watch }) => {
  test.setTimeout(60_000);
  await signIn(page, "alice");
  const a = await publish(page, { title: "Who is here", content: tall("one"), visibility: "organisation" });
  await page.goto(`/a/${a.id}`);
  const aliceSees = page.getByRole("banner").getByRole("group", { name: "Viewing now" });

  const bobContext = await browser.newContext({ baseURL: origin(page) });
  const bob = watch(await bobContext.newPage());
  await signIn(bob, "bob");
  await bob.goto(`/a/${a.id}`);
  const bobSees = bob.getByRole("banner").getByRole("group", { name: "Viewing now" });

  await expect(bobSees.getByText(/alice/i).first()).toBeVisible();
  await expect(aliceSees.getByText(/bob/i).first()).toBeVisible({ timeout: pollSlack });

  await bobContext.close();
  await expect(aliceSees.getByText(/bob/i)).toHaveCount(0, { timeout: goneSlack });
  await expect(aliceSees.getByText(/alice/i).first()).toBeVisible();
});
