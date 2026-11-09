import type { Page } from "@playwright/test";
import { agentOf, browse, originOf } from "./editor-support";
import { expect, nameEditors, png, publish, signIn, test } from "./support";

interface El { id: string; type: string; fileId?: string; x: number; y: number; width: number; height: number; text?: string; containerId?: string | null; isDeleted?: boolean; startBinding?: { elementId: string } | null; endBinding?: { elementId: string } | null }

/** Opens a board and waits until its room's state has arrived. */
async function openBoard(page: Page, id: string): Promise<void> {
  await page.goto(`/a/${id}`);
  await expect(page.locator(".board[data-synced=true][data-connection=live]")).toBeVisible();
}

/** Where a point of the board is on the page, at the zoom and scroll a board opens with. */
async function at(page: Page, x: number, y: number): Promise<{ x: number; y: number }> {
  const canvas = await page.locator(".board-canvas").boundingBox(); // page-level
  if (!canvas) throw new Error("no board on the page");
  return { x: canvas.x + x, y: canvas.y + y };
}

async function drag(page: Page, from: [number, number], to: [number, number]): Promise<void> {
  const a = await at(page, ...from);
  const b = await at(page, ...to);
  await page.mouse.move(a.x, a.y);
  await page.mouse.down();
  await page.mouse.move(b.x, b.y, { steps: 8 });
  await page.mouse.up();
}

/** What this browser's editor shows: every element, as its own copy gives them. */
async function sceneOf(page: Page): Promise<El[]> {
  const empty = await at(page, 1100, 600);
  await page.mouse.click(empty.x, empty.y);
  await page.keyboard.press("ControlOrMeta+a");
  await page.keyboard.press("ControlOrMeta+c");
  const copied = JSON.parse(await page.evaluate(() => navigator.clipboard.readText())) as { elements: El[] };
  await page.keyboard.press("Escape");
  return copied.elements.filter((el) => !el.isDeleted);
}

/** What the room holds, as an agent reads it. */
async function roomOf(page: Page, id: string): Promise<El[]> {
  const agent = await agentOf(page);
  return (JSON.parse((await agent("read", { id })).structured["content"] as string) as { elements: El[] }).elements;
}

const kinds = (els: El[]) => els.map((el) => el.type).sort();
const centred = (box: El, label: El) => Math.abs(label.x + label.width / 2 - (box.x + box.width / 2)) < 1 && Math.abs(label.y + label.height / 2 - (box.y + box.height / 2)) < 1;

