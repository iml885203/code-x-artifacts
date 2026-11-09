import type { Page } from "@playwright/test";
import { originOf } from "./editor-support";
import { expect, mcpTool, personalToken, publish, signIn, test } from "./support";

// Pages and frames through the real socket: structure is the room's edit, so a
// reload, Undo and the saved design.json all agree with it.

const manifest = JSON.stringify({ version: 1, pages: [{ id: "home", name: "Home", rows: [[{ id: "hero", name: "Hero", file: "frames/hero.html", w: 800, h: 600 }]] }] });
const hero = `<!doctype html><html><head><meta charset="utf-8"></head><body style="margin:0"><p id="t" style="margin:20px">Hello</p></body></html>`;

async function open(page: Page, id: string) {
  await page.goto(`/a/${id}`);
  await expect(page.frameLocator('.dframe iframe[title^="Hero,"]').locator("#t")).toBeVisible();
  await expect(page.locator(".canvas-presence .ed-conn-live")).toBeAttached();
  await expect(page.locator(".canvas-presence .ed-conn-live")).toBeHidden();
}

// Dimensions are edited in Properties; the canvas label stays quiet.
async function expectFrameSize(page: Page, name: string, size: string) {
  const toggle = page.locator(".canvas-bar-right").getByRole("button", { name: "Properties" });
  if (await toggle.getAttribute("aria-pressed") !== "true") await toggle.click();
  await page.getByRole("region", { name: "Artboards", exact: true }).getByRole("button", { name, exact: true }).click();
  const section = page.locator(".artboard-properties");
  const [w, h] = size.split("×").map(value => value.trim());
  await expect(section.getByRole("textbox", { name: "Width", exact: true })).toHaveValue(w ?? "");
  await expect(section.getByRole("textbox", { name: "Height", exact: true })).toHaveValue(h ?? "");
  await expect(page.getByTitle(`${name}, ${w ?? ""} by ${h ?? ""}`)).toBeAttached();
  await expect(page.locator(".dframe-size")).toHaveCount(0);
}

async function savedManifest(page: Page, id: string, until: (m: { pages: { name: string; rows: { id: string; file: string; w: number; h: number }[][] }[] }) => boolean) {
  const token = await personalToken(page);
  let last = { pages: [] as { name: string; rows: { id: string; file: string; w: number; h: number }[][] }[] };
  await expect
    .poll(async () => {
      const live = await mcpTool(originOf(page), token, "read", { id });
      const read = await mcpTool(originOf(page), token, "read", { id, version: live.structured["version"] });
      const content = String(read.structured["content"] ?? "{}");
      last = JSON.parse(content) as typeof last;
      if (!until(last)) return false;
      const paths = (read.structured["files"] as { path: string }[]).map((f) => f.path).sort();
      expect(paths).toEqual(last.pages.flatMap((p) => p.rows.flat().map((f) => f.file)).sort());
      return true;
    })
    .toBe(true);
  return last;
}

