import { expect, presented, signIn, test } from "./support";
import { agentOf, block, blocksOf, expectBlock, browse, caretOf, document, openEditor, originOf, publishDoc, typeAtEnd, tool } from "./editor-support";

// The first editor of a doc: people typing together through the real socket
// (docs/editor.md). Each person is a browser of their own.

const skeleton = "# Plan\n\nFirst paragraph.\n\nSecond paragraph.\n";

test("alice and bob type in different blocks and both see both edits, each other's caret and name, and Saved", async ({ page, browser, watch }) => {
  await signIn(page, "alice");
  const made = await publishDoc(page, "Together", skeleton, ["bob"]);
  await page.goto(`/a/${made.id}`);
  const alice = { page };
  const bob = await browse(browser, watch, originOf(page), "bob");

  await openEditor(alice.page, made.id);
  await openEditor(bob.page, made.id);

  await typeAtEnd(alice.page, "First paragraph.", " Alice was here.");
  await typeAtEnd(bob.page, "Second paragraph.", " Bob was here.");

  for (const seen of [alice.page, bob.page]) {
    await expectBlock(seen, "First paragraph.", "First paragraph. Alice was here.");
    await expectBlock(seen, "Second paragraph.", "Second paragraph. Bob was here.");
  }
  // The names are the ones the server knows them by, not what the browser says.
  await expect(caretOf(bob.page, "ALICE")).toBeVisible();
  await expect(caretOf(alice.page, "BOB")).toBeVisible();
  await expect(alice.page.getByRole("group", { name: "Viewing now" })).toContainText("BOB");
  await expect(bob.page.getByRole("group", { name: "Viewing now" })).toContainText("ALICE");
  await expect(alice.page.getByText("Saved", { exact: true })).toBeVisible();
  await expect(bob.page.getByText("Saved", { exact: true })).toBeVisible();
  // Real ProseMirror + Yjs + MCP while Chromium's input protocol holds a composition.
  const agent = await agentOf(page, "claude-code");
  await block(page, "First paragraph.").click();
  await page.keyboard.press("End");
  const ime = await page.context().newCDPSession(page);
  await ime.send("Input.imeSetComposition", { text: "ㄋㄧˇ", selectionStart: 3, selectionEnd: 3 });
  await expectBlock(bob.page, "First paragraph.", "First paragraph. Alice was here.ㄋㄧˇ");
  const read = await agent("read", { id: made.id, view: "blocks" });
  const first = (read.structured.tabs as { blocks: { id: string; hash: string; content?: { text?: string }[] }[] }[])[0]?.blocks.find((b) => b.content?.[0]?.text === "First paragraph. Alice was here.ㄋㄧˇ");
  expect(first).toBeDefined();
  // A guarded edit preserves the observed composing text; replacing stale content would intentionally remove it.
  const changed = await agent("doc_edit", { id: made.id, ops: [{ op: "replace", id: first?.id, ifHash: first?.hash, markdown: "AGENT First paragraph. Alice was here.ㄋㄧˇ" }] });
  expect(changed.isError, changed.text).toBe(false);
  await expectBlock(bob.page, "First paragraph.", "AGENT First paragraph. Alice was here.ㄋㄧˇ");
  await expect(block(page, "First paragraph.")).toContainText("ㄋㄧˇ");
  await ime.send("Input.insertText", { text: "你好" });
  await expectBlock(page, "First paragraph.", "AGENT First paragraph. Alice was here.你好");
  await ime.send("Input.insertText", { text: "A👍🏽👩‍💻𠮷𪚥Z" });
  for (const grapheme of ["Z", "𪚥", "𠮷", "👩‍💻", "👍🏽", "A"]) {
    await page.keyboard.press("Shift+ArrowLeft");
    await expect.poll(() => page.evaluate(() => getSelection()?.toString())).toBe(grapheme);
    await page.keyboard.press("ArrowLeft");
  }
  await page.keyboard.press("End");
  await page.keyboard.press("ArrowLeft");
  await page.keyboard.press("ArrowLeft");
  await ime.send("Input.insertText", { text: "|" });
  await expectBlock(page, "First paragraph.", "AGENT First paragraph. Alice was here.你好A👍🏽👩‍💻𠮷|𪚥Z");
  await page.keyboard.press("Backspace");
  for (const suffix of ["A👍🏽👩‍💻𪚥Z", "A👍🏽𪚥Z", "A𪚥Z"]) {
    await page.keyboard.press("Backspace");
    for (const seen of [page, bob.page]) await expectBlock(seen, "First paragraph.", `AGENT First paragraph. Alice was here.你好${suffix}`);
  }
  const stored = await agent("read", { id: made.id });
  expect(stored.structured.content).toContain("你好A𪚥Z");
  await page.reload();
  await expectBlock(page, "First paragraph.", "AGENT First paragraph. Alice was here.你好A𪚥Z");
  await ime.detach();
  // A mention crosses the picker, the real socket and both browser documents.
  await typeAtEnd(alice.page, "First paragraph.", " @da");
  await alice.page.getByRole("option").filter({ hasText: "DANA" }).click();
  for (const seen of [alice.page, bob.page]) {
    await expect(document(seen).locator(".ed-mention")).toHaveText("@DANA");
  }
  await expect(alice.page.getByText("Saved", { exact: true })).toBeVisible();
  for (const seen of [alice.page, bob.page]) {
    await seen.reload();
    await expect(document(seen).locator(".ed-mention")).toHaveText("@DANA");
  }
  const reader = await browse(browser, watch, originOf(page), "carol");
  await openEditor(reader.page, made.id);
  await expect(document(reader.page)).toHaveAttribute("contenteditable", "false");
  const storedName = document(reader.page).locator(".ed-mention");
  await expect(storedName).toHaveText("@DANA");
  await expect(storedName).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  await expect(storedName).toHaveCSS("color", "rgb(102, 102, 102)");
  await reader.context.close();
  await bob.context.close();
});

