import type { Browser, BrowserContext, Locator, Page } from "@playwright/test";
import { agentOf, browse, openEditor, originOf } from "./editor-support";
import { expect, nameEditors, onScreen, publish, signIn, test } from "./support";

// Other people's selections on the canvas, through the real socket: a box in the
// person's colour with their name, from the name the server knows. An agent that
// publishes a changed frame into the open design shows the change live, under its
// own name, and the mark fades. A signed-out visitor of the public design, or of a public doc, is a guest in the
// same stack as the room's people, each of them once, and sees itself as the guest in the phone's More menu.

const manifest = JSON.stringify({ version: 1, pages: [{ id: "home", name: "Home", rows: [[{ id: "hero", name: "Hero", file: "frames/hero.html", w: 800, h: 600 }]] }] });
const hero = `<!doctype html><html><head><meta charset="utf-8"></head><body style="margin:0"><h1 id="title" style="position:absolute;left:40px;top:40px;margin:0">Title</h1><p id="body" style="position:absolute;left:40px;top:200px;margin:0">Body</p></body></html>`;
const onHero = (page: Page) => page.frameLocator('.dframe iframe[title^="Hero,"]');

async function open(page: Page, id: string) {
  await page.goto(`/a/${id}`);
  await expect(onHero(page).locator("#title")).toBeVisible();
  await expect(page.locator(".canvas-presence .ed-conn-live")).toBeAttached();
  await expect(page.locator(".canvas-presence .ed-conn-live")).toBeHidden();
}

/** Clicks the middle of an element of the Hero frame. */
async function click(page: Page, target: Locator) {
  const b = await onScreen(page.locator('.dframe iframe[title^="Hero,"]'), target);
  await page.mouse.click(b.x + b.width / 2, b.y + b.height / 2);
}

/** A signed-out visitor on a phone: it reads the page and finds itself, as a guest, in the More menu. */
async function guestOnPhone(browser: Browser, watch: (page: Page) => Page, origin: string, id: string): Promise<{ context: BrowserContext; page: Page }> {
  const context = await browser.newContext({ baseURL: origin, viewport: { width: 390, height: 844 } });
  const visitor = watch(await context.newPage());
  await visitor.goto(`/a/${id}`);
  await visitor.getByRole("button", { name: "More actions" }).click();
  await expect(visitor.getByRole("region", { name: "Viewing now" }).getByText(/ \(You\) · Guest$/)).toHaveCount(1);
  await visitor.keyboard.press("Escape");
  return { context, page: visitor };
}

const stackOf = (page: Page) => page.getByRole("banner").getByRole("group", { name: "Viewing now" });

test("bob sees a box with ALICE's name over the title she selected, an agent's published change shows live under its name and fades, and guests join the stack", async ({ page, browser, watch }) => {
  test.setTimeout(90_000);
  await signIn(page, "alice");
  const a = await publish(page, { title: "Presence", kind: "design", visibility: "public", content: manifest, files: { "frames/hero.html": hero } });
  await nameEditors(page, a.id, ["bob"]);
  const bob = await browse(browser, watch, originOf(page), "bob");
  await open(page, a.id);
  await open(bob.page, a.id);
  await expect(bob.page.getByRole("group", { name: "Viewing now" })).toContainText("ALICE");

  const designGuest = await guestOnPhone(browser, watch, originOf(page), a.id);
  await expect(designGuest.page.locator(".dframe").first()).toBeVisible();
  for (const [seen, other] of [[page, "BOB"], [bob.page, "ALICE"]] as const) {
    await expect(stackOf(seen).getByRole("button", { name: / \(Guest\)$/ })).toHaveCount(1);
    await expect(stackOf(seen).getByRole("button", { name: other, exact: true })).toHaveCount(1);
    await expect(stackOf(seen).getByRole("button", { name: /\(You\)$/ })).toHaveCount(1);
  }
  await designGuest.context.close();

  await click(page, onHero(page).locator("#title"));
  await expect(page.locator("[data-selection]")).toHaveCount(1);
  const theirs = bob.page.locator("[data-peer] .peer-tag");
  await expect(theirs).toHaveText("ALICE");
  // The box is over the title, as alice's own selection is on her screen.
  // Each screen has its own size and zoom (alice's properties panel narrows her canvas), so the box is placed
  // within the frame it is over, as a fraction of the frame, on both screens.
  const within = async (p: Page, box: string) => {
    const at = await p.locator(box).boundingBox(); // page-level
    const frame = await p.locator(".dframe").first().boundingBox(); // page-level
    return { x: ((at?.x ?? 0) - (frame?.x ?? 0)) / (frame?.width ?? 1), y: ((at?.y ?? 0) - (frame?.y ?? 0)) / (frame?.height ?? 1) };
  };
  const mine = await within(page, "[data-selection]");
  const seen = await within(bob.page, "[data-peer]");
  expect(seen.x).toBeCloseTo(mine.x, 2);
  expect(seen.y).toBeCloseTo(mine.y, 2);
  // Bob's own selection is not drawn as someone else's on his screen.
  await expect(page.locator("[data-peer]")).toHaveCount(0);

  await page.keyboard.press("Escape");
  await expect(bob.page.locator("[data-peer]")).toHaveCount(0);

  // An agent reads the frame as the open design holds it and publishes it moved: both browsers see it without a reload.
  const agent = await agentOf(page, "claude-code");
  const read = await agent("read", { id: a.id, file: "frames/hero.html" });
  expect(read.isError, read.text).toBe(false);
  const text = (read.structured["file"] as { text: string }).text;
  const moved = await agent("publish", {
    id: a.id, title: "Presence", content: manifest, baseVersion: read.structured["version"], sourceBases: read.structured["sourceBases"], files: { "frames/hero.html": text.replace("left:40px", "left:120px") },
  });
  expect(moved.isError, moved.text).toBe(false);
  expect(moved.structured["live"]).toBe(true);
  for (const seen of [page, bob.page]) {
    await expect(onHero(seen).locator("#title")).toHaveCSS("left", "120px");
    await expect(seen.locator(".peerbox-agent .peer-tag")).toHaveText("alice 的 Claude Code");
  }
  // The mark fades after a few seconds.
  for (const seen of [page, bob.page]) await expect(seen.locator(".peerbox-agent")).toHaveCount(0, { timeout: 10_000 });
  await bob.context.close();

  const doc = await publish(page, { title: "Presence notes", kind: "doc", visibility: "public", content: "# Notes\n\nRead me.\n" });
  await openEditor(page, doc.id);
  const docGuest = await guestOnPhone(browser, watch, originOf(page), doc.id);
  await expect(docGuest.page.frameLocator("iframe[data-artifact-frame]").locator("body")).toContainText("Read me.");
  await expect(stackOf(page).getByRole("button", { name: / \(Guest\)$/ })).toHaveCount(1);
  await expect(stackOf(page).getByRole("button")).toHaveCount(2);
  await docGuest.context.close();
});
