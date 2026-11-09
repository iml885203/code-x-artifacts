import { expect, signIn, test } from "./support";
import { agentOf, block, document, openEditor, publishDoc } from "./editor-support";

// Links in the editor (docs/editor.md): Cmd+K, the bar for a link the caret is in,
// and the same address rules an agent's Markdown is held to.

const skeleton = "# Links\n\nRead the docs here.\n";

test("select text, Cmd+K, save, click the link, change it and remove it", async ({ page }) => {
  await signIn(page, "alice");
  const made = await publishDoc(page, "Links", skeleton);
  await openEditor(page, made.id);

  await block(page, "Read the docs here.").click({ clickCount: 3 });
  await page.keyboard.press("ControlOrMeta+k");
  const form = page.getByRole("dialog", { name: "Link" });
  await form.getByLabel("Address").fill("javascript:alert(1)");
  await form.getByRole("button", { name: "Save" }).click();
  await expect(form.getByRole("alert")).toContainText("Links must start with https:");
  await expect(document(page).locator("a")).toHaveCount(0);

  await form.getByLabel("Address").fill("https://example.com/docs");
  await form.getByRole("button", { name: "Save" }).click();
  await expect(document(page).locator("a")).toHaveAttribute("href", "https://example.com/docs");
  await expect(document(page).locator("a")).toHaveText("Read the docs here.");

  // The caret leaves the link and comes back by a click: the bar shows where it goes.
  await document(page).locator("h1").click();
  await expect(page.getByRole("group", { name: "Link" })).toHaveCount(0);
  await document(page).locator("a").click();
  const bar = page.getByRole("group", { name: "Link" });
  await expect(bar).toContainText("https://example.com/docs");

  await bar.getByRole("button", { name: "Change" }).click();
  await page.getByRole("dialog", { name: "Link" }).getByLabel("Address").fill("mailto:team@example.com");
  await page.getByRole("dialog", { name: "Link" }).getByRole("button", { name: "Save" }).click();
  await expect(document(page).locator("a")).toHaveAttribute("href", "mailto:team@example.com");

  await expect(page.getByText("Saved", { exact: true })).toBeVisible();
  const agent = await agentOf(page);
  const read = await agent("read", { id: made.id });
  expect(JSON.stringify(read.structured)).toContain("[Read the docs here.](mailto:team@example.com)");

  await document(page).locator("a").click();
  await page.getByRole("group", { name: "Link" }).getByRole("button", { name: "Remove" }).click();
  await expect(document(page).locator("a")).toHaveCount(0);
  await expect(block(page, "Read the docs here.")).toHaveText("Read the docs here.");
});

test("Cancel leaves the text as it was", async ({ page }) => {
  await signIn(page, "alice");
  const made = await publishDoc(page, "Links cancel", skeleton);
  await openEditor(page, made.id);

  await block(page, "Read the docs here.").click({ clickCount: 3 });
  await page.keyboard.press("ControlOrMeta+k");
  await page.getByRole("dialog", { name: "Link" }).getByLabel("Address").fill("https://example.com");
  await page.getByRole("dialog", { name: "Link" }).getByRole("button", { name: "Cancel" }).click();
  await expect(page.getByRole("dialog", { name: "Link" })).toHaveCount(0);
  await expect(document(page).locator("a")).toHaveCount(0);
});

test("an agent's Markdown link with a script address is refused with the line", async ({ page }) => {
  await signIn(page, "alice");
  const agent = await agentOf(page);
  const refused = await agent("publish", { title: "Bad link", kind: "doc", content: "# T\n\n[click](javascript:alert(1))\n" });
  expect(refused.isError).toBe(true);
  expect(refused.text).toMatch(/line 3/i);
});
