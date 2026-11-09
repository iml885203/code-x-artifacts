import type { Locator, Page } from "@playwright/test";
import { expect, onScreen, publish, signIn, test } from "./support";
import { browse, originOf } from "./editor-support";

// What a person adds to a design and what they do to the elements they have: the
// copy and the new shapes are the room's, so they are live, survive a reload (the
// frame is the last saved file and the room is what builds the rest) and are
// undone one action at a time.

// A picture a browser can decode (support.ts's `png` is only bytes to upload).
const dot = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADElEQVR4nGM4Y2wMAAMBATPEwnsTAAAAAElFTkSuQmCC", "base64");

const manifest = JSON.stringify({ version: 1, pages: [{ id: "home", name: "Home", rows: [[{ id: "hero", name: "Hero", file: "frames/hero.html", w: 800, h: 600 }]] }] });
const hero = `<!doctype html><html><head><meta charset="utf-8"></head><body style="margin:0"><div style="position:absolute;left:40px;top:40px;width:100px;height:60px;background:#c33"><b>card</b></div><h1 style="margin:200px 0 0 40px">Hello</h1></body></html>`;

const onHero = (page: Page) => page.frameLocator('.dframe iframe[title^="Hero,"]');
const heroFrame = (page: Page) => page.locator('.dframe iframe[title^="Hero,"]');
const place = (el: Locator) => el.evaluate((n) => ({ left: n.style.left, top: n.style.top, width: n.style.width, height: n.style.height }));

async function open(page: Page, id: string) {
  await page.goto(`/a/${id}`);
  await expect(onHero(page).locator("h1")).toBeVisible();
  await page.keyboard.press("1");
  await page.keyboard.press("-");
  await page.keyboard.press("-");
  await expect(page.getByRole("button", { name: /^Zoom 50%/ })).toBeVisible();
}

async function select(page: Page, target: Locator) {
  const b = await onScreen(heroFrame(page), target);
  await page.mouse.click(b.x + b.width / 2, b.y + b.height / 2);
  await expect(page.locator("[data-selection]")).toHaveCount(1);
  await expect(page.locator("[data-selection]")).toHaveAttribute("data-selection", (await target.getAttribute("data-pid")) ?? "");
  return b;
}

/** Clicks a point of the Hero frame, in the frame's own pixels. */
async function clickFrame(page: Page, x: number, y: number) {
  const f = await heroFrame(page).boundingBox(); // page-level
  if (!f) throw new Error("no frame");
  const k = f.width / 800;
  await page.mouse.click(f.x + x * k, f.y + y * k);
}

test("Cmd+D copies an element, the copy moves on its own and Undo puts it back, and a reload keeps both", async ({ page }) => {
  await signIn(page, "alice");
  const a = await publish(page, { title: "Copying", kind: "design", content: manifest, files: { "frames/hero.html": hero } });
  await open(page, a.id);
  const cards = onHero(page).locator("div:has(> b)");
  const b = await select(page, cards.first());
  await page.keyboard.press("ControlOrMeta+d");
  await expect(cards).toHaveCount(2);
  await expect.poll(() => place(cards.nth(1))).toMatchObject({ left: "56px", top: "56px", width: "100px" });
  expect(await place(cards.first())).toMatchObject({ left: "40px", top: "40px" });

  // The copy is what is selected: dragging it moves it and not the original.
  await page.mouse.move(b.x + b.width / 2 + 8, b.y + b.height / 2 + 8);
  await page.mouse.down();
  await page.mouse.move(b.x + b.width / 2 + 38, b.y + b.height / 2 + 23, { steps: 6 });
  await page.mouse.up();
  await expect.poll(() => place(cards.nth(1))).toMatchObject({ left: "116px", top: "86px" });
  expect(await place(cards.first())).toMatchObject({ left: "40px", top: "40px" });

  await page.keyboard.press("ControlOrMeta+z");
  await expect.poll(() => place(cards.nth(1))).toMatchObject({ left: "56px", top: "56px" });

  await page.reload();
  await expect(onHero(page).locator("h1")).toBeVisible();
  await expect(cards).toHaveCount(2);
  await expect.poll(() => place(cards.nth(1))).toMatchObject({ left: "56px", top: "56px" });
  expect(await place(cards.first())).toMatchObject({ left: "40px", top: "40px" });
});