test("an editor opens a doc and types with no Edit button, the text survives a reload, and a reader sees the same doc with no insert row", async ({ page, browser, watch }) => {
  await signIn(page, "alice");
  const made = await publishDoc(page, "Watched", skeleton);
  const carol = await browse(browser, watch, originOf(page), "carol");
  await openEditor(page, made.id);
  await openEditor(carol.page, made.id);

  await expect(page.getByRole("button", { name: "Edit" })).toHaveCount(0);
  await expect(page.getByRole("toolbar", { name: "Editing tools" })).toBeVisible();
  await expect(carol.page.getByRole("toolbar")).toHaveCount(0);
  await expect(carol.page.getByRole("button", { name: "Watch live" })).toHaveCount(0);
  await typeAtEnd(page, "First paragraph.", " Live.");
  await expectBlock(carol.page, "First paragraph.", "First paragraph. Live.");
  await expect(caretOf(carol.page, "ALICE")).toBeVisible();

  await expect(document(carol.page)).toHaveAttribute("contenteditable", "false");
  await block(carol.page, "Second paragraph.").click();
  await carol.page.keyboard.press("End");
  await carol.page.keyboard.type("carol typed this");
  await expect(carol.page.getByText("carol typed this")).toHaveCount(0);
  await expectBlock(page, "Second paragraph.", "Second paragraph.");

  await expect(page.getByText("Saved", { exact: true })).toBeVisible();
  await page.reload();
  await expect(page.locator(".ed[data-connection=live]")).toBeVisible();
  await expectBlock(page, "First paragraph.", "First paragraph. Live.");

  // Inline fields cross the native picker, live reader and saved document.
  await block(page, "First paragraph.").click();
  await page.keyboard.press("End");
  await page.getByRole("button", { name: "Insert block", exact: true }).click();
  await page.getByRole("combobox", { name: "Find a block" }).fill("Date");
  await page.keyboard.press("Enter");
  const dateForm = page.locator(".ed-date-tools");
  const dateInput = dateForm.locator('input[type="date"]');
  await expect(dateInput).toBeVisible();
  await expect(dateInput).toBeFocused();
  await dateInput.fill("2026-10-24");
  await dateForm.getByRole("button", { name: "Save", exact: true }).click();

  await typeAtEnd(page, "Second paragraph.", " /");
  await page.getByRole("combobox", { name: "Find a block" }).fill("Dropdown");
  await page.keyboard.press("Enter");
  const picker = page.getByRole("dialog", { name: "Dropdown", exact: true });
  await expect(picker).toBeVisible();
  await presented(picker);
  await picker.getByLabel("Options 1", { exact: true }).fill("Planned");
  await picker.getByLabel("Options 2", { exact: true }).fill("Done");
  await picker.getByLabel("Selected option").selectOption("1");
  await picker.getByRole("button", { name: "Save", exact: true }).click();
  await expect(document(page).locator(".ed-dropdown-trigger")).toHaveText("Done");
  for (const seen of [page, carol.page]) {
    await expect(document(seen).locator('.ed-date[datetime="2026-10-24"]')).toHaveText("2026-10-24");
    await expect(document(seen).locator(".ed-date")).toBeVisible();
  }
  await expect(document(carol.page).locator(".ed-dropdown > span")).toHaveText("Done");
  await expect(document(carol.page).locator(".ed-dropdown > span")).toBeVisible();
  await expect(page.getByText("Saved", { exact: true })).toBeVisible();

  for (const seen of [page, carol.page]) {
    await seen.reload();
    await expect(seen.locator(".ed[data-connection=live]")).toBeVisible();
    await expect(document(seen).locator(".ed-date")).toHaveText("2026-10-24");
    // Ctrl+P must keep the selected label even when the editable trigger is hidden.
    await seen.emulateMedia({ media: "print" });
    const selected = document(seen).locator(".ed-dropdown > span");
    await expect(selected).toHaveText("Done");
    await expect(selected).toBeVisible();
    await seen.emulateMedia({ media: "screen" });
  }
  await expect(document(page).locator(".ed-dropdown-trigger")).toHaveText("Done");
  await expect(document(carol.page)).toHaveAttribute("contenteditable", "false");
  await expect(carol.page.getByRole("toolbar")).toHaveCount(0);
  await carol.context.close();
});

