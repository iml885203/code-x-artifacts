import type { Locator, Page } from "@playwright/test";
import { browse, originOf } from "./editor-support";
import { expect, nameEditors, onScreen, publish, signIn, test } from "./support";

// The properties panel, through the real socket: what a control writes is the
// room's, so it is in the frame at once, in a second browser, and in the file.

const manifest = JSON.stringify({ version: 1, tokens: { "--accent": "#336699" }, pages: [{ id: "home", name: "Home", rows: [[{ id: "hero", name: "Hero", file: "frames/hero.html", w: 800, h: 600 }, { id: "second", name: "Second", file: "frames/second.html", w: 400, h: 300 }]] }] });
const hero =
  `<!doctype html><html><head><meta charset="utf-8"><style>.t{font-size:20px}</style></head><body style="margin:0">` +
  `<div id="a" style="position:absolute;left:40px;top:40px;width:100px;height:60px;background:#cc3333"><b>a</b></div>` +
  `<h1 id="title" class="t" style="margin:0;margin-top:120px;color:#112233">Title</h1>` +
  `<p id="flow" style="margin:200px 40px;width:200px">In the flow</p>` +
  `<div id="stack" style="position:absolute;left:300px;top:300px;display:flex;width:180px;height:140px">` +
  `<div style="flex:0 0 100px;height:30px;background:#ddd">First</div><div style="flex:0 0 100px;height:30px;background:#bbb">Second</div></div></body></html>`;

const onHero = (page: Page) => page.frameLocator('.dframe iframe[title^="Hero,"]');
const heroFrame = (page: Page) => page.locator('.dframe iframe[title^="Hero,"]');

async function open(page: Page, id: string) {
  await page.goto(`/a/${id}`);
  await expect(onHero(page).locator("#a")).toBeVisible();
}

async function select(page: Page, target: Locator) {
  const b = await onScreen(heroFrame(page), target);
  await page.mouse.click(b.x + b.width / 2, b.y + b.height / 2);
  await expect(page.locator("[data-selection]")).toHaveCount(1);
  const properties = page.locator(".canvas-bar-right").getByRole("button", { name: "Properties", exact: true });
  if (await properties.getAttribute("aria-pressed") !== "true") await properties.click();
  await expect(page.getByRole("tab", { name: "Design" })).toHaveAttribute("aria-selected", "true");
}

async function enter(page: Page, label: string, text: string) {
  const field = page.getByRole("textbox", { name: label, exact: true });
  await field.fill(text);
  await field.press("Enter");
}