test("text, shapes, an image and a pen stroke are inserted, the stroke moves, resizes and is restored by Undo, and a reload and reader keep the vectors", async ({ page, browser, watch }) => {
  await signIn(page, "alice");
  const a = await publish(page, { title: "Inserting", kind: "design", visibility: "organisation", content: manifest, files: { "frames/hero.html": hero } });
  await open(page, a.id);
  // A shape the inspector built is written by the browser (rgb); one the frame was served with is the room's own text.
  const shapes = onHero(page).locator("div[data-pid]:is([style*='background-color: rgb(217, 217, 217)'], [style*='background-color:#d9d9d9'])");

  await page.getByRole("button", { name: "Text" }).click();
  await clickFrame(page, 400, 300);
  await expect(onHero(page).getByText("Text", { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Shape", exact: true }).click();
  await page.getByRole("menuitem", { name: "Rectangle" }).click();
  await clickFrame(page, 100, 400);
  await expect(shapes).toHaveCount(1);
  const rectangle = shapes.first();
  await expect.poll(() => place(rectangle)).toMatchObject({ left: "100px", top: "400px", width: "160px", height: "120px" });

  await page.getByRole("button", { name: "Shape", exact: true }).click();
  await page.getByRole("menuitem", { name: "Oval" }).click();
  await clickFrame(page, 500, 400);
  await expect(shapes).toHaveCount(2);
  await expect(shapes.nth(1)).toHaveCSS("border-radius", /50%|^\d/);

  // An image from a file goes through the artifact's files and shows in the frame.
  await page.getByLabel("Choose a picture to put in an artboard.").setInputFiles({ name: "dot.png", mimeType: "image/png", buffer: dot });
  const image = onHero(page).locator("img[alt]");
  await expect(image).toHaveCount(1);
  await expect.poll(() => image.evaluate((n: HTMLImageElement) => n.naturalWidth)).toBeGreaterThan(0);
  await expect(image).toHaveAttribute("alt", "dot");

  // The rectangle is filled with a picture.
  await select(page, rectangle);
  await page.getByRole("button", { name: "Properties", exact: true }).first().click();
  await page.getByLabel("Fill with image…").setInputFiles({ name: "fill.png", mimeType: "image/png", buffer: dot });
  await expect.poll(() => rectangle.evaluate((n) => n.style.backgroundImage)).toContain("/files/");
  await expect.poll(() => rectangle.evaluate((n) => n.style.backgroundSize)).toBe("cover");

  // The pen gesture crosses the real inspector/room boundary as native SVG.
  await page.getByRole("button", { name: "Close properties" }).click();
  await page.keyboard.press("p");
  const frameBox = await heroFrame(page).boundingBox(); // page-level
  if (!frameBox) throw new Error("no frame to draw in");
  const scale = frameBox.width / 800;
  await page.mouse.move(frameBox.x + 350 * scale, frameBox.y + 350 * scale);
  await page.mouse.down();
  await page.mouse.move(frameBox.x + 420 * scale, frameBox.y + 380 * scale, { steps: 5 });
  await page.mouse.move(frameBox.x + 480 * scale, frameBox.y + 350 * scale, { steps: 5 });
  await page.mouse.up();
  const stroke = onHero(page).locator('svg[data-name="Pencil stroke"]');
  const path = stroke.locator("path");
  await expect(stroke).toHaveCount(1);
  await expect(path).toHaveAttribute("stroke", "#336699");
  await expect(path).toHaveAttribute("stroke-width", "3");
  const d = await path.getAttribute("d");
  expect(d).toMatch(/^M/);
  await page.keyboard.press("v");
  const box = await select(page, stroke);
  const original = await place(stroke);
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width / 2 + 30, box.y + box.height / 2 + 15, { steps: 4 });
  await page.mouse.up();
  await expect.poll(() => place(stroke)).toMatchObject({ left: `${String(Math.round(parseFloat(original.left) + 60))}px`, top: `${String(Math.round(parseFloat(original.top) + 30))}px` });
  // The inspector report follows the room update; resize the updated selection.
  await expect.poll(async () => {
    const selected = await page.locator("[data-selection]").boundingBox(); // page-level
    const moved = await onScreen(heroFrame(page), stroke);
    return Math.abs((selected?.x ?? 0) - moved.x);
  }).toBeLessThan(1);
  const handle = page.getByRole("button", { name: "Resize from bottom right" });
  const corner = await handle.boundingBox(); // page-level
  if (!corner) throw new Error("the stroke has no resize handle");
  await page.mouse.move(corner.x + corner.width / 2, corner.y + corner.height / 2);
  await page.mouse.down();
  await page.mouse.move(corner.x + corner.width / 2 + 20, corner.y + corner.height / 2 + 10, { steps: 4 });
  await page.mouse.up();
  await expect.poll(() => place(stroke)).toMatchObject({ width: `${String(Math.round(parseFloat(original.width) + 40))}px`, height: `${String(Math.round(parseFloat(original.height) + 20))}px` });
  const transformed = await place(stroke);
  await page.keyboard.press("Delete");
  await expect(stroke).toBeHidden();
  await page.keyboard.press("ControlOrMeta+z");
  await expect(stroke).toBeVisible();
  await expect.poll(() => place(stroke)).toEqual(transformed);

  // Connected Pen crosses the actual iframe, SVG build and room boundaries.
  const point = async (x: number, y: number, drag = false) => {
    const bounds = await heroFrame(page).boundingBox(); // page-level
    if (!bounds) throw new Error("missing artboard");
    const k = bounds.width / 800;
    await page.mouse.move(bounds.x + x * k, bounds.y + y * k);
    await page.mouse.down();
    if (drag) await page.mouse.move(bounds.x + (x + 24) * k, bounds.y + (y - 20) * k, { steps: 4 });
    await page.mouse.up();
  };
  await page.keyboard.press("Shift+p");
  await point(300, 80);
  await point(400, 80);
  await point(450, 160, true);
  await point(300, 80);
  const connected = onHero(page).locator('svg[data-name="Path"]');
  await expect(connected).toHaveCount(1);
  await expect(connected.first().locator("path")).toHaveAttribute("d", /L.*C.*Z$/);
  const closedD = await connected.first().locator("path").getAttribute("d");
  const connectedBox = await select(page, connected.first());
  await expect(page.getByRole("toolbar", { name: "Selection actions" })).toBeVisible();
  await page.getByRole("toolbar", { name: "Selection actions" }).getByRole("button", { name: "Properties" }).click();
  await expect(page.getByRole("toolbar", { name: "Selection actions" })).toHaveCount(0);
  await page.getByRole("button", { name: "Close properties" }).click();
  await expect(page.locator("[data-selection]")).toHaveAttribute("data-selection", (await connected.first().getAttribute("data-pid")) ?? "");
  await expect(page.getByRole("toolbar", { name: "Selection actions" })).toBeVisible();
  const beforeMove = await place(connected.first());
  await page.mouse.move(connectedBox.x + connectedBox.width / 2, connectedBox.y + connectedBox.height / 2);
  await page.mouse.down();
  await page.mouse.move(connectedBox.x + connectedBox.width / 2 + 15, connectedBox.y + connectedBox.height / 2 + 10, { steps: 4 });
  await page.mouse.up();
  await expect.poll(() => place(connected.first())).toMatchObject({ left: `${String(Math.round(parseFloat(beforeMove.left) + 30))}px`, top: `${String(Math.round(parseFloat(beforeMove.top) + 20))}px` });
  const connectedMoved = await place(connected.first());
  await page.mouse.click(connectedBox.x + 20, connectedBox.y + 20, {button:"right"});
  await page.getByRole("menu", { name: "Selection actions" }).getByRole("menuitem", { name: "Delete" }).click();
  await expect(connected.first()).toBeHidden();
  await page.keyboard.press("ControlOrMeta+z");
  await expect(connected.first()).toBeVisible();
  await expect.poll(() => place(connected.first())).toEqual(connectedMoved);
  await page.keyboard.press("Shift+p");
  await point(540, 70);
  const previewBounds = await heroFrame(page).boundingBox(); // page-level
  if (!previewBounds) throw new Error("missing artboard for Pen preview");
  const previewScale = previewBounds.width / 800;
  await page.mouse.move(previewBounds.x + 580 * previewScale, previewBounds.y + 90 * previewScale);
  const ghost = page.locator(".path-next");
  await expect(ghost).toBeVisible();
  const firstGhost = await ghost.getAttribute("d");
  await page.mouse.move(previewBounds.x + 600 * previewScale, previewBounds.y + 100 * previewScale);
  await expect(ghost).not.toHaveAttribute("d", firstGhost ?? "");
  await expect(page.locator(".path-anchor")).toHaveCount(1);
  await expect(connected).toHaveCount(1);
  await point(620, 110, true);
  await page.keyboard.press("ControlOrMeta+z");
  await expect(page.locator(".path-anchor")).toHaveCount(1);
  await expect(connected).toHaveCount(1);
  await point(620, 110, true);
  await page.keyboard.press("Escape");
  await expect(connected).toHaveCount(2);
  await expect(connected.nth(1).locator("path")).toHaveAttribute("d", /C/);
  const openD = await connected.nth(1).locator("path").getAttribute("d");
  expect(openD).not.toMatch(/Z$/);
  const openPid = await connected.nth(1).getAttribute("data-pid");
  if (!openPid) throw new Error("the open path has no stable identity");
  const openPath = onHero(page).locator(`svg[data-pid="${openPid}"]`);
  await page.keyboard.press("ControlOrMeta+z");
  await expect(openPath).toBeHidden();
  await expect(connected.first()).toBeVisible();
  await expect(connected.first().locator("path")).toHaveAttribute("d", closedD ?? "");
  await expect.poll(() => place(connected.first())).toEqual(connectedMoved);
  await page.keyboard.press("ControlOrMeta+Shift+z");
  await expect(openPath).toBeVisible();
  await expect(openPath.locator("path")).toHaveAttribute("d", openD ?? "");
  await page.keyboard.press("l");
  await point(90, 70, true);
  const line = onHero(page).locator('svg[data-name="Line"] path');
  await expect(line).toBeVisible();
  const lineD = await line.getAttribute("d");
  await page.keyboard.press("Shift+l");
  await point(90, 110, true);
  const arrow = onHero(page).locator('svg[data-name="Arrow"] path');
  await expect(arrow).toBeVisible();
  const arrowD = await arrow.getAttribute("d");

  await page.reload();
  await expect(onHero(page).locator("h1")).toBeVisible();
  await expect(onHero(page).getByText("Text", { exact: true })).toBeVisible();
  await expect(shapes).toHaveCount(2);
  await expect.poll(() => shapes.first().evaluate((n) => n.style.backgroundImage)).toContain("/files/");
  await expect.poll(() => onHero(page).locator("img[alt]").evaluate((n: HTMLImageElement) => n.naturalWidth)).toBeGreaterThan(0);
  await expect(stroke).toHaveCount(1);
  await expect(path).toHaveAttribute("d", d ?? "");
  await expect(path).toHaveAttribute("stroke", "#336699");
  await expect(path).toHaveAttribute("stroke-width", "3");
  await expect.poll(() => place(stroke)).toEqual(transformed);
  await expect(connected).toHaveCount(2);
  await expect(connected.first().locator("path")).toHaveAttribute("d", closedD ?? "");
  await expect(connected.nth(1).locator("path")).toHaveAttribute("d", openD ?? "");
  await expect(line).toHaveAttribute("d", lineD ?? "");
  await expect(arrow).toHaveAttribute("d", arrowD ?? "");
  await expect.poll(() => place(connected.first())).toEqual(connectedMoved);
  const bob = await browse(browser, watch, originOf(page), "bob");
  await bob.page.goto(`/a/${a.id}`);
  await expect(onHero(bob.page).locator('svg[data-name="Pencil stroke"] path')).toHaveAttribute("d", d ?? "");
  await expect(bob.page.getByRole("button", { name: "Drawing", exact: true })).toHaveCount(0);
  await select(bob.page, onHero(bob.page).locator('svg[data-name="Pencil stroke"]'));
  await bob.page.locator(".canvas-bar-right").getByRole("button", { name: "Properties", exact: true }).click();
  await expect(bob.page.getByRole("tab", { name: "Design", exact: true })).toHaveCount(0);
  await expect(bob.page.getByRole("button", { name: "Fill picker", exact: true })).toHaveCount(0);
  await expect(bob.page.locator("[data-handle]")).toHaveCount(0);
  await bob.page.keyboard.press("p");
  await expect(bob.page.locator(".pen-draw")).toHaveCount(0);
  await bob.context.close();
});
