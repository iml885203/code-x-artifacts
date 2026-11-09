import { expect, nameEditors, presented, publish, signIn, test } from "./support";
import type { Page } from "@playwright/test";

const deck = {
  version: 1,
  slides: [
    { layout: "title", title: "ALPHA", notes: "a" },
    { title: "BRAVO", elements: [{ type: "text", text: "second" }], notes: "b" },
    { title: "CHARLIE", elements: [{ type: "text", text: "third" }], notes: "c" },
  ],
};

const filmstrip = (page: Page) => page.getByRole("list", { name: "Slide list" });
const field = (page: Page) => page.getByRole("textbox", { name: "Text on the slide" });
const names = (page: Page) => filmstrip(page).getByRole("button").evaluateAll((all) => all.map((b) => b.getAttribute("aria-label")));

test("an editor reorders and deletes slides in the filmstrip and fixes a title and a table cell in place, all saved, and neither a reader nor someone not signed in can edit the public deck", async ({ page, browser, watch }) => {
  await signIn(page, "alice");
  const made = await publish(page, { title: "Quarter", kind: "slides", content: JSON.stringify(deck), visibility: "public" });
  await nameEditors(page, made.id, ["bob"]);
  await signIn(page, "bob");
  await page.goto(`/a/${made.id}`);
  const stage = page.frameLocator("iframe[data-artifact-frame]");
  await expect(stage.getByRole("heading", { name: "ALPHA" })).toBeVisible();
  await expect.poll(() => names(page)).toEqual(["Slide 1: ALPHA", "Slide 2: BRAVO", "Slide 3: CHARLIE"]);
  // Each thumbnail is the deck's script-less preview, showing its own slide.
  await expect(filmstrip(page).getByRole("button", { name: "Slide 3: CHARLIE" }).frameLocator("iframe").getByRole("heading", { name: "CHARLIE" })).toBeVisible();

  const charlie = filmstrip(page).getByRole("button", { name: "Slide 3: CHARLIE" });
  const box = await charlie.boundingBox(); // page-level
  if (!box) throw new Error("the last thumbnail is not on screen");
  await filmstrip(page).getByRole("button", { name: "Slide 1: ALPHA" }).dragTo(charlie, { targetPosition: { x: box.width - 4, y: box.height / 2 } });
  await expect.poll(() => names(page)).toEqual(["Slide 1: BRAVO", "Slide 2: CHARLIE", "Slide 3: ALPHA"]);
  await expect(stage.getByRole("heading", { name: "ALPHA" })).toBeVisible();

  await filmstrip(page).getByRole("button", { name: "Slide 1: BRAVO" }).click();
  await expect(stage.getByRole("heading", { name: "BRAVO" })).toBeVisible();
  await page.keyboard.press("Delete");
  const ask = page.getByRole("alertdialog", { name: "Delete slide 1?" });
  await presented(ask);
  await ask.getByRole("button", { name: "Delete" }).click();
  await expect.poll(() => names(page)).toEqual(["Slide 1: CHARLIE", "Slide 2: ALPHA"]);

  await page.reload();
  await expect.poll(() => names(page)).toEqual(["Slide 1: CHARLIE", "Slide 2: ALPHA"]);
  await expect(stage.getByRole("heading", { name: "CHARLIE" })).toBeVisible();
  await expect(page.locator(".vtitle .facts")).toContainText("v3");

  // In place: the shell's own field over the frame's title, saved with replace_slide.
  await page.getByRole("button", { name: "Edit “CHARLIE”" }).click();
  await expect(field(page)).toHaveValue("CHARLIE");
  await field(page).fill("CHARLIE 修正");
  await field(page).press("Enter");
  await expect(stage.getByRole("heading", { name: "CHARLIE 修正" })).toBeVisible();
  // A table from the insert tool opens its first header to type over; the server took its default content.
  await page.getByRole("toolbar", { name: "Insert" }).getByRole("button", { name: "Table" }).click();
  await expect(field(page)).toHaveValue("Column 1");
  await field(page).fill("Owner");
  await field(page).press("Enter");
  await expect(stage.getByRole("columnheader", { name: "Owner" })).toBeVisible();
  await page.getByRole("button", { name: "Edit “empty”" }).first().click();
  await field(page).fill("Logan");
  await field(page).press("Enter");
  await expect(stage.getByRole("cell", { name: "Logan" })).toBeVisible();

  await page.reload();
  await expect(stage.getByRole("heading", { name: "CHARLIE 修正" })).toBeVisible();
  await expect(stage.getByRole("cell", { name: "Logan" })).toBeVisible();
  await expect.poll(() => names(page)).toEqual(["Slide 1: CHARLIE 修正", "Slide 2: ALPHA"]);

  await signIn(page, "carol");
  await page.goto(`/a/${made.id}`);
  await expect(stage.getByRole("heading", { name: "CHARLIE 修正" })).toBeVisible();
  await expect.poll(() => names(page)).toHaveLength(2);
  await expect(page.getByRole("toolbar", { name: "Insert" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: /^Edit “/ })).toHaveCount(0);
  await expect(page.getByRole("textbox", { name: "Text on the slide" })).toHaveCount(0);

  const anon = await browser.newContext({ baseURL: page.url().split("/").slice(0, 3).join("/") });
  const visitor = watch(await anon.newPage());
  await visitor.goto(`/a/${made.id}`);
  await expect.poll(() => names(visitor)).toEqual(["Slide 1: CHARLIE 修正", "Slide 2: ALPHA"]);
  await filmstrip(visitor).getByRole("button", { name: "Slide 2: ALPHA" }).click();
  await expect(visitor.frameLocator("iframe[data-artifact-frame]").getByRole("heading", { name: "ALPHA" })).toBeVisible();
  await expect(visitor.getByRole("button", { name: "New slide" })).toHaveCount(0);
  await expect(visitor.getByRole("toolbar", { name: "Insert" })).toHaveCount(0);
  await expect(visitor.getByRole("button", { name: /^Edit “/ })).toHaveCount(0);
  await anon.close();
});
