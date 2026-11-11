import { readFileSync } from "node:fs";
import type { Page } from "@playwright/test";
import { agentOf, browse, originOf } from "./editor-support";
import { expect, onScreen, publish, republish, signIn, test } from "./support";

// The messy frame the unit tests normalise: agent-style markup (unquoted attributes,
// a table without end tags, gradients, grid, @media, custom properties) that the
// editor must not change on screen.
const messy = readFileSync(new URL("../../internal/designfmt/testdata/messy.html", import.meta.url), "utf8");

const frame = (body: string) => `<!doctype html><html><head><meta charset="utf-8"></head><body style="margin:0">${body}</body></html>`;

const manifest = JSON.stringify({
  version: 1,
  pages: [
    {
      id: "home",
      name: "Home",
      rows: [
        [
          { id: "hero", name: "Hero", file: "frames/hero.html", w: 800, h: 600 },
          { id: "mobile", name: "Mobile", file: "frames/mobile.html", w: 390, h: 844 },
        ],
      ],
    },
    { id: "extra", name: "Extra", rows: [[{ id: "footer", name: "Footer", file: "frames/footer.html", w: 800, h: 200 }]] },
  ],
});

const frames = {
  "frames/hero.html": frame(`<h1 id="title" style="font-size:40px;margin:48px">Hero</h1>`),
  "frames/mobile.html": frame(`<h1 style="font-size:28px">Mobile</h1>`),
  "frames/footer.html": frame(`<p>Footer</p>`),
};

/** A frame of the open canvas, by its name. */
const onCanvas = (page: Page, name: string) => page.frameLocator(`.dframe iframe[title^="${name},"]`);
const zoom = (page: Page) => page.getByRole("button", { name: /^Zoom \d+%/ });
/** Opens the Page menu and chooses a page. */
async function openPage(page: Page, from: string, name: string) {
  await page.getByRole("button", { name: `Page: ${from}` }).click();
  await page.getByRole("menuitemradio", { name }).click();
}

