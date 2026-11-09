import type { Page } from "@playwright/test";
import { agentOf, browse, originOf } from "./editor-support";
import { expect, mcpTool, nameEditors, onScreen, personalToken, publish, signIn, test } from "./support";

// Typing into text in place, through the real frame, the real socket and the
// browser's own input handling: the room has the text, the saved file has it
// with its markup, and Undo works without clicking anything first.

const manifest = JSON.stringify({ version: 1, pages: [{ id: "home", name: "Home", rows: [[{ id: "hero", name: "Hero", file: "frames/hero.html", w: 800, h: 600 }]] }] });
const hero = `<!doctype html><html><head><meta charset="utf-8"></head><body style="margin:0">
<h1 id="title" data-name="Title" style="position:absolute;left:40px;top:40px;margin:0;font:700 40px sans-serif">Headline</h1>
<p id="copy" data-name="Copy" style="position:absolute;left:40px;top:160px;margin:0;width:400px;font:20px sans-serif">Hello <b>bold</b><br>line2</p>
<div id="box" data-name="Box" style="position:absolute;left:500px;top:40px;width:100px;height:60px;background:#36c"></div>
</body></html>`;

const heroFrame = (page: Page) => page.locator('.dframe iframe[title^="Hero,"]');
const onHero = (page: Page) => page.frameLocator('.dframe iframe[title^="Hero,"]');

async function open(page: Page, id: string) {
  await page.goto(`/a/${id}`);
  await expect(onHero(page).locator("#title")).toBeVisible();
  await expect(page.locator(".canvas-presence .ed-conn-live")).toBeAttached();
  await expect(page.locator(".canvas-presence .ed-conn-live")).toBeHidden();
}

async function doubleClick(page: Page, selector: string) {
  const at = await onScreen(heroFrame(page), onHero(page).locator(selector));
  await page.mouse.dblclick(at.x + at.width / 2, at.y + at.height / 2);
  await expect(onHero(page).locator(selector)).toHaveAttribute("contenteditable", "true");
}

async function savedFrame(page: Page, id: string, until: (text: string) => boolean): Promise<string> {
  const token = await personalToken(page);
  let text = "";
  await expect
    .poll(async () => {
      const read = await mcpTool(originOf(page), token, "read", { id, file: "frames/hero.html" });
      text = (read.structured["file"] as { text?: string } | undefined)?.text ?? "";
      return until(text);
    })
    .toBe(true);
  return text;
}

