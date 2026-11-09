import type { Page } from "@playwright/test";
import { expect, signIn, test } from "./support";
import { agentOf, block, browse, document, openEditor, originOf, publishDoc } from "./editor-support";

// Tables in the editor (docs/editor.md): built from the toolbar and the keyboard,
// edited by two people at once, pasted from a web page, and the same blocks an agent writes.

const cells = (page: Page) => document(page).locator("table td");
const cell = (page: Page, text: string) => document(page).locator("table td").filter({ hasText: new RegExp(`^${text}$`) });

test("insert a table, fill it with Tab, add a row from the last cell, align a column; an agent reads the pipe table", async ({ page }) => {
  await signIn(page, "alice");
  const made = await publishDoc(page, "Tables", "# Table\n\nIntro.\n");
  await openEditor(page, made.id);

  await block(page, "Intro.").click();
  await page.keyboard.press("End");
  await page.getByRole("button", { name: "Insert table" }).click();
  await expect(cells(page)).toHaveCount(9);

  for (const word of ["Item", "Cost", "Note", "Desk", "120", "big", "Chair", "40", "small"]) {
    await page.keyboard.type(word);
    await page.keyboard.press("Tab");
  }
  // The Tab in the last cell added a row and the caret is in it.
  await expect(cells(page)).toHaveCount(12);

  await cell(page, "Cost").click();
  await page.getByRole("button", { name: "Align right" }).click();
  await expect(document(page).locator("td.ed-right")).toHaveCount(4);
  await expect(page.getByText("Saved", { exact: true })).toBeVisible();

  const agent = await agentOf(page);
  const read = await agent("read", { id: made.id });
  const markdown = JSON.stringify(read.structured);
  expect(markdown).toContain("| Item | Cost | Note |");
  expect(markdown).toContain("| --- | --: | --- |");
  expect(markdown).toContain("| Desk | 120 | big |");

  await page.reload();
  await expect(page.locator(".ed[data-connection=live]")).toBeVisible();
  await expect(cells(page)).toHaveCount(12);
  await expect(document(page).locator("td.ed-right")).toHaveCount(4);

  await cell(page, "Desk").click();
  await page.getByRole("button", { name: "Delete row" }).click();
  await expect(cells(page)).toHaveCount(9);
  await page.getByRole("button", { name: "Delete table" }).click();
  await expect(document(page).locator("table")).toHaveCount(0);
});

test("two people edit different cells at once and both edits stay; one adds a row while the other types in a cell", async ({ page, browser, watch }) => {
  await signIn(page, "alice");
  const made = await publishDoc(page, "Tables together", "# Shared\n\n| A | B |\n| --- | --- |\n| a1 | b1 |\n| a2 | b2 |\n", ["bob"]);
  const bob = await browse(browser, watch, originOf(page), "bob");
  await openEditor(page, made.id);
  await openEditor(bob.page, made.id);

  await cell(page, "a1").click();
  await page.keyboard.press("End");
  await page.keyboard.type("-alice");
  await cell(bob.page, "b2").click();
  await bob.page.keyboard.press("End");
  await bob.page.keyboard.type("-bob");
  for (const seen of [page, bob.page]) {
    await expect(cell(seen, "a1-alice")).toHaveCount(1);
    await expect(cell(seen, "b2-bob")).toHaveCount(1);
  }

  await cell(page, "a2").click();
  await page.getByRole("button", { name: "Row below" }).click();
  await cell(bob.page, "b1").click();
  await bob.page.keyboard.press("End");
  await bob.page.keyboard.type("!");
  for (const seen of [page, bob.page]) {
    await expect(cells(seen)).toHaveCount(8);
    await expect(cell(seen, "b1!")).toHaveCount(1);
    await expect(cell(seen, "b2-bob")).toHaveCount(1);
  }
  await bob.context.close();
});

test("a table pasted from a web page becomes a table with its header and alignment, and nothing unsafe comes with it", async ({ page }) => {
  await signIn(page, "alice");
  const made = await publishDoc(page, "Pasted table", "# Paste\n\nHere:\n");
  await openEditor(page, made.id);
  await block(page, "Here:").click();
  await page.keyboard.press("End");
  await page.keyboard.press("Enter");
  await document(page).evaluate((el) => {
    const data = new DataTransfer();
    data.setData("text/html", '<table onclick="window.stolen=1"><thead><tr><th>Name</th><th style="text-align:right">Qty</th></tr></thead><tbody><tr><td><a href="https://example.com/x">Bolt</a></td><td>4</td></tr></tbody></table>');
    el.dispatchEvent(new ClipboardEvent("paste", { clipboardData: data, bubbles: true, cancelable: true }));
  });
  await expect(cells(page)).toHaveCount(4);
  await expect(document(page).locator("td.ed-right")).toHaveCount(2);
  await expect(document(page).locator("table.ed-headed")).toHaveCount(1);
  await expect(document(page).locator("table a")).toHaveAttribute("href", "https://example.com/x");
  expect(await page.evaluate(() => (window as unknown as { stolen?: number }).stolen)).toBeUndefined();
});

test("a table an agent adds with doc_edit shows in the editor and can be edited", async ({ page }) => {
  await signIn(page, "alice");
  const made = await publishDoc(page, "Agent table", "# Agent\n\nStart.\n");
  await openEditor(page, made.id);
  const agent = await agentOf(page);
  const read = await agent("read", { id: made.id, view: "blocks" });
  const first = (read.structured.tabs as { blocks: { id: string }[] }[])[0]?.blocks[0];
  const edited = await agent("doc_edit", { id: made.id, ops: [{ op: "insert", after: first?.id, markdown: "| X | Y |\n| :-- | --: |\n| 1 | 2 |\n" }] });
  expect(edited.isError, edited.text).toBe(false);
  await expect(cells(page)).toHaveCount(4);
  await expect(document(page).locator("td.ed-left")).toHaveCount(2);
  await expect(document(page).locator("td.ed-right")).toHaveCount(2);
  await cell(page, "2").click();
  await page.getByRole("button", { name: "Column left" }).click();
  await expect(cells(page)).toHaveCount(6);
});
