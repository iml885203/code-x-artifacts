import type { Locator, Page } from "@playwright/test";
import { browse, originOf } from "./editor-support";
import { expect, nameEditors, onScreen, publish, signIn, test } from "./support";

// Moving, resizing and deleting on the canvas, through the real socket: the edits
// are the room's, so they are live for everyone and survive a reload.

const manifest = JSON.stringify({ version: 1, pages: [{ id: "home", name: "Home", rows: [[{ id: "hero", name: "Hero", file: "frames/hero.html", w: 800, h: 600 }]] }] });
const box = (id: string, left: number, color: string) => `<div id="${id}" style="position:absolute;left:${String(left)}px;top:40px;width:100px;height:60px;background:${color}"><b>${id}</b></div>`;
const hero = `<!doctype html><html><head><meta charset="utf-8"></head><body style="margin:0">${box("a", 40, "#c33")}${box("b", 300, "#36c")}<p id="flow" style="margin:200px 40px;width:200px">In the flow</p></body></html>`;

const onHero = (page: Page) => page.frameLocator('.dframe iframe[title^="Hero,"]');
const heroFrame = (page: Page) => page.locator('.dframe iframe[title^="Hero,"]');
/** Where an element of the Hero frame is on screen (see onScreen). */
const screenBox = (page: Page, el: Locator) => onScreen(heroFrame(page), el);
const place = (el: Locator) => el.evaluate((n) => ({ left: n.style.left, top: n.style.top, width: n.style.width, height: n.style.height }));

async function zoomTo50(page: Page) {
  await page.keyboard.press("1");
  await page.keyboard.press("-");
  await page.keyboard.press("-");
  await expect(page.getByRole("button", { name: /^Zoom 50%/ })).toBeVisible();
}

async function open(page: Page, id: string) {
  await page.goto(`/a/${id}`);
  await expect(onHero(page).locator("#a")).toBeVisible();
  await zoomTo50(page);
}

async function select(page: Page, target: Locator) {
  const b = await screenBox(page, target);
  await page.mouse.click(b.x + b.width / 2, b.y + b.height / 2);
  await expect(page.locator("[data-selection]")).toHaveCount(1);
  return b;
}

async function drag(page: Page, from: { x: number; y: number }, dx: number, dy: number) {
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  await page.mouse.move(from.x + dx / 2, from.y + dy / 2, { steps: 5 });
  await page.mouse.move(from.x + dx, from.y + dy, { steps: 5 });
  await page.mouse.up();
}

test("at 50% zoom a drag of 30 by 15 on screen moves the element 60 by 30, one Undo puts it back, and a reload keeps the move", async ({ page }) => {
  await signIn(page, "alice");
  const a = await publish(page, { title: "Editing", kind: "design", content: manifest, files: { "frames/hero.html": hero } });
  await open(page, a.id);
  const el = onHero(page).locator("#a");
  const b = await select(page, el);
  await drag(page, { x: b.x + b.width / 2, y: b.y + b.height / 2 }, 30, 15);
  await expect.poll(() => place(el)).toMatchObject({ left: "100px", top: "70px", width: "100px", height: "60px" });
  // The selection follows the element.
  const after = await screenBox(page, el);
  const drawn = await page.locator("[data-selection]").boundingBox(); // page-level
  expect(drawn?.x).toBeCloseTo(after?.x ?? -1, 0);

  await page.keyboard.press("ControlOrMeta+z");
  await expect.poll(() => place(el)).toMatchObject({ left: "40px", top: "40px" });
  await page.keyboard.press("ControlOrMeta+Shift+z");
  await expect.poll(() => place(el)).toMatchObject({ left: "100px", top: "70px" });

  await page.reload();
  await expect(onHero(page).locator("#a")).toBeVisible();
  await expect.poll(() => place(onHero(page).locator("#a"))).toMatchObject({ left: "100px", top: "70px" });
});

test("the corner handles resize, an element in the flow only resizes and says why it cannot be dragged, and Delete is undone", async ({ page }) => {
  await signIn(page, "alice");
  const a = await publish(page, { title: "Editing", kind: "design", content: manifest, files: { "frames/hero.html": hero } });
  await open(page, a.id);
  const el = onHero(page).locator("#a");
  await select(page, el);
  const se = page.locator('[data-handle="se"]');
  const h = await se.boundingBox(); // page-level
  if (!h) throw new Error("no handle");
  await drag(page, { x: h.x + h.width / 2, y: h.y + h.height / 2 }, 20, 10);
  await expect.poll(() => place(el)).toMatchObject({ left: "40px", top: "40px", width: "140px", height: "80px" });

  const flow = onHero(page).locator("#flow");
  const f = await select(page, flow);
  await expect(page.locator('[data-handle]')).toHaveCount(1);
  await drag(page, { x: f.x + f.width / 2, y: f.y + f.height / 2 }, 40, 40);
  await expect(page.locator(".canvas-status")).toContainText("isn't absolutely positioned");
  expect(await flow.evaluate((n) => n.style.left)).toBe("");

  await select(page, el);
  await expect(page.locator("[data-handle]")).toHaveCount(4);
  await page.keyboard.press("Delete");
  await expect(el).toBeHidden();
  await page.keyboard.press("ControlOrMeta+z");
  await expect(el).toBeVisible();
  await expect.poll(() => place(el)).toMatchObject({ width: "140px", height: "80px" });
});

test("two browsers move different elements and both see both, and one's Undo leaves the other's move", async ({ page, browser, watch }) => {
  await signIn(page, "alice");
  const a = await publish(page, { title: "Together", kind: "design", content: manifest, files: { "frames/hero.html": hero } });
  await nameEditors(page, a.id, ["bob"]);
  const bob = await browse(browser, watch, originOf(page), "bob");
  await open(page, a.id);
  await open(bob.page, a.id);

  const mine = await select(page, onHero(page).locator("#a"));
  const theirs = await select(bob.page, onHero(bob.page).locator("#b"));
  await drag(page, { x: mine.x + mine.width / 2, y: mine.y + mine.height / 2 }, 30, 0);
  await drag(bob.page, { x: theirs.x + theirs.width / 2, y: theirs.y + theirs.height / 2 }, 0, 30);

  for (const seen of [page, bob.page]) {
    await expect.poll(() => place(onHero(seen).locator("#a"))).toMatchObject({ left: "100px", top: "40px" });
    await expect.poll(() => place(onHero(seen).locator("#b"))).toMatchObject({ left: "300px", top: "100px" });
  }
  await page.keyboard.press("ControlOrMeta+z");
  for (const seen of [page, bob.page]) {
    await expect.poll(() => place(onHero(seen).locator("#a"))).toMatchObject({ left: "40px" });
    await expect.poll(() => place(onHero(seen).locator("#b"))).toMatchObject({ top: "100px" });
  }
  await bob.context.close();
});
