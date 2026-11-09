import { expect, signIn, test } from "./support";
import { agentOf, block, browse, document, expectBlock, openEditor, originOf, publishDoc, typeAtEnd, tool } from "./editor-support";

// Lists, quotes, code, rules and marks in the editor (docs/editor.md): what one
// person formats, the others see, and Undo only takes back the person's own.

const skeleton = "# Plan\n\nAlice block.\n\nBob block.\n";

test("alice makes a list while bob types in another block; both see both, and her Undo leaves bob's text", async ({ page, browser, watch }) => {
  await signIn(page, "alice");
  const made = await publishDoc(page, "Formatting together", skeleton, ["bob"]);
  const bob = await browse(browser, watch, originOf(page), "bob");
  await openEditor(page, made.id);
  await openEditor(bob.page, made.id);

  await block(page, "Alice block.").click({ clickCount: 3 });
  await (await tool(page, "Bullet list")).click();
  await typeAtEnd(bob.page, "Bob block.", " Bob typed.");

  for (const seen of [page, bob.page]) {
    await expect(document(seen).locator("ul > li").filter({ hasText: "Alice block." })).toHaveCount(1);
    await expectBlock(seen, "Bob block.", "Bob block. Bob typed.");
  }

  await (await tool(page, "Undo")).click();
  for (const seen of [page, bob.page]) {
    await expect(document(seen).locator("ul")).toHaveCount(0);
    await expect(block(seen, "Alice block.")).toHaveText("Alice block.");
    await expectBlock(seen, "Bob block.", "Bob block. Bob typed.");
  }
  await bob.context.close();
});

test("a ticked to-do is still ticked after a reload and is in the Markdown an agent reads", async ({ page }) => {
  await signIn(page, "alice");
  const made = await publishDoc(page, "To-dos", skeleton);
  await openEditor(page, made.id);

  await block(page, "Alice block.").click();
  await page.getByRole("button", { name: "To-do list" }).click();
  await document(page).locator("input.ed-check").check();
  await expect(document(page).locator("li.ed-done")).toHaveCount(1);
  await expect(page.getByText("Saved", { exact: true })).toBeVisible();

  await page.reload();
  await expect(page.locator(".ed[data-connection=live]")).toBeVisible();
  await expect(document(page).locator("input.ed-check")).toBeChecked();

  const agent = await agentOf(page);
  const read = await agent("read", { id: made.id });
  expect(read.isError, read.text).toBe(false);
  expect(JSON.stringify(read.structured)).toContain("- [x] Alice block.");
});

test("a selected word is bold from the bubble and still bold after a reload, the marks also apply with their shortcuts, and quote, code and rule are blocks of the doc", async ({ page }) => {
  await signIn(page, "alice");
  const made = await publishDoc(page, "Marks", skeleton);
  await openEditor(page, made.id);

  await block(page, "Alice block.").click({ clickCount: 3 });
  await expect(page.getByRole("toolbar", { name: "Format" })).toBeVisible();
  await (await tool(page, "Bold")).click();
  await expect(document(page).locator("p strong")).toHaveText("Alice block.");
  await page.keyboard.press("ControlOrMeta+i");
  await (await tool(page, "Underline")).click();
  await (await tool(page, "Code")).click();
  await (await tool(page, "Strikethrough")).click();
  await (await tool(page, "Highlight")).click();
  for (const tag of ["em", "u", "s", "mark", "code"]) await expect(document(page).locator(`p ${tag}`)).toHaveText("Alice block.");

  await block(page, "Bob block.").click();
  await (await tool(page, "Quote")).click();
  await expect(document(page).locator("blockquote")).toContainText("Bob block.");
  await (await tool(page, "Code block")).click();
  await expect(document(page).locator("blockquote pre")).toContainText("Bob block.");
  await (await tool(page, "Horizontal rule")).click();
  await expect(document(page).locator("hr")).toHaveCount(1);
  await expect(page.getByText("Saved", { exact: true })).toBeVisible();

  await page.reload();
  await expect(page.locator(".ed[data-connection=live]")).toBeVisible();
  await expect(document(page).locator("p strong")).toHaveText("Alice block.");

  const agent = await agentOf(page);
  const read = await agent("read", { id: made.id });
  const markdown = JSON.stringify(read.structured);
  expect(markdown).toContain("---");
  expect(markdown).toContain("> ");
});

test("blocks an agent writes as Markdown, and adds with doc_edit, are the same lists, quotes, code and marks in the editor", async ({ page }) => {
  await signIn(page, "alice");
  const agent = await agentOf(page);
  const markdown = "# Agent\n\n- one\n- two\n  - nested\n\n1. first\n2. second\n\n- [x] done\n- [ ] todo\n\n> quoted\n\n```\ncode here\n```\n\n---\n\n**bold** *italic* ~~gone~~ `mono`\n";
  const made = await agent("publish", { title: "From Markdown", kind: "doc", content: markdown });
  expect(made.isError, made.text).toBe(false);
  const id = made.structured["id"] as string;
  await openEditor(page, id);

  const doc = document(page);
  await expect(doc.locator("ul > li").filter({ hasText: /^one$/ })).toHaveCount(1);
  await expect(doc.locator("ul ul li")).toHaveText("nested");
  await expect(doc.locator("ol > li")).toHaveCount(2);
  await expect(doc.locator("input.ed-check")).toHaveCount(2);
  await expect(doc.locator("input.ed-check").first()).toBeChecked();
  await expect(doc.locator("input.ed-check").last()).not.toBeChecked();
  await expect(doc.locator("blockquote")).toHaveText("quoted");
  await expect(doc.locator("pre")).toHaveText("code here");
  await expect(doc.locator("hr")).toHaveCount(1);
  for (const tag of ["strong", "em", "s", "code"]) await expect(doc.locator(`p ${tag}`)).toHaveCount(1);

  const read = await agent("read", { id, view: "blocks" });
  const first = (read.structured.tabs as { blocks: { id: string }[] }[])[0]?.blocks[0];
  expect(first).toBeDefined();
  const edited = await agent("doc_edit", { id, ops: [{ op: "insert", after: first?.id, markdown: "- [ ] added by the agent\n" }] });
  expect(edited.isError, edited.text).toBe(false);
  await expect(doc.locator("li.ed-task").filter({ hasText: "added by the agent" })).toHaveCount(1);
  await expect(doc.locator("input.ed-check")).toHaveCount(3);
});
