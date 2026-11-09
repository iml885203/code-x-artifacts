import { readFileSync } from "node:fs";
import type { Download, Page } from "@playwright/test";
import { agentOf, block, browse, caretOf, document, openEditor, originOf, publishDoc, typeAtEnd } from "./editor-support";
import { pdfPages, pdfPageSizes } from "./pdf";
import { expect, nameEditors, onScreen, pickVersion, presented, publish, signIn, test } from "./support";
import { extractZip } from "./zip";

type Agent = Awaited<ReturnType<typeof agentOf>>;
type Versions = { version: number; authors?: string[]; agents?: { by: string; client: string }[]; restoredFrom?: number }[];
const client = "claude-code";
const agentName = "alice 的 Claude Code";
const initialDoc = "# Acceptance\n\nDoc first.\n\nDoc second.\n\nThe review sentence.\n";
const manifest = JSON.stringify({ version: 1, pages: [{ id: "home", name: "Home", rows: [[
  { id: "hero", name: "Hero", file: "frames/hero.html", w: 800, h: 600 },
  { id: "footer", name: "Footer", file: "frames/footer.html", w: 390, h: 300 },
]] }] });
const initialHero = '<!doctype html><html><head></head><body data-pid="root" style="margin:0"><h1 id="title" data-pid="title" data-name="Title" style="position:absolute;left:40px;top:40px;margin:0;width:250px">Design title.</h1><p id="copy" data-pid="copy" data-name="Copy" style="position:absolute;left:40px;top:180px;margin:0;width:400px">Design body.</p></body></html>';
const heroFrame = (page: Page) => page.locator('.dframe iframe[title^="Hero,"]');
const hero = (page: Page) => heroFrame(page).contentFrame();
const comments = (page: Page) => page.getByRole("complementary", { name: "Comments" });

async function clickElement(page: Page, selector: string, twice = false) {
  const b = await onScreen(heroFrame(page), hero(page).locator(selector));
  if (twice) {
    const panel = page.getByRole("region", { name: "Element properties", exact: true });
    const wasOpen = await panel.isVisible();
    const width = await page.locator(".canvas-viewport").evaluate((el) => el.getBoundingClientRect().width);
    const pid = await hero(page).locator(selector).getAttribute("data-pid");
    expect(pid).toBeTruthy();
    await page.mouse.click(b.x + b.width / 2, b.y + b.height / 2);
    await expect(page.locator(`[data-selection="${pid ?? ""}"]`)).toHaveCount(1);
    if (!wasOpen) await page.locator(".canvas-bar-right").getByRole("button", { name: "Properties" }).click();
    await expect(panel).toBeVisible();
    if (!wasOpen) expect(await page.locator(".canvas-viewport").evaluate((el) => el.getBoundingClientRect().width)).toBeLessThan(width);
    const after = await onScreen(heroFrame(page), hero(page).locator(selector));
    expect(after.x).toBeCloseTo(b.x, 1);
    expect(after.y).toBeCloseTo(b.y, 1);
    expect(after.width).toBeCloseTo(b.width, 1);
    expect(after.height).toBeCloseTo(b.height, 1);
    // Complete the same native double-click after Properties has explicitly opened.
    await page.mouse.click(b.x + b.width / 2, b.y + b.height / 2, { clickCount: 2 });
  } else await page.mouse.click(b.x + b.width / 2, b.y + b.height / 2);
}

async function appendDesign(page: Page, selector: string, words: string) {
  await clickElement(page, selector, true);
  await expect(hero(page).locator(selector)).toHaveAttribute("contenteditable", "true");
  await page.keyboard.press("End");
  await page.keyboard.type(words);
  await page.keyboard.press("Enter");
  await expect(hero(page).locator(selector)).not.toHaveAttribute("contenteditable");
}

async function dragElement(page: Page, selector: string, dx: number) {
  await clickElement(page, selector);
  const b = await onScreen(heroFrame(page), hero(page).locator(selector));
  await page.mouse.move(b.x + b.width / 2, b.y + b.height / 2);
  await page.mouse.down();
  await page.mouse.move(b.x + b.width / 2 + dx, b.y + b.height / 2, { steps: 5 });
  await page.mouse.up();
}

async function openDesign(page: Page, id: string) {
  await page.goto(`/a/${id}`);
  await expect(hero(page).locator("#title")).toBeVisible();
  await expect(page.locator(".canvas-presence .ed-conn-live")).toBeAttached();
  await expect(page.locator(".canvas-presence .ed-conn-live")).toBeHidden();
}