test("a design opens on a canvas: frames, Play and anchored comments shared with a reader", async ({ page, browser, watch }) => {
  await signIn(page, "alice");
  const a = await publish(page, { title: "Landing", kind: "design", content: manifest, files: frames, visibility: "organisation" });
  await page.goto(`/a/${a.id}`);
  await expect(onCanvas(page, "Hero").locator("#title")).toHaveText("Hero");
  await expect(onCanvas(page, "Mobile").locator("h1")).toHaveText("Mobile");
  await expect(page.locator(".dframe-name", { hasText: "Hero" })).toBeVisible();

  // The page is fitted to the window, then zoom steps from there.
  const fitted = Number.parseInt((await zoom(page).textContent()) ?? "0", 10);
  expect(fitted).toBeGreaterThan(0);
  expect(fitted).toBeLessThanOrEqual(100);
  await zoom(page).click();
  await page.getByRole("menuitem", { name: "Zoom in" }).click();
  const stepped = Number.parseInt((await zoom(page).textContent()) ?? "0", 10);
  expect(stepped).toBeGreaterThan(fitted);
  await page.keyboard.press("1");
  await expect(zoom(page)).toHaveText("100%");
  await zoom(page).click();
  await page.getByRole("menuitem", { name: "Fit" }).click();
  await expect(zoom(page)).toHaveText(`${String(fitted)}%`);

  // The frame on screen is the size the design says, scaled by the zoom.
  const box = await page.locator(".dframe iframe").first().boundingBox(); // page-level
  expect(box?.width).toBeCloseTo((800 * fitted) / 100, -1);

  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.getByRole("button", { name: "Page: Home" }).click();
  await page.getByRole("menuitem", { name: "More actions for Extra" }).hover();
  await page.getByRole("menuitem", { name: "Copy link to page" }).click();
  const expectedLink = new URL(page.url());
  expectedLink.searchParams.set("page", "extra");
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(expectedLink.href);
  const copiedPage = await page.evaluate(() => navigator.clipboard.readText());
  expect(new URL(copiedPage).searchParams.get("page")).toBe("extra");
  await expect(page.getByRole("button", { name: "Page: Home" })).toBeVisible();
  await page.goto(copiedPage);
  await expect(page.getByRole("button", { name: "Page: Extra" })).toBeVisible();
  await expect(onCanvas(page, "Footer").locator("p")).toHaveText("Footer");
  await expect(page.locator(".dframe", { hasText: "Hero" })).toHaveCount(0);
  await openPage(page, "Extra", "Home");

  const frameActions = page.getByRole("group", { name: "Artboard actions" });
  if (await frameActions.isVisible()) {
    await frameActions.getByRole("button", { name: /^Artboard:/ }).click();
    await page.getByRole("menuitemradio", { name: /^Mobile/ }).click();
  }
  await page.getByRole("button", { name: "Expand Mobile" }).click();
  const playing = page.getByRole("dialog", { name: "Playing Mobile" });
  await expect(playing).toBeVisible();
  await expect(playing.frameLocator("iframe").locator("h1")).toHaveText("Mobile");
  // Fit shows all of it, at its real size unless the window is smaller; Fill makes its width the window's.
  const stage = await playing.locator(".play-stage").boundingBox(); // page-level
  const played = await playing.locator("iframe").boundingBox();
  if (!stage || !played) throw new Error("nothing is playing");
  expect(played.width).toBeCloseTo(390 * Math.min(1, stage.width / 390, stage.height / 844), 0);
  await playing.getByRole("button", { name: "Fill" }).click();
  await expect.poll(async () => (await playing.locator("iframe").boundingBox())?.width).toBeCloseTo(stage.width, 0); // page-level
  await page.keyboard.press("Escape");
  await expect(playing).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Expand Mobile" })).toBeFocused();
  // A frame or source element has one conversation in both the pin layer and panel.
  const titleBox = await onScreen(page.locator('.dframe iframe[title^="Hero,"]'), onCanvas(page, "Hero").locator("#title"));
  await page.mouse.click(titleBox.x + titleBox.width / 2, titleBox.y + titleBox.height / 2);
  await page.getByRole("button", { name: "Comment on selection" }).click();
  const comments = page.getByRole("complementary", { name: "Comments" });
  await comments.getByRole("textbox", { name: "Write a comment" }).fill("Review this heading");
  await comments.getByRole("button", { name: "Comment", exact: true }).click();
  await expect(comments.getByRole("article")).toContainText("Review this heading");
  await page.reload();
  const pin = page.getByRole("button", { name: "Comment 1 on Hero" });
  await expect(pin).toBeVisible();
  await expect(pin).not.toHaveAttribute("data-fallback", "true");
  await pin.click();
  await expect(comments.getByRole("article")).toContainText("Review this heading");
  await expect(pin).toHaveAttribute("aria-pressed", "true");


  const reader = await browse(browser, watch, originOf(page), "bob");
  await reader.page.goto(`/a/${a.id}`);
  await expect(reader.page.locator('.dframe iframe[title^="Hero,"]')).toBeVisible();
  const readerFrames = reader.page.getByRole("group", { name: "Artboard actions" });
  if (await readerFrames.isVisible()) {
    await readerFrames.getByRole("button", { name: /^Artboard:/ }).click();
    await reader.page.getByRole("menuitemradio", { name: /^Hero/ }).click();
  }
  const readerTitle = onCanvas(reader.page, "Hero").locator("#title");
  const readerTitleBox = await onScreen(reader.page.locator('.dframe iframe[title^="Hero,"]'), readerTitle);
  await reader.page.mouse.click(readerTitleBox.x + readerTitleBox.width / 2, readerTitleBox.y + readerTitleBox.height / 2);
  await reader.page.getByRole("button", { name: "Comment on selection", exact: true }).click();
  const readerComments = reader.page.getByRole("complementary", { name: "Comments" });
  await readerComments.getByRole("textbox", { name: "Write a comment" }).fill("Reader frame feedback");
  await readerComments.getByRole("button", { name: "Comment", exact: true }).click();
  await expect(readerComments.getByRole("article").filter({ hasText: "Reader frame feedback" })).toBeVisible();
  await reader.page.reload();
  await reader.page.getByRole("button", { name: "Comment 2 on Hero", exact: true }).click();
  await expect(readerComments.getByRole("article").filter({ hasText: "Reader frame feedback" })).toBeVisible();
  const agent = await agentOf(page);
  const listed = (await agent("comments", { id: a.id })).structured.comments as { body: string; anchor: { type: string; frameId: string; pid?: string; x: number; y: number } }[];
  const heading = listed.find((c) => c.body === "Review this heading");
  expect(heading?.anchor).toMatchObject({ type: "design", frameId: "hero" });
  expect(heading?.anchor.pid).toBeTruthy();
  expect(listed.find((c) => c.body === "Reader frame feedback")?.anchor).toMatchObject({ type: "design", frameId: "hero", pid: heading?.anchor.pid });
  await comments.getByRole("button", { name: "Close", exact: true }).click();
  await page.keyboard.press("p");
  const heroBox = await page.locator('.dframe iframe[title^="Hero,"]').boundingBox(); // page-level
  if (!heroBox) throw new Error("no frame to draw in");
  const scale = heroBox.width / 800;
  await page.mouse.move(heroBox.x + 350 * scale, heroBox.y + 300 * scale);
  await page.mouse.down();
  await page.mouse.move(heroBox.x + 480 * scale, heroBox.y + 330 * scale, { steps: 5 });
  await page.mouse.up();
  const drawing = onCanvas(page, "Hero").locator('svg[data-pen="stroke"]');
  await expect(drawing).toHaveCount(1);
  await page.keyboard.press("v");
  const drawingBox = await onScreen(page.locator('.dframe iframe[title^="Hero,"]'), drawing);
  await page.mouse.click(drawingBox.x + drawingBox.width / 2, drawingBox.y + drawingBox.height / 2);
  await page.getByRole("button", { name: "Comment on selection" }).click();
  await comments.getByRole("textbox", { name: "Write a comment" }).fill("Review this drawing");
  await comments.getByRole("button", { name: "Comment", exact: true }).click();
  await expect(comments.getByRole("article").filter({ hasText: "Review this drawing" })).toBeVisible();
  await comments.getByRole("button", { name: "Close", exact: true }).click();
  await page.mouse.click(drawingBox.x + drawingBox.width / 2, drawingBox.y + drawingBox.height / 2);
  await expect(page.locator("[data-selection]")).toBeVisible();
  await page.keyboard.press("Delete");
  await expect(drawing).toBeHidden();
  await expect(page.getByRole("button", { name: "Comment 3 on Pencil stroke", exact: true })).toHaveCount(0);
  await page.keyboard.press("ControlOrMeta+z");
  await expect(drawing).toBeVisible();
  const drawingPin = page.getByRole("button", { name: "Comment 3 on Pencil stroke", exact: true });
  await expect(drawingPin).toBeVisible();
  await drawingPin.click();
  await expect(comments.getByRole("article").filter({ hasText: "Review this drawing" })).toBeVisible();
  await comments.getByRole("button", { name: "Close", exact: true }).click();
  const restoredBox = await onScreen(page.locator('.dframe iframe[title^="Hero,"]'), drawing);
  await page.mouse.click(restoredBox.x + restoredBox.width / 2, restoredBox.y + restoredBox.height / 2);
  await expect(page.locator("[data-selection]")).toBeVisible();
  await page.keyboard.press("Delete");
  await expect(drawing).toBeHidden();
  // A reload drops an edit the server has not taken yet: wait until the room's file no longer has the stroke.
  await expect.poll(async () => ((await agent("read", { id: a.id, file: "frames/hero.html" })).structured["file"] as { text?: string } | undefined)?.text ?? "").not.toContain('data-pen="stroke"');
  await page.reload();
  await expect(page.getByRole("button", { name: "Comment 1 on Hero", exact: true })).toBeVisible();
  await expect(drawingPin).toHaveCount(0);
  await page.getByRole("button", { name: "Comments", exact: true }).click();
  const orphan = comments.getByRole("listitem").filter({ hasText: "Review this drawing" });
  await expect(orphan.getByText("Original element removed")).toBeVisible();
  await orphan.getByRole("button", { name: "Delete comment", exact: true }).click();
  await page.getByRole("alertdialog", { name: "Delete your comment?" }).getByRole("button", { name: "Delete comment", exact: true }).click();
  await expect(comments.getByText("Review this drawing")).toHaveCount(0);
  const current = await agent("read", { id: a.id });
  const source = await agent("read", { id: a.id, file: "frames/hero.html" });
  await republish(page, a.id, { title: "Landing", content: current.structured.content as string, baseVersion: current.structured.version as number, sourceBases: { ...current.structured.sourceBases as Record<string, string>, ...source.structured.sourceBases as Record<string, string> }, files: { "frames/hero.html": frame('<p data-pid="replacement">Replacement</p>') } });
  await reader.page.reload();
  await expect(reader.page.getByRole("button", { name: "Comment 1 on Hero", exact: true })).toHaveCount(0);
  await expect(reader.page.getByRole("button", { name: "Comment 2 on Hero", exact: true })).toHaveCount(0);
  await reader.page.getByRole("button", { name: "Comments", exact: true }).click();
  const removedHeading = readerComments.getByRole("listitem").filter({ hasText: "Review this heading" });
  await expect(removedHeading).toBeVisible();
  await expect(removedHeading.getByText("Original element removed")).toBeVisible();
  await expect(removedHeading.getByRole("button", { name: "Delete comment", exact: true })).toHaveCount(0);
  await reader.context.close();

});