test("add a page from the Page menu, add a frame and draw one with the Frame tool, resize, reload; the saved design.json has them, and deleting a page asks once and Undo brings it back", async ({ page, expectConsoleErrors }) => {
  // A frame added and removed within a moment (the page is deleted, then Undo brings it back) can be asked for
  // after the server no longer holds it; the browser logs that 404, and the frame is drawn once it is served.
  expectConsoleErrors(/Failed to load resource.*404.*\/frames\/[^/]+\.html/);
  await signIn(page, "alice");
  const a = await publish(page, { title: "Structure", kind: "design", content: manifest, files: { "frames/hero.html": hero } });
  await open(page, a.id);

  await page.getByRole("button", { name: "Page: Home" }).click();
  await page.getByRole("menuitem", { name: "Add page" }).click();
  await expect(page.getByRole("button", { name: "Page: Page 2" })).toBeVisible();
  await expect(page.locator(".dframe-name", { hasText: "Artboard 2" })).toBeVisible();
  // The new frame is drawn from the room at once: the page is there before any version holds the frame.
  await expect(page.frameLocator('.dframe iframe[title^="Artboard 2,"]').locator("body")).toBeAttached();

  await page.locator(".canvas-bar-right").getByRole("button", { name: "Properties" }).click();
  await page.getByRole("button", { name: "Add artboard" }).click();
  await expect(page.locator(".dframe-name", { hasText: "Artboard 3" })).toBeVisible();
  await expectFrameSize(page, "Artboard 3", "1440 × 900");
  await page.getByRole("textbox", { name: "Width" }).fill("500");
  await page.keyboard.press("Enter");
  await expectFrameSize(page, "Artboard 3", "500 × 900");

  // Select the frame itself through its label, independently of its iframe's elements.
  const addedFrame = page.locator(".dframe", { has: page.locator(".dframe-name", { hasText: "Artboard 3" }) });
  const label = addedFrame.locator(".dframe-name");
  await expect(label).toBeVisible();
  await label.click();
  const focusAndSelection = await label.evaluate(element => ({
    focused: document.activeElement === element,
    activeTag: document.activeElement?.tagName,
    activeLabel: document.activeElement?.getAttribute("aria-label"),
    pressed: element.getAttribute("aria-pressed"),
    frameSelected: element.closest(".dframe")?.hasAttribute("data-frame-selected"),
  }));
  await test.info().attach("visible-artboard-label-before-delete", { body: JSON.stringify(focusAndSelection), contentType: "application/json" });
  await expect(label).toBeFocused();
  await expect(label).toHaveAttribute("aria-pressed", "true");
  await expect(addedFrame).toHaveAttribute("data-frame-selected", "");
  await page.keyboard.press("Delete");
  await expect(addedFrame).toHaveCount(0);
  await savedManifest(page, a.id, (m) => m.pages[1]?.rows.flat().length === 1);
  await page.keyboard.press("ControlOrMeta+z");
  await expect(addedFrame).toBeVisible();
  await expectFrameSize(page, "Artboard 3", "500 × 900");

  // The Frame tool draws a frame at the size dragged: at 100% a drag of 300 by 200 on the screen is that size.
  await page.getByRole("button", { name: /^Zoom \d+%/ }).click();
  await page.getByRole("menuitem", { name: "100%" }).click();
  await page.getByRole("button", { name: "Artboard", exact: true }).click();
  const surface = await page.locator(".frame-draw").boundingBox(); // page-level
  if (!surface) throw new Error("the Frame tool put nothing over the canvas");
  await page.mouse.move(surface.x + 60, surface.y + 120);
  await page.mouse.down();
  await page.mouse.move(surface.x + 360, surface.y + 320, { steps: 4 });
  await page.mouse.up();
  await expectFrameSize(page, "Artboard 4", "300 × 200");
  await expect(page.locator(".frame-draw")).toHaveCount(0);

  await page.reload();
  await expect(page.getByRole("button", { name: "Page: Page 2" })).toBeVisible();
  await expectFrameSize(page, "Artboard 3", "500 × 900");
  await expectFrameSize(page, "Artboard 4", "300 × 200");

  const saved = await savedManifest(page, a.id, (m) => m.pages.length === 2 && m.pages[1]?.rows.flat().some((f) => f.w === 500) === true && m.pages[1]?.rows.flat().some((f) => f.w === 300 && f.h === 200) === true);
  expect(saved.pages.map((p) => p.name)).toEqual(["Home", "Page 2"]);

  await page.getByRole("button", { name: "Page: Page 2" }).click();
  await page.getByRole("menuitem", { name: "More actions for Page 2" }).hover();
  await page.getByRole("menuitem", { name: "Delete page" }).click();
  await expect(page.getByRole("alertdialog")).toContainText("Page 2");
  await page.getByRole("alertdialog").getByRole("button", { name: "Delete" }).click();
  await expect(page.getByRole("button", { name: "Page: Home" })).toBeVisible();
  await page.locator(".canvas-viewport").click({ position: { x: 5, y: 5 } });
  await page.keyboard.press("ControlOrMeta+z");
  await page.getByRole("button", { name: "Page: Home" }).click();
  await expect(page.getByRole("menuitemradio", { name: "Page 2" })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("menu")).toHaveCount(0);

  // At phone width every tool of both bars is on the screen and nothing covers it: none is hidden past an edge.
  const close = page.getByRole("button", { name: "Close properties" });
  if (await close.isVisible()) await close.click();
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.getByRole("toolbar", { name: "Canvas" }).getByRole("button", { name: "Shape" })).toBeVisible();
  const hidden = await page.getByRole("toolbar").evaluateAll((bars) =>
    bars.flatMap((bar) => [...bar.querySelectorAll("button")]).filter((b) => {
      const r = b.getBoundingClientRect();
      const hit = document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2);
      return r.x < 0 || r.right > window.innerWidth || !hit || !b.contains(hit);
    }).map((b) => b.getAttribute("aria-label")),
  );
  expect(hidden).toEqual([]);
});