async function versionRows(agent: Agent, id: string): Promise<Versions> {
  const result = await agent("versions", { id });
  expect(result.isError, result.text).toBe(false);
  return result.structured.versions as Versions;
}

async function storedText(agent: Agent, id: string, version: number, file?: string): Promise<string> {
  const result = await agent("read", { id, version, ...(file ? { file } : {}) });
  expect(result.isError, result.text).toBe(false);
  return file ? (result.structured.file as { text: string }).text : result.structured.content as string;
}

async function quietSnapshot(agent: Agent, id: string, marker: string, file?: string): Promise<number> {
  let version = 1;
  // CI's real handler shortens quiet to 2s; this is not the manual 120-second acceptance.
  await expect.poll(async () => {
    const rows = await versionRows(agent, id);
    version = rows[0]?.version ?? 1;
    return version > 1 ? await storedText(agent, id, version, file) : "";
  }, { timeout: 40_000 }).toContain(marker);
  const rows = await versionRows(agent, id);
  expect(rows.flatMap((row) => row.authors ?? [])).toEqual(expect.arrayContaining(["alice", "bob"]));
  expect(rows.flatMap((row) => row.agents ?? [])).toContainEqual({ by: "alice", client: "Claude Code" });
  return version;
}

async function restoreFirst(page: Page, design: boolean, protect: () => Promise<void>) {
  await page.getByRole("button", { name: "More actions" }).click();
  await page.getByRole("menuitem", { name: "Versions" }).click();
  const panel = page.getByRole("complementary", { name: "Versions" });
  await pickVersion(panel, 1);
  await expect(page.getByText("Viewing v1, read only")).toBeVisible();
  if (design) await expect(page.locator("[data-handle]")).toHaveCount(0);
  else await expect(page.frameLocator("iframe[data-artifact-frame]").locator("[contenteditable]")).toHaveCount(0);
  await panel.getByRole("button", { name: "Restore", exact: true }).click();
  const confirm = page.getByRole("alertdialog");
  await presented(confirm);
  // Make the peer's protected edit after preparing the dialog, within the real quiet window.
  await protect();
  const [restored] = await Promise.all([
    page.waitForResponse((response) => response.url().endsWith("/versions/1/restore") && response.request().method() === "POST"),
    confirm.getByRole("button", { name: "Restore", exact: true }).click(),
  ]);
  expect(restored.status()).toBe(200);
  await expect(confirm).toBeHidden();
  await panel.getByRole("button", { name: "Close", exact: true }).click();
}

async function sendComment(page: Page, body: string) {
  await comments(page).getByRole("textbox", { name: "Write a comment" }).fill(body);
  await comments(page).getByRole("button", { name: "Comment", exact: true }).click();
  await expect(comments(page).getByRole("article").filter({ hasText: body })).toBeVisible();
}

async function download(page: Page, menu: string): Promise<Download> {
  await page.getByRole("button", { name: "More actions" }).click();
  const [download] = await Promise.all([
    page.waitForEvent("download"),
    page.getByRole("menuitem", { name: menu, exact: true }).click(),
  ]);
  await expect(page.getByRole("menu")).toBeHidden();
  return download;
}

async function printURL(page: Page): Promise<string> {
  await page.getByRole("button", { name: "More actions" }).click();
  await page.getByRole("menuitem", { name: "PDF", exact: true }).click();
  const frame = page.locator("iframe.print-frame").contentFrame();
  await expect.poll(() => frame.locator("body").evaluate(() => (window as unknown as { betaPrinted?: boolean }).betaPrinted)).toBe(true);
  expect(await page.evaluate(() => (window as unknown as { betaPrinted?: boolean }).betaPrinted)).toBeUndefined();
  return frame.locator("body").evaluate(() => location.href);
}

async function removeBob(page: Page) {
  await page.getByRole("button", { name: "Share", exact: true }).click();
  const panel = page.getByRole("dialog");
  await presented(panel);
  await panel.getByRole("button", { name: "Remove bob", exact: true }).click();
  await expect(panel.getByRole("button", { name: "Remove bob", exact: true })).toHaveCount(0);
  await panel.getByRole("button", { name: "Done", exact: true }).click();
}