test("alice and bob draw one board at once: a box with R, a label typed in Zhuyin, an arrow between two boxes; each sees the other's cursor and drawing, a reload keeps it, an agent's change shows live under its name, a pasted picture is a file of the board, a reader cannot draw, and nothing loads from another site", async ({ page, browser, watch }) => {
  await signIn(page, "alice");
  const made = await publish(page, { title: "Flow", kind: "board", content: '{"version":1,"elements":[]}', visibility: "organisation" });
  await nameEditors(page, made.id, ["bob"]);
  const bob = await browse(browser, watch, originOf(page), "bob");
  const elsewhere: string[] = [];
  for (const p of [page, bob.page]) {
    await p.context().grantPermissions(["clipboard-read", "clipboard-write"]);
    p.on("request", (r) => {
      if (!r.url().startsWith(originOf(page)) && !r.url().startsWith("data:")) elsewhere.push(r.url());
    });
    await openBoard(p, made.id);
  }

  // A box with R, then Enter and a label composed in Zhuyin.
  const empty = await at(page, 1100, 600);
  await page.mouse.click(empty.x, empty.y);
  await page.keyboard.press("r");
  await drag(page, [320, 200], [480, 280]);
  await page.keyboard.press("Enter");
  await expect(page.locator("textarea.excalidraw-wysiwyg")).toBeFocused();
  const ime = await page.context().newCDPSession(page);
  await ime.send("Input.imeSetComposition", { text: "ㄋㄧˇ", selectionStart: 3, selectionEnd: 3 });
  await ime.send("Input.insertText", { text: "你" });
  await ime.send("Input.imeSetComposition", { text: "ㄏㄠˇ", selectionStart: 3, selectionEnd: 3 });
  await ime.send("Input.insertText", { text: "好" });
  await page.keyboard.press("Escape");
  // A second box and an arrow from the first to it.
  await page.keyboard.press("r");
  await drag(page, [720, 200], [880, 280]);
  await page.keyboard.press("a");
  await drag(page, [477, 240], [723, 240]); // from the first box's edge to the second's, where the editor binds an arrow
  await page.keyboard.press("Escape");

  // The room holds the two boxes, the label in the first, and the arrow bound to both.
  await expect.poll(async () => {
    const room = await roomOf(page, made.id);
    const [first, second] = room.filter((el) => el.type === "rectangle").sort((a, b) => a.x - b.x);
    const label = room.find((el) => el.type === "text");
    const arrow = room.find((el) => el.type === "arrow");
    return {
      kinds: kinds(room),
      label: label?.text,
      inFirst: label !== undefined && label.containerId === first?.id,
      bound: arrow?.startBinding?.elementId === first?.id && arrow?.endBinding?.elementId === second?.id,
    };
  }).toEqual({ kinds: ["arrow", "rectangle", "rectangle", "text"], label: "你好", inFirst: true, bound: true });

  // Bob's editor shows the same board, and Alice's cursor with her name.
  await expect.poll(async () => (await sceneOf(bob.page)).map((el) => `${el.type}:${el.text ?? ""}`).sort()).toEqual(["arrow:", "rectangle:", "rectangle:", "text:你好"]);
  const point = await at(page, 500, 450);
  await page.mouse.move(point.x, point.y, { steps: 4 });
  await expect(bob.page.locator(".UserList__collaborator")).toHaveCount(1);

  await bob.page.reload();
  await expect(bob.page.locator(".board[data-synced=true]")).toBeVisible();
  await expect.poll(async () => kinds(await sceneOf(bob.page))).toEqual(["arrow", "rectangle", "rectangle", "text"]);
  expect(elsewhere).toEqual([]);

  // An agent changes the first box's words while both have the board open: they see it at once, and the agent by name.
  const agent = await agentOf(page, "claude-code");
  const read = await agent("read", { id: made.id });
  const box = (await roomOf(page, made.id)).filter((el) => el.type === "rectangle").sort((a, b) => a.x - b.x)[0];
  const hashes = read.structured["hashes"] as Record<string, string>;
  const edited = await agent("board_edit", { id: made.id, ops: [{ op: "change", id: box?.id, ifHash: hashes[box?.id ?? ""], fields: { label: "你好嗎" } }] });
  expect(edited.isError, edited.text).toBe(false);
  expect(edited.structured["saved"]).toBe(false);
  for (const seen of [page, bob.page]) {
    await expect.poll(async () => (await sceneOf(seen)).find((el) => el.type === "text")?.text).toBe("你好嗎");
    await expect(seen.getByRole("group", { name: "Viewing now" })).toContainText("alice 的 Claude Code");
  }

  // Alice pastes a screenshot: it becomes a file of the board, and Bob's editor shows it from there, live and after a reload.
  const pictureLoads: string[] = [];
  bob.page.on("response", (r) => {
    if (r.url().includes(`/files/${made.id}/`)) pictureLoads.push(`${String(r.status())} ${r.url()}`);
  });
  await page.evaluate((b64) => {
    const data = new DataTransfer();
    data.items.add(new File([Uint8Array.from(atob(b64), (c) => c.charCodeAt(0))], "image.png", { type: "image/png" }));
    document.dispatchEvent(new ClipboardEvent("paste", { clipboardData: data, bubbles: true, cancelable: true }));
  }, png.toString("base64"));
  await expect.poll(async () => (await roomOf(page, made.id)).find((el) => el.type === "image")?.fileId ?? "").toMatch(/^image-[0-9a-f]{8}\.png$/);
  const picture = (await roomOf(page, made.id)).find((el) => el.type === "image")?.fileId ?? "";
  await expect.poll(async () => (await sceneOf(bob.page)).find((el) => el.type === "image")?.fileId).toBe(picture);
  await expect.poll(() => pictureLoads.some((l) => l.startsWith("200 ") && l.endsWith(`/${picture}`))).toBe(true);
  await bob.page.reload();
  await expect(bob.page.locator(".board[data-synced=true]")).toBeVisible();
  await expect.poll(async () => (await sceneOf(bob.page)).find((el) => el.type === "image")?.fileId).toBe(picture);
  // Once its file is deleted the board still opens, with the image as a placeholder and nothing asked of a missing file.
  const deleted = await page.evaluate(async ([id, name]) => (await fetch(`/api/artifacts/${id ?? ""}/assets/${name ?? ""}`, { method: "DELETE", headers: { "X-Pictor": "1" } })).status, [made.id, picture]);
  expect(deleted).toBe(204);
  pictureLoads.length = 0;
  await bob.page.reload();
  await expect(bob.page.locator(".board[data-synced=true]")).toBeVisible();
  await expect.poll(async () => (await sceneOf(bob.page)).some((el) => el.type === "image")).toBe(true);
  expect(pictureLoads).toEqual([]);

  // Carol may only read: no tools, and what she does changes nothing.
  const carol = await browse(browser, watch, originOf(page), "carol");
  await openBoard(carol.page, made.id);
  await expect(carol.page.locator(".App-toolbar")).toHaveCount(0);
  await carol.page.keyboard.press("r");
  await drag(carol.page, [320, 450], [420, 520]);
  await carol.page.keyboard.press("Delete");
  expect(kinds(await roomOf(page, made.id))).toEqual(["arrow", "image", "rectangle", "rectangle", "text"]);
  await carol.context.close();
  await bob.context.close();
});

