import { expect, signIn, test } from "./support";
import { block, document } from "./editor-support";

// Starting a document or a design from the site, and renaming it from the viewer's header.

test("New, Document, type, reload, rename: the title changes and the heading does not", async ({ page }) => {
  await signIn(page, "alice");
  await page.goto("/");
  await page.getByRole("button", { name: "New" }).click();
  await page.getByRole("menuitem", { name: "Document" }).click();
  await expect(page).toHaveURL(/\/a\/[^/?]+$/);
  await expect(page.locator(".ed[data-connection=live]")).toBeVisible();

  // The caret is already in the empty heading.
  await page.keyboard.type("My first page");
  await expect(block(page, "My first page")).toBeVisible();
  await expect(page.getByText("Saved", { exact: true })).toBeVisible();

  await page.reload();
  await expect(page.locator(".ed[data-connection=live]")).toBeVisible();
  await expect(block(page, "My first page")).toBeVisible();

  await page.getByRole("button", { name: "Untitled document" }).click();
  await page.getByRole("menuitem", { name: "Rename" }).click();
  await page.getByRole("textbox", { name: "Title" }).fill("Quarterly plan");
  await page.keyboard.press("Enter");
  await expect(page.getByRole("heading", { name: "Quarterly plan", level: 1 }).first()).toBeVisible();
  await page.reload();
  await expect(page.getByRole("button", { name: "Quarterly plan" })).toBeVisible();
  await expect(document(page).locator("h1")).toHaveText("My first page");
});
