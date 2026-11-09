import type { Page } from "@playwright/test";
import { expect, signIn, test } from "./support";
import { agentOf, block, browse, openEditor, originOf, publishDoc, typeAtEnd } from "./editor-support";

// Comments on words in the editor (docs/editor.md): one journey across three browsers and an agent. How an anchor follows
// edits, moves and deletions is proven in vitest (anchor.test.ts); this is that it works with a real room, selection and panel.

const skeleton = "# Plan\n\nIntro line.\n\nThe quick brown fox.\n\nA closing line.\n";

const panel = (page: Page) => page.getByRole("complementary", { name: "Comments" });
const marked = (page: Page) => page.locator(".ed-doc .ed-commented");

/** Selects the words of a block, as a person dragging across them does: the browser's selection, which the editor follows. */
async function selectWords(page: Page, text: string): Promise<void> {
  await block(page, text).evaluate((el) => {
    const range = el.ownerDocument.createRange();
    range.selectNodeContents(el);
    const selection = el.ownerDocument.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
  });
}

test("alice comments on a sentence, bob types above it and the mark stays; a reader comments too, and an agent reads where", async ({ page, browser, watch }) => {
  await signIn(page, "alice");
  const made = await publishDoc(page, "Commented plan", skeleton, ["bob"]);
  const bob = await browse(browser, watch, originOf(page), "bob");
  const carol = await browse(browser, watch, originOf(page), "carol");
  await openEditor(page, made.id);

  await block(page, "The quick brown fox.").click();
  await selectWords(page, "The quick brown fox.");
  await page.getByRole("toolbar", { name: "Format", exact: true }).getByRole("button", { name: "Comment", exact: true }).click();
  await expect(panel(page).getByText("Commenting on")).toBeVisible();
  await panel(page).getByRole("textbox", { name: "Write a comment" }).fill("Which fox?");
  await panel(page).getByRole("button", { name: "Comment", exact: true }).click();
  await expect(panel(page).getByRole("article")).toContainText("Which fox?");

  // The others open the doc afterwards (a comment made while a viewer is open reaches it with the viewer's next poll, every 10 seconds):
  // the words are marked for the other editor and for the reader, who has no toolbar.
  await openEditor(bob.page, made.id);
  await openEditor(carol.page, made.id);
  await expect(marked(page)).toHaveText("The quick brown fox.");
  await expect(marked(bob.page)).toHaveText("The quick brown fox.");
  await expect(marked(carol.page)).toHaveText("The quick brown fox.");

  // Bob types above the sentence: the mark stays on it in both browsers.
  await typeAtEnd(bob.page, "Intro line.", " Bob added a lot of words before the fox here.");
  await expect(block(page, "Intro line.")).toContainText("Bob added");
  await expect(marked(page)).toHaveText("The quick brown fox.");
  await expect(marked(bob.page)).toHaveText("The quick brown fox.");

  // Carol may only read the document and can still comment on it; clicking a mark opens its thread.
  await expect(carol.page.getByRole("toolbar")).toHaveCount(0);
  await marked(carol.page).click();
  await expect(panel(carol.page).getByRole("article")).toContainText("Which fox?");
  await selectWords(carol.page, "A closing line.");
  await carol.page.getByRole("toolbar", { name: "Format", exact: true }).getByRole("button", { name: "Comment", exact: true }).click();
  await panel(carol.page).getByRole("textbox", { name: "Write a comment" }).fill("Why a closing?");
  await panel(carol.page).getByRole("button", { name: "Comment", exact: true }).click();
  await expect(panel(carol.page).getByRole("article").filter({ hasText: "Why a closing?" })).toBeVisible();

  // An agent reads both anchors: the tab, the block and the words.
  const agent = await agentOf(page);
  const read = await agent("read", { id: made.id, view: "blocks" });
  const blocks = (read.structured.tabs as { id: string; blocks: { id: string; type: string }[] }[])[0];
  const comments = (await agent("comments", { id: made.id })).structured.comments as { body: string; forAgent: boolean; anchor: { tab: string; block: string; quote: string } }[];
  const fox = comments.find((c) => c.body === "Which fox?");
  expect(fox).toMatchObject({ forAgent: true, anchor: { tab: blocks?.id, quote: "The quick brown fox." } });
  expect(blocks?.blocks.map((b) => b.id)).toContain(fox?.anchor.block);
  expect(comments.find((c) => c.body === "Why a closing?")?.anchor.quote).toBe("A closing line.");

  await bob.context.close();
  await carol.context.close();
});