test("alice moves a box while bob retypes its label: both changes stay with the label centred in both browsers, and alice's Undo puts the box back and leaves bob's words", async ({ page, browser, watch }) => {
  await signIn(page, "alice");
  const made = await publish(page, {
    title: "Labels",
    kind: "board",
    visibility: "public",
    content: JSON.stringify({ version: 1, elements: [
      { id: "box", type: "rectangle", x: 200, y: 200, width: 200, height: 100, index: "a0", boundElements: [{ id: "label", type: "text" }] },
      { id: "label", type: "text", width: 50, height: 25, fontSize: 20, lineHeight: 1.25, text: "Start", originalText: "Start", containerId: "box", textAlign: "center", verticalAlign: "middle", index: "a1" },
    ] }),
  });
  await nameEditors(page, made.id, ["bob"]);
  const bob = await browse(browser, watch, originOf(page), "bob");
  for (const p of [page, bob.page]) {
    await p.context().grantPermissions(["clipboard-read", "clipboard-write"]);
    await openBoard(p, made.id);
  }

  // Bob is cut off while both edit, so neither sees the other's change first. Cut off
  // means from the fonts too, so the label's font is loaded before.
  await bob.page.evaluate(async () => {
    await document.fonts.ready;
  });
  await bob.context.setOffline(true);
  await expect(bob.page.locator(".board[data-connection=offline]")).toBeVisible();
  const centre = await at(bob.page, 300, 250);
  await bob.page.mouse.dblclick(centre.x, centre.y);
  await bob.page.keyboard.press("ControlOrMeta+a");
  await bob.page.keyboard.type("Begin");
  await bob.page.keyboard.press("Escape");
  await drag(page, [230, 220], [530, 420]);
  await bob.context.setOffline(false);

  const boxAndLabel = async (p: Page) => {
    const els = await sceneOf(p);
    const box = els.find((el) => el.id === "box");
    const label = els.find((el) => el.id === "label");
    return box && label ? { x: box.x, text: label.text, centred: centred(box, label) } : null;
  };
  await expect.poll(() => boxAndLabel(bob.page)).toEqual({ x: 500, text: "Begin", centred: true });
  const room = await roomOf(page, made.id);
  expect(room.find((el) => el.id === "label")?.text).toBe("Begin");

  // Alice's Undo, straight after her move, takes back her move and nothing of Bob's.
  await page.keyboard.press("ControlOrMeta+z");
  for (const p of [page, bob.page]) await expect.poll(() => boxAndLabel(p)).toEqual({ x: 200, text: "Begin", centred: true });
  await bob.context.close();

  // The board moves as a design does: 1, + and - step the zoom, 0 fits.
  const zoom = page.locator(".reset-zoom-button");
  await page.keyboard.press("1");
  await expect(zoom).toHaveText("100%");
  await page.keyboard.press("+");
  await expect(zoom).toHaveText("125%");
  await page.keyboard.press("-");
  await page.keyboard.press("-");
  await expect(zoom).toHaveText("75%");
  await page.keyboard.press("0");
  await expect(zoom).not.toHaveText("75%");
  await page.keyboard.press("1");
  await expect(zoom).toHaveText("100%");
  // Space-drag pans, H then a drag pans, and V gives the selection back: a drag on the empty board no longer moves it.
  const xOfBoxDrawnAt = async (x: number): Promise<number> => {
    const had = new Set((await sceneOf(page)).map((el) => el.id));
    await page.keyboard.press("r");
    await drag(page, [x, 480], [x + 60, 540]);
    await page.keyboard.press("Escape");
    const drawn = (await sceneOf(page)).find((el) => !had.has(el.id));
    return Math.round(drawn?.x ?? Number.NaN);
  };
  const before = await xOfBoxDrawnAt(700);
  await page.keyboard.down(" ");
  await drag(page, [900, 300], [1000, 300]);
  await page.keyboard.up(" ");
  expect(await xOfBoxDrawnAt(700)).toBe(before - 100);
  await page.keyboard.press("h");
  await drag(page, [900, 300], [950, 300]);
  expect(await xOfBoxDrawnAt(700)).toBe(before - 150);
  await page.keyboard.press("v");
  await drag(page, [900, 120], [1000, 140]);
  await page.keyboard.press("Escape");
  expect(await xOfBoxDrawnAt(700)).toBe(before - 150);

  // Export from the open board is the editor's own drawing, named after the title.
  const exported = async (format: string): Promise<{ name: string; bytes: Buffer }> => {
    await page.getByRole("button", { name: "More actions" }).click();
    const downloading = page.waitForEvent("download");
    await page.getByRole("menuitem", { name: `${format.toUpperCase()} (.${format})` }).click();
    const download = await downloading;
    const chunks: Buffer[] = [];
    for await (const chunk of await download.createReadStream()) chunks.push(chunk as Buffer);
    return { name: download.suggestedFilename(), bytes: Buffer.concat(chunks) };
  };
  const svg = await exported("svg");
  expect(svg.name).toBe("Labels.svg");
  expect(svg.bytes.toString("utf8")).toMatch(/<svg[\s\S]*Begin/);
  const png = await exported("png");
  expect(png.name).toBe("Labels.png");
  expect(png.bytes.subarray(1, 4).toString("latin1")).toBe("PNG");

  // The gallery card draws the board, and someone not signed in sees it drawn, with no tools.
  await page.goto("/");
  const card = page.locator(`[data-artifact-id="${made.id}"]`);
  await card.scrollIntoViewIfNeeded();
  // The card shows the newest version, which may be from before the room's last edits: its box and label are there either way.
  await expect(card.frameLocator("[data-thumb] iframe").locator("svg rect + text tspan")).toBeAttached();
  const outsider = await browser.newContext({ baseURL: originOf(page) });
  const stranger = watch(await outsider.newPage());
  await stranger.goto(`/a/${made.id}`);
  // Someone not signed in sees the newest saved version: the room's edits become one once it has been quiet.
  await expect(async () => {
    await stranger.reload();
    await expect(stranger.frameLocator("[data-artifact-frame]").locator("svg tspan", { hasText: "Begin" })).toBeVisible({ timeout: 1000 });
  }).toPass({ timeout: 15_000 });
  await expect(stranger.locator(".App-toolbar")).toHaveCount(0);
  await outsider.close();
});