test("alice puts an element in the flow keeping its offset and gives it a translucent fill, and changes a token across two frames for both live clients", async ({ page, browser, watch }) => {
  await signIn(page, "alice");
  const a = await publish(page, { title: "Properties", kind: "design", content: manifest, files: { "frames/hero.html": hero, "frames/second.html": `<div id="token-second" style="background-color:var(--accent);width:100px;height:60px">Shared</div>` } });
  await nameEditors(page, a.id, ["bob"]);
  const bob = await browse(browser, watch, originOf(page), "bob");
  await open(page, a.id);
  await open(bob.page, a.id);
  const mine = onHero(page).locator("#a");
  const theirs = onHero(bob.page).locator("#a");
  const flow = onHero(bob.page).locator("#flow");
  const before = await flow.evaluate((n) => n.getBoundingClientRect().top);
  await select(page, mine);
  await page.getByRole("button", { name: "Relative" }).click();
  // It takes room in the flow (the paragraph moves down by its height) and keeps its offset.
  await expect.poll(() => flow.evaluate((n) => n.getBoundingClientRect().top)).toBeCloseTo(before + 60, 0);
  expect(await theirs.evaluate((n) => [getComputedStyle(n).position, n.style.left, n.style.top])).toEqual(["relative", "40px", "40px"]);
  await enter(page, "Fill, hex", "#336699");
  await enter(page, "Fill, Alpha (%)", "50");
  await expect.poll(() => theirs.evaluate((n) => getComputedStyle(n).backgroundColor)).toBe("rgba(51, 102, 153, 0.5)");
  await page.getByRole("button", { name: "Row", exact: true }).click();
  await page.getByRole("combobox", { name: "Wrap", exact: true }).selectOption("wrap-reverse");
  await page.getByRole("combobox", { name: "Border sides", exact: true }).selectOption("top");
  await enter(page, "Border width", "3");
  await expect.poll(() => theirs.evaluate(n => [getComputedStyle(n).flexWrap, getComputedStyle(n).borderTopWidth])).toEqual(["wrap-reverse", "3px"]);
  await select(page, onHero(page).locator("#title"));
  await select(page, mine);
  await expect(page.getByRole("textbox", { name: "Fill, Alpha (%)", exact: true })).toHaveValue("50");
  await expect(page.getByRole("combobox", { name: "Wrap", exact: true })).toHaveValue("wrap-reverse");
  await page.getByRole("combobox", { name: "Border sides", exact: true }).selectOption("top");
  await expect(page.getByRole("textbox", { name: "Border width", exact: true })).toHaveValue("3");
  // Choosing a token saves the binding, and changing its shared value reaches
  // both sandbox frames and the other live browser without reloading.
  await page.getByRole("button", { name: "Fill picker", exact: true }).click();
  const picker = page.getByRole("dialog", { name: "Fill", exact: true });
  await expect(picker.getByRole("tab", { name: "Theme", exact: true })).toHaveAttribute("aria-selected", "true");
  await picker.getByRole("button", { name: /--accent/ }).click();
  await page.keyboard.press("Escape");
  await expect.poll(() => theirs.evaluate((n) => n.style.backgroundColor)).toBe("var(--accent)");
  await enter(page, "--accent", "#224466");
  for (const client of [page, bob.page]) {
    await expect.poll(() => onHero(client).locator("#a").evaluate((n) => getComputedStyle(n).backgroundColor)).toBe("rgb(34, 68, 102)");
    await expect.poll(() => client.frameLocator('.dframe iframe[title^="Second,"]').locator("#token-second").evaluate((n) => getComputedStyle(n).backgroundColor)).toBe("rgb(34, 68, 102)");
  }
  // Play stays on the same frame while the other editor changes an unsaved token.
  await bob.page.getByRole("button", { name: "Expand Second", exact: true }).click();
  const played = bob.page.frameLocator('[role="dialog"] iframe[title="Second"]').locator("#token-second");
  await expect.poll(() => played.evaluate((n) => getComputedStyle(n).backgroundColor)).toBe("rgb(34, 68, 102)");
  await enter(page, "--accent", "#335577");
  await expect.poll(() => played.evaluate((n) => getComputedStyle(n).backgroundColor)).toBe("rgb(51, 85, 119)");
  await enter(page, "--accent", "#224466");
  await expect.poll(() => played.evaluate((n) => getComputedStyle(n).backgroundColor)).toBe("rgb(34, 68, 102)");
  await bob.page.keyboard.press("Escape");
  const stack = onHero(page).locator("#stack");
  const stackLayout = (target: Locator) => target.evaluate(n => {
    const parent = n.getBoundingClientRect();
    return Array.from(n.children, child => {
      const box = child.getBoundingClientRect();
      return [Math.round(box.left - parent.left), Math.round(box.top - parent.top)];
    });
  });
  await expect.poll(() => stackLayout(stack)).toEqual([[0, 0], [100, 0]]);
  await select(page, stack);
  await expect(page.locator("[data-selection]")).toHaveAttribute("data-selection", (await stack.getAttribute("data-pid"))!);
  await expect(page.getByRole("textbox", { name: "Width", exact: true })).toHaveValue("180");
  await expect(page.getByRole("combobox", { name: "Wrap", exact: true })).toHaveValue("nowrap");
  await page.getByRole("combobox", { name: "Wrap", exact: true }).selectOption("wrap");
  // The constrained Stack must visibly put its second child on a new row.
  await expect.poll(() => stackLayout(stack)).toEqual([[0, 0], [0, 70]]);
  await expect.poll(() => stackLayout(onHero(bob.page).locator("#stack"))).toEqual([[0, 0], [0, 70]]);
  await page.reload();
  await expect.poll(() => stackLayout(stack)).toEqual([[0, 0], [0, 70]]);
  await expect.poll(() => onHero(page).locator("#a").evaluate((n) => getComputedStyle(n).backgroundColor)).toBe("rgb(34, 68, 102)");
  await expect.poll(() => mine.evaluate(n => [n.style.backgroundColor, getComputedStyle(n).flexWrap, getComputedStyle(n).borderTopWidth])).toEqual(["var(--accent)", "wrap-reverse", "3px"]);
  await select(page, mine);
  await expect(page.getByRole("combobox", { name: "Wrap", exact: true })).toHaveValue("wrap-reverse");
  await page.getByRole("combobox", { name: "Border sides", exact: true }).selectOption("top");
  await expect(page.getByRole("textbox", { name: "Border width", exact: true })).toHaveValue("3");
  await bob.context.close();
});