test("offline for one browser, edits on both sides, and after it reconnects nothing is lost", async ({ page, browser, watch }) => {
  await signIn(page, "alice");
  const made = await publishDoc(page, "Offline", skeleton, ["bob"]);
  const bob = await browse(browser, watch, originOf(page), "bob");
  await openEditor(page, made.id);
  await openEditor(bob.page, made.id);

  await bob.context.setOffline(true);
  await expect(bob.page.locator(".ed[data-connection=offline]")).toBeVisible();
  await expect(bob.page.getByText("You are offline.")).toBeVisible();
  await typeAtEnd(bob.page, "Second paragraph.", " Bob offline.");
  await expect(bob.page.getByText("Not saved yet")).toBeVisible();
  await typeAtEnd(page, "First paragraph.", " Alice online.");

  await bob.context.setOffline(false);
  await expect(bob.page.locator(".ed[data-connection=live]")).toBeVisible();
  const both = ["Plan", "First paragraph. Alice online.", "Second paragraph. Bob offline."];
  await expect.poll(() => blocksOf(page)).toEqual(both);
  await expect.poll(() => blocksOf(bob.page)).toEqual(both);
  await expect(bob.page.getByText("Saved", { exact: true })).toBeVisible();
  await bob.context.close();
});

test("undo in one browser leaves the other person's text in place", async ({ page, browser, watch }) => {
  await signIn(page, "alice");
  const made = await publishDoc(page, "Two people typing", skeleton, ["bob"]);
  const bob = await browse(browser, watch, originOf(page), "bob");
  await openEditor(page, made.id);
  await openEditor(bob.page, made.id);

  await typeAtEnd(page, "First paragraph.", " Alice.");
  await expectBlock(bob.page, "First paragraph.", "First paragraph. Alice.");
  await typeAtEnd(bob.page, "First paragraph.", " Bob.");
  await expectBlock(page, "First paragraph.", "First paragraph. Alice. Bob.");

  await (await tool(page, "Undo")).click();
  await expectBlock(page, "First paragraph.", "First paragraph. Bob.");
  await expectBlock(bob.page, "First paragraph.", "First paragraph. Bob.");
  await bob.context.close();
});

test("the editor is a chunk of its own: the gallery does not load it, a doc's page does", async ({ page }) => {
  const scripts: string[] = [];
  page.on("request", (r) => {
    if (r.resourceType() === "script") scripts.push(new URL(r.url()).pathname);
  });
  await signIn(page, "alice");
  const made = await publishDoc(page, "Chunks", skeleton);
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Artifacts" })).toBeVisible();
  expect(scripts.filter((s) => s.startsWith("/assets/")).length).toBeGreaterThan(0);
  expect(scripts.filter((s) => /DocEditor/.test(s))).toEqual([]);

  await page.goto(`/a/${made.id}`);
  await expect(page.locator(".ed[data-connection=live]")).toBeVisible();
  expect(scripts.filter((s) => /DocEditor/.test(s))).toHaveLength(1);
});

test("an agent's document opens in the editor and its blocks are the ones the agent wrote", async ({ page }) => {
  await signIn(page, "alice");
  const agent = await agentOf(page);
  const made = await agent("publish", { title: "From an agent", kind: "doc", content: skeleton });
  expect(made.isError, made.text).toBe(false);
  await openEditor(page, made.structured["id"] as string);
  await expect.poll(() => blocksOf(page)).toEqual(["Plan", "First paragraph.", "Second paragraph."]);
});