// One cross-surface handoff, not another matrix of formatting/export/access rules.
test("two independent people and a named agent keep Doc and Design work through snapshots, restore, anchored feedback, export and connected revocation", async ({ page, browser, watch }, info) => {
  test.setTimeout(120_000);
  await signIn(page, "alice");
  const doc = await publishDoc(page, "Beta Doc", initialDoc, ["bob"]);
  const design = await publish(page, { title: "Beta Design", kind: "design", visibility: "organisation", content: manifest, files: {
    "frames/hero.html": initialHero, "frames/footer.html": "<!doctype html><p>FOOTEREXPORT</p>",
  } });
  await nameEditors(page, design.id, ["bob"]);
  const bob = await browse(browser, watch, originOf(page), "bob");
  expect(bob.context).not.toBe(page.context());
  const designAlice = watch(await page.context().newPage());
  const designBob = watch(await bob.context.newPage());
  const agent = await agentOf(page, client);
  const bobAgent = await agentOf(bob.page, client);
  for (const p of [page, designAlice]) await p.addInitScript(() => { window.print = () => { (window as unknown as { betaPrinted?: boolean }).betaPrinted = true; }; });
  try {
    await test.step("Doc: both people, named carets and targeted agent edit preserve Bob's typing location", async () => {
      await Promise.all([openEditor(page, doc.id), openEditor(bob.page, doc.id)]);
      await typeAtEnd(page, "Doc first.", " Alice.");
      await typeAtEnd(bob.page, "Doc second.", " Bob.");
      for (const p of [page, bob.page]) {
        await expect(block(p, "Doc first.")).toHaveText("Doc first. Alice.");
        await expect(block(p, "Doc second.")).toHaveText("Doc second. Bob.");
      }
      await expect(caretOf(page, "BOB")).toBeVisible();
      await expect(caretOf(bob.page, "ALICE")).toBeVisible();
      const read = await agent("read", { id: doc.id, view: "blocks" });
      const target = (read.structured.tabs as { blocks: { id: string; content?: { text: string }[] }[] }[])[0]?.blocks.find((b) => b.content?.[0]?.text === "Doc first. Alice.");
      expect(target).toBeDefined();
      const changed = await agent("doc_edit", { id: doc.id, ops: [{ op: "replace", id: target?.id, markdown: "Doc first. Alice. Agent." }] });
      expect(changed.isError, changed.text).toBe(false);
      for (const p of [page, bob.page]) {
        await expect(block(p, "Doc first.")).toHaveText("Doc first. Alice. Agent.");
        await expect(p.locator(`.ed-agent-block[data-agent="${agentName}"]`)).toBeVisible();
        await expect(p.getByRole("group", { name: "Viewing now" })).toContainText(agentName);
      }
      await bob.page.keyboard.type(" Kept.");
      await expect(block(page, "Doc second.")).toHaveText("Doc second. Bob. Kept.");
    });

    await test.step("Design: named selection overlays and targeted agent edit preserve the other source element", async () => {
      await Promise.all([openDesign(designAlice, design.id), openDesign(designBob, design.id)]);
      await appendDesign(designAlice, "#title", " Alice.");
      await appendDesign(designBob, "#copy", " Bob.");
      await clickElement(designAlice, "#title");
      await clickElement(designBob, "#copy");
      await expect(designAlice.locator('[data-peer="BOB"] .peer-tag')).toBeVisible();
      await expect(designBob.locator('[data-peer="ALICE"] .peer-tag')).toBeVisible();
      const read = await agent("read", { id: design.id, file: "frames/hero.html" });
      const source = (read.structured.file as { text: string }).text;
      expect(source).toContain("Design body. Bob.");
      expect(source).toContain(">Design title. Alice.</h1>");
      for (const p of [designAlice, designBob]) await expect(hero(p).locator("#title")).toHaveText("Design title. Alice.");
      const edited = await agent("publish", { id: design.id, title: "Beta Design", content: manifest, baseVersion: read.structured.version, sourceBases: read.structured.sourceBases, files: { "frames/hero.html": source.replace("Design title. Alice.", "Design title. Alice. Agent.") } });
      expect(edited.isError, edited.text).toBe(false);
      for (const p of [designAlice, designBob]) {
        await expect(hero(p).locator("#title")).toHaveText("Design title. Alice. Agent.");
        await expect(hero(p).locator("#copy")).toHaveText("Design body. Bob.");
        await expect(p.locator(".peerbox-agent .peer-tag").first()).toHaveText(agentName);
      }
      await expect(designBob.locator('[data-selection="copy"]')).toHaveCount(1);
    });

    await test.step("Quiet snapshots and protective UI restore keep the pre-restore merged source", async () => {
      const [docSnapshot, designSnapshot] = await Promise.all([quietSnapshot(agent, doc.id, "Agent."), quietSnapshot(agent, design.id, "Agent.", "frames/hero.html")]);
      await restoreFirst(page, false, async () => {
        await typeAtEnd(bob.page, "Doc second.", " Protected.");
        await expect.poll(async () => (await agent("read", { id: doc.id })).structured.content).toContain("Protected.");
        expect((await versionRows(agent, doc.id))[0]?.version).toBe(docSnapshot);
      });
      await restoreFirst(designAlice, true, async () => {
        // The viewer learns of the snapshot on its next poll and reloads every frame, which ends a text edit begun before it.
        await expect(designBob.locator(".vbar .facts").getByText(`v${String(designSnapshot)}`, { exact: true })).toBeVisible({ timeout: 15_000 });
        await appendDesign(designBob, "#copy", " Protected.");
        await expect.poll(async () => ((await agent("read", { id: design.id, file: "frames/hero.html" })).structured.file as { text: string }).text).toContain("Protected.");
        expect((await versionRows(agent, design.id))[0]?.version).toBe(designSnapshot);
      });
      for (const p of [page, bob.page]) await expect(block(p, "Doc second.")).toHaveText("Doc second.");
      for (const p of [designAlice, designBob]) await expect(hero(p).locator("#copy")).toHaveText("Design body.");
      for (const [id, snapshot, file] of [[doc.id, docSnapshot, undefined], [design.id, designSnapshot, "frames/hero.html"]] as const) {
        const rows = await versionRows(agent, id);
        const restored = rows[0];
        expect(restored?.restoredFrom).toBe(1);
        expect(restored?.version).toBe(snapshot + 2);
        expect(await storedText(agent, id, (restored?.version ?? 0) - 1, file)).toContain("Protected.");
        expect(await storedText(agent, id, snapshot, file)).toContain("Agent.");
      }
    });

    await test.step("Anchored feedback on both surfaces survives another person's change and is readable by MCP", async () => {
      await block(page, "The review sentence.").click({ clickCount: 3 });
      await expect(document(page)).toBeFocused();
      await expect.poll(() => page.evaluate(() => getSelection()?.toString().trim())).toBe("The review sentence.");
      await page.getByRole("toolbar", { name: "Format", exact: true }).getByRole("button", { name: "Comment", exact: true }).click();
      await sendComment(page, "Doc anchored review");
      await typeAtEnd(bob.page, "Doc first.", " Above the quote.");
      await page.locator(".ed-commented").click();
      await expect(comments(page).getByRole("article")).toContainText("Doc anchored review");
      await expect(page.locator(".ed-commented")).toHaveText("The review sentence.");
      await designAlice.getByRole("button", { name: "Close properties", exact: true }).click();
      await clickElement(designAlice, "#title");
      await designAlice.getByRole("button", { name: "Comment on selection" }).click();
      await sendComment(designAlice, "Design anchored review");
      await dragElement(designBob, "#title", 20);
      await designAlice.getByRole("button", { name: "Comment 1 on Title" }).click();
      await expect(comments(designAlice).getByRole("article")).toContainText("Design anchored review");
      const docComments = (await agent("comments", { id: doc.id })).structured.comments as { anchor: Record<string, unknown> }[];
      expect(docComments[0]?.anchor).toMatchObject({ quote: "The review sentence." });
      expect(docComments[0]?.anchor.block).toBeTruthy();
      const designComments = (await agent("comments", { id: design.id })).structured.comments as { anchor: Record<string, unknown> }[];
      expect(designComments[0]?.anchor).toMatchObject({ type: "design", frameId: "hero", pid: "title" });
    });

    await test.step("Export this work through the actual menus, inspect bytes and print output", async () => {
      const md = await download(page, "Markdown (.md)");
      expect(md.suggestedFilename()).toBe("Beta Doc.md");
      await md.saveAs(info.outputPath("doc.md"));
      expect(readFileSync(info.outputPath("doc.md"), "utf8")).toContain("The review sentence.");
      const word = await download(page, "Word (.docx)");
      expect(word.suggestedFilename()).toBe("Beta Doc.docx");
      await word.saveAs(info.outputPath("doc.docx"));
      extractZip(info.outputPath("doc.docx"), info.outputPath("word"));
      expect(readFileSync(info.outputPath("word", "word", "document.xml"), "utf8")).toContain("The review sentence.");
      const zip = await download(designAlice, "Zip (.zip)");
      await zip.saveAs(info.outputPath("design.zip"));
      extractZip(info.outputPath("design.zip"), info.outputPath("design"));
      // Offline opening is required manual acceptance (see docs/acceptance-beta.md).
      // ZIP source checks cannot establish that an offline browser renders it.
      const exported = readFileSync(info.outputPath("design", "index.html"), "utf8");
      expect(exported).toContain("Hero");
      expect(exported).toContain("Footer");
      expect(readFileSync(info.outputPath("design", "frames", "hero.html"), "utf8")).toContain("Design title.");
      expect(readFileSync(info.outputPath("design", "frames", "footer.html"), "utf8")).toContain("FOOTEREXPORT");
      for (const [p, isDesign] of [[page, false], [designAlice, true]] as const) {
        const url = await printURL(p);
        const printing = watch(await page.context().newPage());
        try {
          await printing.goto(url);
          if (isDesign) await expect(printing.frameLocator('iframe[title="Footer"]').locator("body")).toHaveText("FOOTEREXPORT");
          else await expect(printing.getByText("The review sentence.")).toBeVisible();
          await printing.emulateMedia({ media: "print" });
          const bytes = await printing.pdf({ preferCSSPageSize: true, printBackground: true });
          const text = await pdfPages(bytes);
          expect(text.join(" ")).toContain(isDesign ? "Design title." : "The review sentence.");
          if (isDesign) {
            const styles = (await printing.locator("style").allTextContents()).join("\n");
            for (const [name, width, height] of [["Hero", 800, 600], ["Footer", 390, 300]] as const) {
              expect(styles).toContain(`@page s${String(width)}x${String(height)} { size: ${String(width)}px ${String(height)}px; margin: 0; }`);
              expect(await printing.locator(`iframe[title="${name}"]`).evaluate((el) => {
                const rect = el.getBoundingClientRect();
                return [rect.width, rect.height];
              })).toEqual([width, height]);
            }
            // The existing design-export journey allows one CSS pixel when PDF points are rounded.
            const sizes = await pdfPageSizes(bytes);
            expect(sizes).toHaveLength(2);
            [[800, 600], [390, 300]].forEach(([width = 0, height = 0], index) => {
              expect(Math.abs((sizes[index]?.width ?? 0) - width)).toBeLessThanOrEqual(1);
              expect(Math.abs((sizes[index]?.height ?? 0) - height)).toBeLessThanOrEqual(1);
            });
            expect(text[0]).toContain("Design title.");
            expect(text[1]).toContain("FOOTEREXPORT");
          }
        } finally { await printing.close(); }
      }
    });

    await test.step("Remove Bob while both surfaces remain connected; his next UI and MCP writes do not persist", async () => {
      await block(bob.page, "Doc second.").click();
      await bob.page.keyboard.press("End");
      const beforeRemoval = await bobAgent("read", { id: doc.id, view: "blocks" });
      const writableBlock = (beforeRemoval.structured.tabs as { blocks: { id: string }[] }[])[0]?.blocks[0]?.id;
      expect(writableBlock).toBeTruthy();
      await removeBob(page);
      await bob.page.keyboard.type("FORBIDDEN_DOC");
      const deniedDoc = await bobAgent("doc_edit", { id: doc.id, ops: [{ op: "replace", id: writableBlock, markdown: "FORBIDDEN_DOC" }] });
      expect(deniedDoc.isError).toBe(true);
      expect(deniedDoc.text).toContain("ask its author to make you an editor");
      await expect(document(bob.page)).toHaveAttribute("contenteditable", "false");
      await expect(document(page)).not.toContainText("FORBIDDEN_DOC");
      await clickElement(designBob, "#copy", true);
      await expect(hero(designBob).locator("#copy")).toHaveAttribute("contenteditable", "true");
      await removeBob(designAlice);
      await designBob.keyboard.type("FORBIDDEN_DESIGN");
      await designBob.keyboard.press("Enter");
      const read = await agent("read", { id: design.id, file: "frames/hero.html" });
      const deniedDesign = await bobAgent("publish", { id: design.id, title: "Beta Design", content: manifest, baseVersion: read.structured.version, files: { "frames/hero.html": initialHero.replace("Design body.", "FORBIDDEN_DESIGN") } });
      expect(deniedDesign.isError).toBe(true);
      expect(deniedDesign.text).toContain("ask its author to make you an editor");
      await expect(hero(designAlice).locator("body")).not.toContainText("FORBIDDEN_DESIGN");
      const docRead = await agent("read", { id: doc.id });
      expect(docRead.structured.content).not.toContain("FORBIDDEN_DOC");
      const designRead = await agent("read", { id: design.id, file: "frames/hero.html" });
      expect((designRead.structured.file as { text: string }).text).not.toContain("FORBIDDEN_DESIGN");
      await expect(designBob.locator('[data-handle]')).toHaveCount(0);
    });
  } finally {
    await designAlice.close();
    await bob.context.close();
  }
});
