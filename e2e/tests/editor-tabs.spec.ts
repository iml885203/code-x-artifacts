import type { Page } from "@playwright/test";
import { expect, signIn as signInAs, test } from "./support";
import { block, browse, openEditor, openMenu, originOf, publishDoc, typeAtEnd } from "./editor-support";

// Tabs in the editor (docs/editor.md): a list beside the page to add, rename, move and delete them,
// live for everyone. The rest (rename, move, delete, the agent ops) is proven in vitest and Go.

const tabs = (page: Page) => page.getByRole("navigation", { name: "Tabs" });

test("alice adds a tab while bob types in another: both see it, and bob's text is intact", async ({ page, browser, watch }) => {
  await signIn(page);
  const made = await publishDoc(page, "Tabs together", "# One\n\nfirst\n", ["bob"]);
  const bob = await browse(browser, watch, originOf(page), "bob");
  await openEditor(page, made.id);
  await openEditor(bob.page, made.id);

  await typeAtEnd(bob.page, "first", " and bob");
  await (await openMenu(page, "Tabs")).getByRole("button", { name: "Add tab" }).click();
  await page.keyboard.type("Notes page");
  await openMenu(page, "Tabs");
  await expect(tabs(page).getByRole("button", { name: "Untitled tab" })).toBeVisible();

  // Bob is still in the tab he was typing in, with his text; the new tab holds what alice typed in its heading.
  await expect(block(bob.page, "first and bob")).toBeVisible();
  await bob.page.keyboard.type("!");
  await expect(block(bob.page, "first and bob!")).toBeVisible();
  await openMenu(bob.page, "Tabs");
  await expect(tabs(bob.page).getByRole("button", { name: "Untitled tab" })).toBeVisible();
  await tabs(bob.page).getByRole("button", { name: "Untitled tab" }).click();
  await expect(block(bob.page, "Notes page")).toBeVisible();
  await expect(block(page, "Notes page")).toBeVisible();
});

const signIn = (page: Page) => signInAs(page, "alice");