test("double-click a headline, give up with Esc, then type Zhuyin and press Enter; Undo works right after without a click, and bold text keeps its markup", async ({ page }) => {
  await signIn(page, "alice");
  const a = await publish(page, { title: "Text", kind: "design", content: manifest, files: { "frames/hero.html": hero } });
  await open(page, a.id);
  const agent = await agentOf(page, "claude-code");
  const title = onHero(page).locator("#title");
  const style = await title.getAttribute("style");

  await doubleClick(page, "#title");
  await page.keyboard.type("Nope");
  await page.keyboard.press("Escape");
  await expect(title).toHaveText("Headline");
  await expect(title).not.toHaveAttribute("contenteditable");

  // A two-part composition through the browser's own protocol, as an input method sends it.
  await doubleClick(page, "#title");
  const ime = await page.context().newCDPSession(page);
  await ime.send("Input.imeSetComposition", { text: "ㄋ", selectionStart: 1, selectionEnd: 1 });
  await ime.send("Input.imeSetComposition", { text: "ㄋㄧˇ", selectionStart: 2, selectionEnd: 2 });
  const current = await agent("read", { id: a.id, file: "frames/hero.html" });
  const source = (current.structured.file as { text: string }).text;
  const changed = await agent("publish", { id: a.id, title: "Text", content: manifest, baseVersion: current.structured.version, sourceBases: current.structured.sourceBases, files: { "frames/hero.html": source.replace(">Headline</h1>", ">AGENT Headline</h1>") } });
  expect(changed.isError, changed.text).toBe(false);
  await savedFrame(page, a.id, (text) => text.includes("AGENT Headline"));
  await expect(title).toHaveAttribute("contenteditable", "true");
  await expect(title).toHaveText("ㄋㄧˇ");
  await ime.send("Input.insertText", { text: "你" });
  await ime.send("Input.imeSetComposition", { text: "ㄏㄠˇ", selectionStart: 2, selectionEnd: 2 });
  await ime.send("Input.insertText", { text: "好" });
  await page.keyboard.press("Enter");
  await expect(title).toHaveText("AGENT 你好");
  // Chrome rewrites the style of an element it edits; the frame has it back as it was.
  expect(await title.getAttribute("style")).toBe(style);
  await expect(title).not.toHaveAttribute("contenteditable");
  const saved = await savedFrame(page, a.id, (text) => text.includes("你好"));
  expect(saved).not.toContain("Headline");

  // Nothing was clicked since: the keyboard is the page's again.
  await page.keyboard.press("ControlOrMeta+z");
  await expect(title).toHaveText("AGENT Headline");
  await savedFrame(page, a.id, (text) => text.includes("Headline"));

  // Native browser caret/deletion cannot be established by jsdom or a Y.Text round trip.
  await doubleClick(page, "#title");
  await ime.send("Input.insertText", { text: "A👍🏽👩‍💻𠮷𪚥Z" });
  await page.keyboard.press("End");
  for (const grapheme of ["Z", "𪚥", "𠮷", "👩‍💻", "👍🏽", "A"]) {
    await page.keyboard.press("Shift+ArrowLeft");
    await expect.poll(() => onHero(page).locator("body").evaluate(() => getSelection()?.toString())).toBe(grapheme);
    await page.keyboard.press("ArrowLeft");
  }
  await page.keyboard.press("End");
  await page.keyboard.press("ArrowLeft");
  await page.keyboard.press("ArrowLeft");
  await ime.send("Input.insertText", { text: "|" });
  await expect(title).toHaveText("A👍🏽👩‍💻𠮷|𪚥Z");
  await page.keyboard.press("Backspace");
  for (const remaining of ["A👍🏽👩‍💻𪚥Z", "A👍🏽𪚥Z", "A𪚥Z"]) {
    await page.keyboard.press("Backspace");
    await expect(title).toHaveText(remaining);
  }
  await page.keyboard.press("Enter");
  await savedFrame(page, a.id, (text) => text.includes("A𪚥Z"));
  await page.reload();
  await expect(title).toHaveText("A𪚥Z");
  await ime.detach();

  // The bold word and the break stay when the paragraph is typed into, Cmd+B makes the last line bold and Shift+Enter breaks the line.
  await doubleClick(page, "#copy");
  await page.keyboard.press("End");
  await page.keyboard.type("!");
  for (let i = 0; i < "line2!".length; i++) await page.keyboard.press("Shift+ArrowLeft");
  await page.keyboard.press("ControlOrMeta+b");
  await page.keyboard.press("End");
  await page.keyboard.press("Shift+Enter");
  await page.keyboard.type("z");
  await page.keyboard.press("Enter");
  const copy = await savedFrame(page, a.id, (text) => text.includes("line2!"));
  expect(copy).toMatch(/Hello <b data-pid="[^"]+">bold<\/b><br data-pid="[^"]+"\/?><b data-pid="[^"]+">line2!(<\/b>)?<br data-pid="[^"]+"\/?>(<b data-pid="[^"]+">)?z(<\/b>)?<\/p>/);
});

test("bob types into a paragraph while alice moves it in the outline; the text and the order are in both browsers and in the saved file", async ({ page, browser, watch }) => {
  await signIn(page, "alice");
  const a = await publish(page, { title: "Text and order", kind: "design", content: manifest, files: { "frames/hero.html": hero } });
  await nameEditors(page, a.id, ["bob"]);
  const bob = await browse(browser, watch, originOf(page), "bob");
  await open(page, a.id);
  await open(bob.page, a.id);
  const order = (seen: Page) => onHero(seen).locator("body > *").evaluateAll((els) => els.map((e) => e.id));
  expect(await order(page)).toEqual(["title", "copy", "box"]);

  // Bob is typing at the end of the paragraph, not yet finished.
  await doubleClick(bob.page, "#copy");
  await bob.page.keyboard.press("End");
  await bob.page.keyboard.type(" and more");

  // Alice puts the paragraph first, through the outline.
  await page.locator(".canvas-bar-right").getByRole("button", { name: "Properties", exact: true }).click();
  await expect(page.getByRole("navigation", { name: "Layers", exact: true })).toBeVisible();
  const row = (seen: Page, name: string) => seen.locator(".orow", { has: seen.locator(".orow-label", { hasText: new RegExp(`^${name}$`) }) });
  await expect(row(page, "Copy")).toBeVisible();
  await row(page, "Copy").dragTo(row(page, "Title"), { targetPosition: { x: 40, y: 3 } });
  await bob.page.keyboard.press("Enter");
  for (const seen of [page, bob.page]) {
    await expect.poll(() => order(seen)).toEqual(["copy", "title", "box"]);
    await expect(onHero(seen).locator("#copy")).toContainText("line2 and more");
  }
  const saved = await savedFrame(page, a.id, (text) => text.includes("line2 and more") && text.indexOf('id="copy"') < text.indexOf('id="title"'));
  expect(saved).toMatch(/<b data-pid="[^"]+">bold<\/b><br/);
  await bob.context.close();
});
