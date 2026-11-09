import type { Locator, Page } from "@playwright/test";
import { browse, originOf } from "./editor-support";
import { expect, mcpTool, nameEditors, onScreen, personalToken, publish, signIn, test } from "./support";

// The outline panel through the real socket: reorder, rename and hide are the
// room's edits, so another person's edits and the saved file agree with them.

const manifest = JSON.stringify({ version: 1, pages: [{ id: "home", name: "Home", rows: [[{ id: "hero", name: "Hero", file: "frames/hero.html", w: 800, h: 600 }]] }] });
const box = (id: string, name: string, left: number, color: string) => `<div id="${id}" data-name="${name}" style="position:absolute;left:${String(left)}px;top:40px;width:100px;height:60px;background:${color}"></div>`;
const hero = `<!doctype html><html><head><meta charset="utf-8"></head><body style="margin:0">${box("a", "Red", 40, "#c33")}${box("b", "Blue", 300, "#36c")}<p id="flow" data-name="Flow" style="margin:200px 40px;width:200px">In the flow</p></body></html>`;

const onHero = (page: Page) => page.frameLocator('.dframe iframe[title^="Hero,"]');
const order = (page: Page) => onHero(page).locator("body > *").evaluateAll((els) => els.map((e) => e.id));
const rowOf = (page: Page, name: string): Locator => page.locator(".orow", { has: page.locator(".orow-label", { hasText: new RegExp(`^${name}$`) }) });

async function open(page: Page, id: string) {
  await page.goto(`/a/${id}`);
  await expect(onHero(page).locator("#a")).toBeVisible();
  await page.locator(".canvas-bar-right").getByRole("button", { name: "Properties" }).click();
  await expect(rowOf(page, "Red")).toBeVisible();
  await expect(page.locator(".canvas-presence .ed-conn-live")).toBeAttached();
  await expect(page.locator(".canvas-presence .ed-conn-live")).toBeHidden();
}

async function savedFrame(page: Page, id: string, until: (text: string) => boolean): Promise<string> {
  const token = await personalToken(page);
  let text = "";
  await expect
    .poll(async () => {
      const read = await mcpTool(originOf(page), token, "read", { id, file: "frames/hero.html" });
      text = ((read.structured["file"] as { text?: string } | undefined)?.text) ?? "";
      return until(text);
    })
    .toBe(true);
  return text;
}

test("alice reorders through the outline while bob moves an element; both survive, in both browsers, after a reload and in the saved file", async ({ page, browser, watch }) => {
  await signIn(page, "alice");
  const a = await publish(page, { title: "Outline", kind: "design", content: manifest, files: { "frames/hero.html": hero } });
  await nameEditors(page, a.id, ["bob"]);
  const bob = await browse(browser, watch, originOf(page), "bob");
  await open(page, a.id);
  await open(bob.page, a.id);
  expect(await order(page)).toEqual(["a", "b", "flow"]);

  // Bob moves the red box on the canvas (50% zoom, so 30 on screen is 60 in the frame).
  await bob.page.keyboard.press("1");
  await bob.page.keyboard.press("-");
  await bob.page.keyboard.press("-");
  const red = await onScreen(bob.page.locator('.dframe iframe[title^="Hero,"]'), onHero(bob.page).locator("#a"));
  await bob.page.mouse.click(red.x + red.width / 2, red.y + red.height / 2);
  await expect(bob.page.locator("[data-selection]")).toHaveCount(1);
  await bob.page.mouse.move(red.x + red.width / 2, red.y + red.height / 2);
  await bob.page.mouse.down();
  await bob.page.mouse.move(red.x + red.width / 2 + 30, red.y + red.height / 2, { steps: 8 });
  await bob.page.mouse.up();

  // Alice puts the flow paragraph first, by dragging its row above Red's.
  await rowOf(page, "Flow").dragTo(rowOf(page, "Red"), { targetPosition: { x: 40, y: 3 } });
  for (const seen of [page, bob.page]) {
    await expect.poll(() => order(seen)).toEqual(["flow", "a", "b"]);
    await expect.poll(() => onHero(seen).locator("#a").evaluate((n) => n.style.left)).toBe("100px");
  }

  await page.reload();
  await expect(onHero(page).locator("#a")).toBeVisible();
  await expect.poll(() => order(page)).toEqual(["flow", "a", "b"]);

  // An agent reading the file sees the new order and the move.
  const saved = await savedFrame(page, a.id, (text) => text.indexOf('id="flow"') >= 0 && text.indexOf('id="flow"') < text.indexOf('id="a"'));
  expect(saved).toContain("left:100px");
  await bob.context.close();
});

test("a rename changes only data-name, the eye hides an element that stays in the file, and Undo reverses each", async ({ page }) => {
  await signIn(page, "alice");
  const a = await publish(page, { title: "Names", kind: "design", content: manifest, files: { "frames/hero.html": hero } });
  await open(page, a.id);

  await rowOf(page, "Blue").locator(".orow-main").dblclick();
  await page.getByRole("textbox", { name: "Name of Blue" }).fill("Sky");
  await page.keyboard.press("Enter");
  await expect(rowOf(page, "Sky")).toBeVisible();

  await page.getByRole("button", { name: "Hide Sky" }).click();
  await expect(onHero(page).locator("#b")).toBeHidden();
  await expect(rowOf(page, "Sky")).toHaveClass(/is-hidden/);

  const saved = await savedFrame(page, a.id, (text) => text.includes('data-name="Sky"') && text.includes("display:none"));
  expect(saved).toContain('id="b"'); // hidden, still in the file
  expect(saved).toContain("In the flow");

  await page.getByRole("button", { name: "Show Sky" }).click();
  await expect(onHero(page).locator("#b")).toBeVisible();
  await page.keyboard.press("ControlOrMeta+z"); // un-show
  await page.keyboard.press("ControlOrMeta+z"); // un-hide
  await page.keyboard.press("ControlOrMeta+z"); // un-rename
  await expect(rowOf(page, "Blue")).toBeVisible();
});