test("dragging the canvas moves the page and the hand tool and Space both pan", async ({ page }) => {
  await signIn(page, "alice");
  const a = await publish(page, { title: "Landing", kind: "design", content: manifest, files: frames });
  await page.goto(`/a/${a.id}`);
  await expect(onCanvas(page, "Hero").locator("#title")).toHaveText("Hero");
  // Pan moves the canvas box; its iframe can reload as the room connects.
  const hero = page.locator('[data-frame="hero"]');
  await expect(hero).toBeVisible();
  const start = await hero.boundingBox(); // page-level
  const viewport = page.locator(".canvas-viewport");
  const v = await viewport.boundingBox(); // page-level
  if (!start || !v) throw new Error("nothing on screen");
  await page.mouse.move(v.x + 20, v.y + v.height - 20);
  await page.mouse.down();
  await page.mouse.move(v.x + 80, v.y + v.height - 50, { steps: 4 });
  await page.mouse.up();
  const dragged = await hero.boundingBox(); // page-level
  expect(dragged?.x).toBeCloseTo(start.x + 60, 0);
  expect(dragged?.y).toBeCloseTo(start.y - 30, 0);
  // The wheel pans too.
  await page.mouse.move(v.x + v.width / 2, v.y + v.height / 2);
  await page.mouse.wheel(0, 50);
  await expect.poll(async () => (await hero.boundingBox())?.y).toBeCloseTo(start.y - 30 - 50, 0); // page-level
});