test("an agent's edit shows its name at the top right and on the block it changed to two browsers, then fades", async ({ page, browser, watch }) => {
  test.setTimeout(90_000);
  await signIn(page, "alice");
  const made = await publishDoc(page, "Agent at work", skeleton, ["bob"]);
  const bob = await browse(browser, watch, originOf(page), "bob");
  await openEditor(page, made.id);
  await openEditor(bob.page, made.id);

  const agent = await agentOf(page, "claude-code");
  const read = await agent("read", { id: made.id, view: "blocks" });
  const blocks = (read.structured.tabs as { blocks: { id: string; content?: { text: string }[] }[] }[])[0]?.blocks ?? [];
  const target = blocks.find((b) => b.content?.[0]?.text === "Second paragraph.");
  expect(target, "the agent reads the block it will change").toBeDefined();
  const edited = await agent("doc_edit", { id: made.id, ops: [{ op: "replace", id: target?.id, markdown: "Second paragraph, by the agent." }] });
  expect(edited.isError, edited.text).toBe(false);
  expect(edited.structured.saved).toBe(false);

  const name = "alice 的 Claude Code";
  for (const seen of [page, bob.page]) {
    await expectBlock(seen, "Second paragraph", "Second paragraph, by the agent.");
    await expect(seen.locator(`.ed-agent-block[data-agent="${name}"]`)).toContainText("Second paragraph, by the agent.");
    await expect(seen.getByRole("group", { name: "Viewing now" })).toContainText(name);
  }
  // The block mark fades after a few seconds and the agent leaves the top right when it is idle.
  for (const seen of [page, bob.page]) {
    await expect(seen.locator(".ed-agent-block")).toHaveCount(0, { timeout: 10_000 });
    await expect(seen.getByRole("group", { name: "Viewing now" })).not.toContainText(name, { timeout: 45_000 });
  }
  await bob.context.close();
});

test("two people's edits become one version that names both, and the open panel shows it as current without a reload", async ({ page, browser, watch }) => {
  test.setTimeout(90_000);
  await signIn(page, "alice");
  const made = await publishDoc(page, "Versioned", skeleton, ["bob"]);
  const bob = await browse(browser, watch, originOf(page), "bob");
  await openEditor(page, made.id);
  await openEditor(bob.page, made.id);

  // The history is open while the editors work: it has to learn of the version the room cuts.
  await page.getByRole("button", { name: "More actions" }).click();
  await page.getByRole("menuitem", { name: "Versions" }).click();
  const versions = page.getByRole("complementary", { name: "Versions" });
  await expect(versions.getByRole("listitem")).toHaveCount(1);

  // Alice keeps editing until bob has made his edit, so the room is never quiet between them and cuts one version.
  let bobDone = false;
  const alice = (async () => {
    await block(page, "First paragraph.").click();
    // The caret is in the document before the first key: a key typed while focus is elsewhere is lost without a trace.
    await expect(document(page)).toBeFocused();
    await page.keyboard.press("End");
    while (!bobDone) await page.keyboard.type(".", { delay: 20 });
  })();
  // Bob starts once alice's first character is in the document, so both are editing at the same time.
  await expect(block(bob.page, "First paragraph.")).toHaveText(/^First paragraph\.\./);
  await typeAtEnd(bob.page, "Second paragraph.", " Bob.");
  bobDone = true;
  await alice;

  // The room goes quiet (the smoke server shortens the pause) and becomes version 2, by both.
  const newest = versions.getByRole("listitem").first();
  await expect(newest).toContainText("v2", { timeout: 40_000 });
  await expect(newest).toContainText("alice");
  await expect(newest).toContainText("bob");
  await expect(newest.getByText("Current")).toBeVisible();
  await expect(page.locator(".vbar .facts").getByText("v2", { exact: true })).toBeVisible();
  await expect(versions.getByRole("listitem").nth(1).getByText("Current")).toHaveCount(0);
  await bob.context.close();
});

test("a person who clicks into a paragraph while someone else is typing elsewhere keeps their caret there", async ({ page, browser, watch }) => {
  await signIn(page, "alice");
  const made = await publishDoc(page, "Caret", skeleton, ["bob"]);
  const bob = await browse(browser, watch, originOf(page), "bob");
  await openEditor(page, made.id);
  await openEditor(bob.page, made.id);

  // Alice's keystrokes reach Bob's browser while he clicks and types: his caret must not jump back.
  await block(page, "First paragraph.").click();
  await page.keyboard.press("End");
  const typing = page.keyboard.type("a".repeat(60), { delay: 5 });
  await block(bob.page, "Second paragraph.").click();
  await bob.page.keyboard.press("End");
  await bob.page.keyboard.type(" Bob was here.");
  await typing;

  const want = ["Plan", `First paragraph.${"a".repeat(60)}`, "Second paragraph. Bob was here."];
  await expect.poll(() => blocksOf(bob.page)).toEqual(want);
  await expect.poll(() => blocksOf(page)).toEqual(want);
  await bob.context.close();
});
