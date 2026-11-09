import type { Page } from "@playwright/test";
import { expect, signIn, test } from "./support";
import { agentOf, block, document as editor, openEditor, publishDoc } from "./editor-support";

// Rich content copied from a web page (docs/editor.md, #73): the browser's own
// copy puts text/html on the clipboard and the browser's own paste hands it to
// the editor. Nothing here dispatches a paste event by hand.

test.use({ permissions: ["clipboard-read", "clipboard-write"] });

const article = `
<article id="copied" style="font-family: Georgia, serif">
  <p>Releases are cut every <strong>Friday</strong> after the checks pass.</p>
  <ul>
    <li>Run the checks</li>
    <li>Tag the commit</li>
  </ul>
  <p>Read the <a href="https://example.com/handbook">release handbook</a> for the details.</p>
</article>`;

/** Copies the article of a web page, as a person does: select it and press Copy. */
async function copyFromWebPage(web: Page): Promise<void> {
  // A page on its own https origin, as a web page is (the clipboard API needs a secure context).
  await web.route("https://news.example.test/**", (route) =>
    route.fulfill({ contentType: "text/html", body: `<!doctype html><title>A web page</title><body>${article}</body>` }));
  await web.goto("https://news.example.test/releases");
  await web.evaluate(() => {
    const range = document.createRange();
    range.selectNodeContents(document.getElementById("copied") as HTMLElement);
    const selection = window.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
  });
  await web.keyboard.press("ControlOrMeta+c");
  const types = await web.evaluate(async () => (await navigator.clipboard.read()).flatMap((item) => item.types));
  expect(types).toContain("text/html");
}

async function expectPasted(page: Page): Promise<void> {
  await expect(block(page, "Releases are cut every Friday after the checks pass.")).toBeVisible();
  await expect(editor(page).locator("ul > li")).toHaveText(["Run the checks", "Tag the commit"]);
  await expect(editor(page).locator("a")).toHaveCount(1);
  await expect(editor(page).locator("a")).toHaveText("release handbook");
  await expect(editor(page).locator("a")).toHaveAttribute("href", "https://example.com/handbook");
  await expect(block(page, "for the details.")).toBeVisible();
}

test("a paragraph, a bulleted list and a link copied from a web page paste as blocks, and are still there after a reload", async ({ page, context }) => {
  await signIn(page, "alice");
  const made = await publishDoc(page, "Pasted from the web", "# Notes\n\nStart here:\n");
  await openEditor(page, made.id);

  const web = await context.newPage();
  await copyFromWebPage(web);
  await web.close();
  await page.bringToFront();

  await block(page, "Start here:").click();
  await page.keyboard.press("End");
  await page.keyboard.press("Enter");
  await page.keyboard.press("ControlOrMeta+v");
  await expectPasted(page);
  await expect(page.getByText("Saved", { exact: true })).toBeVisible();

  const agent = await agentOf(page);
  const read = await agent("read", { id: made.id });
  const markdown = JSON.stringify(read.structured);
  expect(markdown).toContain("- Run the checks");
  expect(markdown).toContain("- Tag the commit");
  expect(markdown).toContain("[release handbook](https://example.com/handbook)");

  await page.reload();
  await expect(page.locator(".ed[data-connection=live]")).toBeVisible();
  await expectPasted(page);
});
