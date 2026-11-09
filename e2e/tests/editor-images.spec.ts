import type { Page } from "@playwright/test";
import { expect, signIn, test } from "./support";
import { agentOf, block, browse, document, openEditor, originOf, publishDoc, tool } from "./editor-support";

// Pictures in the editor (docs/editor.md): pasted, picked and dropped files become uploads of the doc;
// size, alt text and Delete; the same picture for a second person, in the viewer's sandbox and for an agent.

// A 2x2 PNG.
const png = "iVBORw0KGgoAAAANSUhEUgAAAAIAAAACCAYAAABytg0kAAAAFklEQVQI12P8z8Dwn4GBgYGJAQoAHhgCAh6X4CYAAAAASUVORK5CYII=";

const pictures = (page: Page) => document(page).locator("img");

/** What a browser does when a screenshot is pasted: a clipboard with one file and no text. */
async function pastePicture(page: Page, name = "image.png"): Promise<void> {
  await document(page).evaluate(
    (el, args) => {
      const bytes = Uint8Array.from(atob(args.png), (c) => c.charCodeAt(0));
      const data = new DataTransfer();
      data.items.add(new File([bytes], args.name, { type: "image/png" }));
      el.dispatchEvent(new ClipboardEvent("paste", { clipboardData: data, bubbles: true, cancelable: true }));
    },
    { png, name },
  );
}

test("paste a picture from the clipboard, set its alt text and size, reload: it is still there, and a second browser sees it live", async ({ page, browser, watch }) => {
  await signIn(page, "alice");
  const made = await publishDoc(page, "Pictures", "# Pictures\n\nBefore.\n", ["bob"]);
  const bob = await browse(browser, watch, originOf(page), "bob");
  await openEditor(page, made.id);
  await openEditor(bob.page, made.id);

  await block(page, "Before.").click();
  await page.keyboard.press("End");
  await pastePicture(page);
  await expect(pictures(page)).toHaveCount(1);
  await expect(pictures(page)).toHaveAttribute("src", new RegExp(`^/files/${made.id}/image-[0-9a-f]{8}\\.png$`));
  await expect.poll(() => pictures(page).evaluate((img: HTMLImageElement) => img.naturalWidth)).toBe(2);
  await expect(pictures(bob.page)).toHaveCount(1);

  const tools = page.getByRole("toolbar", { name: "Image tools" });
  await tools.getByLabel("Alt text").fill("Team photo");
  await tools.getByLabel("Alt text").press("Enter");
  await tools.getByRole("button", { name: "Extra large" }).click();
  for (const seen of [page, bob.page]) {
    await expect(pictures(seen)).toHaveAttribute("alt", "Team photo");
    await expect(pictures(seen)).toHaveAttribute("width", "672");
  }
  await expect(page.getByText("Saved", { exact: true })).toBeVisible();

  await page.reload();
  await expect(page.locator(".ed[data-connection=live]")).toBeVisible();
  await expect(pictures(page)).toHaveAttribute("alt", "Team photo");
  await expect(pictures(page)).toHaveAttribute("width", "672");

  const agent = await agentOf(page);
  const read = await agent("read", { id: made.id });
  expect(JSON.stringify(read.structured)).toMatch(new RegExp(`!\\[Team photo\\]\\(/files/${made.id}/image-[0-9a-f]{8}\\.png \\\\"xlarge\\\\"\\)`));
  await bob.context.close();
});

test("two pictures pasted one after the other are two files: the second does not replace the first", async ({ page }) => {
  await signIn(page, "alice");
  const made = await publishDoc(page, "Two pictures", "# Two\n\nText.\n");
  await openEditor(page, made.id);
  await block(page, "Text.").click();
  await pastePicture(page);
  await expect(pictures(page)).toHaveCount(1);
  await block(page, "Text.").click();
  await pastePicture(page);
  await expect(pictures(page)).toHaveCount(2);
  const sources = await pictures(page).evaluateAll((all) => all.map((img) => img.getAttribute("src")));
  expect(new Set(sources).size).toBe(2);
});

