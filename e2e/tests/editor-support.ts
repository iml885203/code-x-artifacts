// The harness for every test of the Docs editor: several people in their own
// browsers, an agent calling the MCP tools, and the handful of things all of them
// ask of a page. Later editor items (lists, comments, tabs...) build on this file.
import type { Browser, BrowserContext, Locator, Page } from "@playwright/test";
import { expect, mcpTool, nameEditors, personalToken, publish, signIn, type Person } from "./support";

/** One person in a browser of their own. */
export interface Browsing {
  who: Person;
  page: Page;
  context: BrowserContext;
}

/** Signs `who` in, in a new browser context; `watch` holds the page to the console-error rule. */
export async function browse(browser: Browser, watch: (page: Page) => Page, origin: string, who: Person): Promise<Browsing> {
  const context = await browser.newContext({ baseURL: origin });
  const page = watch(await context.newPage());
  await signIn(page, who);
  return { who, page, context };
}

export const originOf = (page: Page) => page.url().split("/").slice(0, 3).join("/");

/** The document being edited (or watched). */
export const document = (page: Page): Locator => page.getByRole("textbox", { name: "Document" });

/** A block of the document, found by the text it holds. */
export const block = (page: Page, text: string): Locator => document(page).locator("p, h1, h2, h3, h4, h5, h6").filter({ hasText: text });

/** Waits for a block (found by `key`) to say exactly `text`. */
export async function expectBlock(page: Page, key: string, text: string): Promise<void> {
  await expect(block(page, key)).toHaveText(text);
}

/** Another person's caret, found by the name on its tag. */
export const caretOf = (page: Page, name: string): Locator => page.locator(`.ed-caret[data-name="${name}"]`);

/** Opens a doc, which opens straight into the live editor (read-only for a reader): waits until it is connected. */
export async function openEditor(page: Page, id: string): Promise<void> {
  await page.goto(`/a/${id}`);
  await expect(page.locator(".ed[data-connection=live]")).toBeVisible();
}

/** Opens the insert row's menu of the given name (Tabs, More) and returns its panel. */
export async function openMenu(page: Page, name: "Tabs" | "More"): Promise<Locator> {
  await page.getByRole("toolbar", { name: "Editing tools" }).getByRole("button", { name, exact: true }).click();
  return page.getByRole("group", { name, exact: true });
}

/** Puts the caret at the end of a block and types: what a person does to add to a paragraph. */
export async function typeAtEnd(page: Page, blockText: string, text: string): Promise<void> {
  await block(page, blockText).click();
  await page.keyboard.press("End");
  await page.keyboard.type(text);
}

/** A doc made from Markdown by its author, who names the editors. */
export async function publishDoc(page: Page, title: string, markdown: string, editors: string[] = []): Promise<{ id: string; version: number }> {
  const made = await publish(page, { title, kind: "doc", content: markdown, visibility: "organisation" });
  if (editors.length > 0) await nameEditors(page, made.id, editors);
  return made;
}

/** An agent: the same person's token and the MCP tools, as an agent client calls them. */
/** `client` is the name the agent client gives itself in the MCP clientInfo. */
export async function agentOf(page: Page, client?: string): Promise<(name: string, args: Record<string, unknown>) => ReturnType<typeof mcpTool>> {
  const token = await personalToken(page);
  const origin = originOf(page);
  return (name, args) => mcpTool(origin, token, name, args, client);
}

/** The text of the document as the editor shows it, block by block. */
export async function blocksOf(page: Page): Promise<string[]> {
  return document(page).locator("p, h1, h2, h3, h4, h5, h6").allTextContents();
}

/** A tool of the editor's insert row, of the bubble over the selection, or of the More menu (opened when the tool is in neither). */
export async function tool(page: Page, name: string): Promise<Locator> {
  const row = page.getByRole("toolbar", { name: "Editing tools" });
  const found = row.getByRole("button", { name, exact: true });
  if (await found.count() > 0) return found;
  const floating = page.getByRole("toolbar", { name: "Format" }).getByRole("button", { name, exact: true });
  if (await floating.count() > 0) return floating;
  const panel = await openMenu(page, "More");
  return panel.getByRole("button", { name, exact: true });
}