test("a frame is in an opaque origin, has no bridge, and runs no script of its own", async ({ page, expectConsoleErrors }) => {
  // The test puts a script in a frame to see the policy refuse it.
  // Chrome words the refusal differently across versions ("Refused to execute inline script" before 155, "Executing inline script violates" after).
  expectConsoleErrors(/Refused to execute inline script|Executing inline script violates/);
  await signIn(page, "alice");
  const a = await publish(page, { title: "Landing", kind: "design", content: manifest, files: frames });
  await page.goto(`/a/${a.id}`);
  const hero = onCanvas(page, "Hero");
  await expect(hero.locator("#title")).toHaveText("Hero");
  expect(await hero.locator("body").evaluate(() => window.origin)).toBe("null");
  expect(await hero.locator("html").evaluate((el) => el.innerHTML.includes("window.pictor"))).toBe(false);
  await hero.locator("body").evaluate((body) => {
    const added = document.createElement("script");
    added.textContent = "document.body.dataset.ran = 'yes'";
    body.appendChild(added);
  });
  await expect(hero.locator("body")).not.toHaveAttribute("data-ran", "yes");
});

test("a click selects the element under it at 50% zoom, the box is where the element is, Shift adds one and the Code panel reads its CSS", async ({ page }) => {
  // Wide enough for both frames to be on screen beside the outline and the Code panel.
  await page.setViewportSize({ width: 1600, height: 800 });
  await signIn(page, "alice");
  const a = await publish(page, { title: "Landing", kind: "design", content: manifest, files: frames });
  await page.goto(`/a/${a.id}`);
  await expect(onCanvas(page, "Hero").locator("#title")).toHaveText("Hero");
  await expect(onCanvas(page, "Mobile").locator("h1")).toHaveText("Mobile");
  await page.keyboard.press("1");
  await page.keyboard.press("-");
  await page.keyboard.press("-");
  await expect(zoom(page)).toHaveText("50%");

  const centre = async (frameName: string, inner: string) => {
    const box = await onScreen(page.locator(`.dframe iframe[title^="${frameName},"]`), onCanvas(page, frameName).locator(inner));
    return { box, x: box.x + box.width / 2, y: box.y + box.height / 2 };
  };
  const title = await centre("Hero", "#title");
  await page.mouse.click(title.x, title.y);
  const selection = page.locator("[data-selection]");
  await expect(selection).toHaveCount(1);
  const drawn = await selection.boundingBox(); // page-level
  if (!drawn) throw new Error("no selection box");
  expect(drawn.x).toBeCloseTo(title.box.x, 0);
  expect(drawn.y).toBeCloseTo(title.box.y, 0);
  expect(drawn.width).toBeCloseTo(title.box.width, 0);
  expect(drawn.height).toBeCloseTo(title.box.height, 0);

  // The Code panel says what the heading is, and the size it ends up with.
  // Properties opens explicitly without changing the selected source coordinates.
  await page.locator(".canvas-bar-right").getByRole("button", { name: "Properties" }).click();
  await page.getByRole("tab", { name: "Code" }).click();
  const panel = page.getByRole("region", { name: "Element properties" });
  await expect(panel.getByRole("heading", { level: 3, name: /^<h1>/ })).toContainText("<h1>");
  const computed = panel.locator("section", { has: page.getByRole("heading", { name: "Computed" }) });
  await expect(computed.locator(".code-row", { has: page.locator("dt", { hasText: /^font-size$/ }) }).locator("dd")).toHaveText("40px");

  // Shift adds the heading of another frame.
  const mobile = await centre("Mobile", "h1");
  await page.keyboard.down("Shift");
  await page.mouse.click(mobile.x, mobile.y);
  await page.keyboard.up("Shift");
  await expect(selection).toHaveCount(2);
  await expect(panel.getByText("2 selected, showing the last")).toBeVisible();
  // Without Shift the click replaces the selection; Escape clears it.
  await page.mouse.click(title.x, title.y);
  await expect(selection).toHaveCount(1);
  await page.keyboard.press("Escape");
  await expect(selection).toHaveCount(0);
  await expect(panel).toBeVisible();
  await panel.getByRole("button", { name: "Close properties" }).click();
  await expect(panel).toHaveCount(0);
});