test("the Image button picks a file and Delete takes the picture out, Undo brings it back", async ({ page }) => {
  await signIn(page, "alice");
  const made = await publishDoc(page, "Picker", "# Picker\n\nText.\n");
  await openEditor(page, made.id);
  await block(page, "Text.").click();
  await page.getByTestId("image-picker").setInputFiles({ name: "chart.png", mimeType: "image/png", buffer: Buffer.from(png, "base64") });
  await expect(pictures(page)).toHaveCount(1);
  await expect(pictures(page)).toHaveAttribute("src", new RegExp(`^/files/${made.id}/chart-[0-9a-f]{8}\\.png$`));
  await page.waitForTimeout(700);
  await page.getByRole("toolbar", { name: "Image tools" }).getByRole("button", { name: "Delete image" }).click();
  await expect(pictures(page)).toHaveCount(0);
  await (await tool(page, "Undo")).click();
  await expect(pictures(page)).toHaveCount(1);
});

test("a dropped file becomes a picture, and a file the server refuses shows the server's reason and adds nothing", async ({ page, expectConsoleErrors }) => {
  // The browser logs the refused upload (400) itself; the reason below is what the person reads.
  expectConsoleErrors(/Failed to load resource.*400/);
  await signIn(page, "alice");
  const made = await publishDoc(page, "Drop", "# Drop\n\nText.\n");
  await openEditor(page, made.id);
  await document(page).evaluate(
    (el, args) => {
      const bytes = Uint8Array.from(atob(args.png), (c) => c.charCodeAt(0));
      const data = new DataTransfer();
      data.items.add(new File([bytes], "dropped.png", { type: "image/png" }));
      const box = el.getBoundingClientRect();
      el.dispatchEvent(new DragEvent("drop", { dataTransfer: data, clientX: box.left + 20, clientY: box.top + 20, bubbles: true, cancelable: true }));
    },
    { png },
  );
  await expect(pictures(page)).toHaveCount(1);

  await page.getByTestId("image-picker").setInputFiles({ name: "notes.png", mimeType: "image/png", buffer: Buffer.from("this is not a picture") });
  await expect(page.getByRole("alert")).toContainText("The picture was not added: notes-");
  await expect(page.getByRole("alert")).toContainText("this file is not an accepted type");
  await expect(pictures(page)).toHaveCount(1);
});

test("a picture an agent uploads and writes as Markdown shows in the editor and in the viewer's sandbox; once its file is deleted both show the placeholder", async ({ page, browser, watch, expectConsoleErrors }) => {
  // The editor asks for the deleted file and the browser logs the 404; the placeholder is what the person sees.
  expectConsoleErrors(/Failed to load resource.*404/);
  await signIn(page, "alice");
  const agent = await agentOf(page);
  const made = await publishDoc(page, "Agent picture", "# Agent\n\nText.\n");
  const up = await agent("upload_asset", { id: made.id, name: "chart.png", content_base64: png });
  expect(up.isError, up.text).toBe(false);
  // The address as a doc keeps it: this server is plain http here, and a doc takes a full address only over https.
  const url = new URL(up.structured["url"] as string).pathname;
  const edited = await agent("publish", { id: made.id, title: "Agent picture", kind: "doc", content: `# Agent\n\n![A chart](${url} "medium")\n` });
  expect(edited.isError, edited.text).toBe(false);

  await page.goto(`/a/${made.id}`);
  await expect(page.locator(".ed[data-connection=live]")).toBeVisible();
  await expect(page.locator(".ed[data-connection=live]")).toBeVisible();
  await expect(pictures(page)).toHaveAttribute("alt", "A chart");
  await expect.poll(() => pictures(page).evaluate((img: HTMLImageElement) => img.naturalWidth)).toBe(2);

  const deleted = await agent("delete_asset", { id: made.id, name: "chart.png" });
  expect(deleted.isError, deleted.text).toBe(false);
  // A browser that has not seen the picture (this one has it cached for a day).
  const again = await browse(browser, watch, originOf(page), "alice");
  await again.page.goto(`/a/${made.id}`);
  await expect(again.page.locator(".ed[data-connection=live]")).toBeVisible();
  await expect(again.page.locator(".ed[data-connection=live]")).toBeVisible();
  await expect(document(again.page).locator(".ed-missing")).toContainText("Picture deleted: A chart");
  await again.context.close();
});