test("a frame cannot reach the API: its policy refuses the request", async ({ page, expectConsoleErrors }) => {
  expectConsoleErrors(/Refused to connect|Connecting to .* violates/);
  const refusals: string[] = [];
  page.on("console", (m) => {
    if (m.type() === "error") refusals.push(m.text());
  });
  await signIn(page, "alice");
  const a = await publish(page, { title: "Landing", kind: "design", content: manifest, files: frames });
  await page.goto(`/a/${a.id}`);
  const hero = onCanvas(page, "Hero");
  await expect(hero.locator("#title")).toHaveText("Hero");
  // A page of the frame's origin would be let through; this one is in an opaque origin under default-src 'none'.
  const outcome = await hero.locator("body").evaluate(async () => {
    try {
      await fetch(`${location.origin}/api/artifacts`);
      return "answered";
    } catch {
      return "refused";
    }
  });
  expect(outcome).toBe("refused");
  // The browser says whose refusal it was: the frame's policy, not the API (which would have said so in a response).
  expect(refusals.some((m) => /Refused to connect|Connecting to .* violates/.test(m) && m.includes("default-src 'none'"))).toBe(true);
});

test("the gallery card of a design shows its frames as a thumbnail, scaled to the card", async ({ page }) => {
  await signIn(page, "alice");
  const title = `Card ${String(Date.now())}`;
  const a = await publish(page, { title, kind: "design", content: manifest, files: frames });
  await page.goto("/");
  const card = page.locator(`[data-artifact-id="${a.id}"]`);
  await expect(card).toBeVisible();
  await card.scrollIntoViewIfNeeded();
  const image=card.locator(".thumb-screen.loaded > img");
  await expect(image).toBeVisible({timeout:25000});
  await expect(card.locator("iframe")).toHaveCount(0);
  expect(await image.evaluate((node)=>{const img=node as HTMLImageElement;return [img.naturalWidth,img.naturalHeight];})).toEqual([1120,720]);
  const drawn=await image.boundingBox(); // page-level
  const slot=await card.locator("[data-thumb]").boundingBox(); // page-level
  if(!drawn||!slot)throw Error("no image thumbnail box");
  expect(drawn.width).toBeLessThanOrEqual(slot.width+1);
  expect(drawn.width).toBeGreaterThan(slot.width*0.7);
  expect(drawn.x).toBeGreaterThanOrEqual(slot.x);

});

test("a frame is drawn the same on screen after publish as the page it was written as", async ({ page }) => {
  await signIn(page, "alice");
  const asPage = await publish(page, { title: "Original", content: messy });
  const asFrame = await publish(page, {
    title: "Normalised",
    kind: "design",
    content: JSON.stringify({ version: 1, pages: [{ id: "p", name: "P", rows: [[{ id: "f", name: "F", file: "frames/messy.html", w: 800, h: 600 }]] }] }),
    files: { "frames/messy.html": messy },
  });
  await page.setViewportSize({ width: 800, height: 600 });
  // The same markup, once as the page the author wrote it as and once as the frame the
  // platform stored and serves; the frame's address is the one the canvas loads.
  await page.goto(`/a/${asFrame.id}`);
  const src = await page.locator(".dframe iframe").getAttribute("src");
  expect(src).toBeTruthy();
  await expect(onCanvas(page, "F").locator("[data-pid]").first()).toBeAttached();
  const shot = async (url: string) => {
    await page.goto(url);
    await page.waitForLoadState("load");
    return page.screenshot({ animations: "disabled", caret: "hide" });
  };
  const frameShot = await shot(src ?? "");
  const pageShot = await shot(`/raw/${asPage.id}`);
  expect(frameShot.equals(pageShot), "the frame's screenshot differs from the original page's").toBe(true);
});
