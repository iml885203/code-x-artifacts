import type { Page } from "@playwright/test";
import { expect, presented, publish, signIn, test, visibilityOf } from "./support";

const goneSlack = 20_000;

const hello = "<!doctype html><html><head><title>Hello</title></head><body><h1>Hello from the page</h1></body></html>";
const manifest = JSON.stringify({ version: 1, pages: [{ id: "home", name: "Home", rows: [[{ id: "hero", name: "Hero", file: "frames/hero.html", w: 800, h: 600 }]] }] });

test("sign in, publish and view: a signed-in person opens an artifact from the overview", async ({ page, expectConsoleErrors }) => {
  // The refused sign-in is answered 401, which the browser logs.
  expectConsoleErrors(/status of 401/);
  await page.goto("/login");
  await page.getByLabel("Username").fill("alice");
  await page.getByLabel("Password").fill("wrong");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("alert")).toHaveText("Wrong username or password");

  await signIn(page, "alice");
  await publish(page, { title: "Overview piece", content: hello });
  await page.reload();
  await page.getByRole("link", { name: /Overview piece/ }).click();
  await expect(page).toHaveURL(/\/a\//);
  await expect(page.frameLocator("iframe[data-artifact-frame]").locator("h1")).toHaveText("Hello from the page");
});

test("a web page, a doc and a design open with the same bar, and the author shares to the organisation from the dialog", async ({ page, browser, watch }) => {
  await signIn(page, "alice");
  const web = await publish(page, { title: "Shared", content: hello });
  const doc = await publish(page, { title: "Notes", kind: "doc", content: "# Notes\n\nA line.\n" });
  const design = await publish(page, { title: "Board", kind: "design", content: manifest, files: { "frames/hero.html": "<!doctype html><h1>Hero</h1>" } });
  expect(await visibilityOf(page, web.id)).toBe("private");

  for (const [a, kind, title] of [[web, "Web page", "Shared"], [doc, "Doc", "Notes"], [design, "Design", "Board"]] as const) {
    await page.goto(`/a/${a.id}`);
    const bar = page.getByRole("banner");
    await expect(bar.getByRole("img", { name: kind, exact: true })).toBeVisible();
    await expect(bar.getByRole("button", { name: title })).toBeVisible();
    await expect(bar.getByRole("img", { name: "Only me" })).toBeVisible();
    await expect(bar.getByRole("button", { name: "Share", exact: true })).toBeVisible();
    await expect(bar.getByRole("button", { name: "More actions" })).toBeVisible();
  }

  await page.goto(`/a/${web.id}`);
  await page.getByRole("button", { name: "Share", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await presented(dialog);
  await expect(dialog.getByRole("radio", { name: /^Only me/ })).toBeChecked();
  await dialog.getByRole("radio", { name: /^Organisation/ }).click();
  await expect(dialog.getByRole("radio", { name: /^Organisation/ })).toBeChecked();
  await expect.poll(() => visibilityOf(page, web.id)).toBe("organisation");
  const box = await dialog.boundingBox(); // page-level
  if (!box) throw new Error("Share has no visible bounds");
  await page.mouse.click(box.x + 3, box.y + 3);
  await expect(dialog).toBeVisible();
  const link = dialog.getByRole("textbox", { name: "Link" });
  await link.hover();
  await page.mouse.down();
  await page.mouse.move(box.x - 12, box.y + 20);
  await page.mouse.up();
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button", { name: "Done" }).focus();
  await page.keyboard.press("Shift+Tab");
  await expect(dialog.getByRole("checkbox")).toBeFocused();
  await page.getByRole("button", { name: "Share", exact: true }).focus();
  await expect(dialog.getByRole("checkbox")).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(dialog).toBeHidden();
  await expect(page.getByRole("button", { name: "Share", exact: true })).toBeFocused();
  await expect.poll(() => visibilityOf(page, web.id)).toBe("organisation");

  await page.getByRole("button", { name: "Share", exact: true }).click();
  await presented(dialog);
  await dialog.getByRole("radio", { name: /^Organisation/ }).click();
  await expect.poll(() => visibilityOf(page, web.id)).toBe("organisation");
  await page.mouse.click(box.x - 12, box.y + 20);
  await expect(dialog).toBeHidden();
  await expect(page.getByRole("button", { name: "Share", exact: true })).toBeFocused();
  await expect.poll(() => visibilityOf(page, web.id)).toBe("organisation");

  await page.getByRole("button", { name: "Share", exact: true }).click();
  await presented(dialog);
  await dialog.getByRole("radio", { name: /^Organisation/ }).click();
  await expect.poll(() => visibilityOf(page, web.id)).toBe("organisation");
  await dialog.getByRole("textbox", { name: "Account name" }).fill("bob");
  await dialog.getByRole("button", { name: "Add", exact: true }).click();
  await expect(dialog.getByText("bob", { exact: true }).locator("..").locator("..")).toContainText("Can edit");
  await dialog.getByRole("button", { name: "Done" }).click();
  await expect(dialog).toBeHidden();
  await expect(page.getByRole("banner").getByRole("img", { name: "Organisation" })).toBeVisible();

  const touchContext = await browser.newContext({ baseURL: new URL(page.url()).origin, hasTouch: true });
  const reader = watch(await touchContext.newPage());
  await signIn(reader, "bob");
  await reader.goto(`/a/${web.id}`);
  await reader.getByRole("button", { name: "Shared", exact: true }).click();
  await reader.getByRole("menuitem", { name: "Rename", exact: true }).click();
  await reader.getByRole("textbox", { name: "Title", exact: true }).fill("Shared with Bob");
  await reader.getByRole("textbox", { name: "Title", exact: true }).press("Enter");
  await expect(reader.getByRole("button", { name: "Shared with Bob", exact: true })).toBeVisible();
  await reader.getByRole("button", { name: "Share", exact: true }).tap();
  const readOnlyShare = reader.getByRole("dialog");
  await presented(readOnlyShare);
  const readerBox = await readOnlyShare.boundingBox(); // page-level
  if (!readerBox) throw new Error("Reader Share has no visible bounds");
  await reader.touchscreen.tap(readerBox.x - 12, readerBox.y + 20);
  await expect(readOnlyShare).toBeHidden();
  await expect.poll(() => visibilityOf(page, web.id)).toBe("organisation");
  await touchContext.close();

  await page.reload();
  await expect(page.getByRole("banner").getByRole("img", { name: "Organisation" })).toBeVisible();

  await page.getByRole("button", { name: "Share", exact: true }).click();
  await presented(page.getByRole("dialog"));
  await page.getByRole("dialog").getByRole("radio", { name: /^Only me/ }).click();
  await expect.poll(() => visibilityOf(page, web.id)).toBe("private");
});

test("the menu offers the artifact's actions, and someone who is not the author gets no author controls", async ({ page, browser, watch }) => {
  await signIn(page, "alice");
  const a = await publish(page, { title: "Menu", content: hello, visibility: "organisation" });
  await page.goto(`/a/${a.id}`);
  await page.getByRole("button", { name: "More actions" }).click();
  for (const name of ["Versions", "Open in new tab", "Delete…"]) {
    await expect(page.getByRole("menuitem", { name })).toBeVisible();
  }

  const bobContext = await browser.newContext({ baseURL: page.url().split("/a/")[0] ?? "" });
  const bob = watch(await bobContext.newPage());
  await signIn(bob, "bob");
  await bob.goto(`/a/${a.id}`);
  await expect(bob.frameLocator("iframe[data-artifact-frame]").locator("h1")).toHaveText("Hello from the page");
  await bob.getByRole("button", { name: "Share", exact: true }).click();
  await presented(bob.getByRole("dialog"));
  await expect(bob.getByRole("radiogroup", { name: "Who can open this" })).toHaveCount(0);
  await bob.getByRole("button", { name: "Done" }).click();
  await bob.getByRole("button", { name: "More actions" }).click();
  await expect(bob.getByRole("menuitem", { name: "Versions" })).toHaveCount(0);
  await expect(bob.getByRole("menuitem", { name: "Delete…" })).toHaveCount(0);
  await bobContext.close();
});

/** The nickname a signed-out page shows for itself, as its own avatar in the bar names it. */
async function ownGuestName(visitor: Page): Promise<string> {
  const own = visitor.getByRole("banner").getByRole("button", { name: / \(Guest, You\)$/ });
  await expect(own).toHaveCount(1);
  return ((await own.getAttribute("aria-label")) ?? (await own.locator(".visually-hidden").textContent()) ?? "").replace(/ \(Guest, You\)$/, "");
}

/** Who alice's own page last heard is here, from the same list her bar shows. */
async function seenByAlice(page: Page, id: string): Promise<{ name: string; guest: boolean }[]> {
  return page.evaluate(async (path) => (await (await fetch(path, { headers: { "X-Pictor": "1" } })).json()) as { name: string; guest: boolean }[], `/api/artifacts/${id}/viewers`);
}

// The people and the signed-out visitors of a public link, as the author sees them at the top right. A visitor is a
// guest under a nickname its browser keeps: across a reload and a second tab it is one entry, another browser profile
// is another guest (with site storage blocked and on a phone too), a guest who closes the page leaves after the
// server's expiry (three seconds in the smoke server), and signing in turns the guest into the account.
test("a public link opens without signing in, its visitors are guests to the author, and closing it again hides the page", async ({ page, browser, watch, expectConsoleErrors }) => {
  test.setTimeout(90_000);
  await signIn(page, "alice");
  const a = await publish(page, { title: "Public one", content: hello });
  const base = page.url().split("/").slice(0, 3).join("/");

  const anon = await browser.newContext({ baseURL: base });
  const visitor = watch(await anon.newPage());
  await visitor.goto(`/a/${a.id}`);
  await expect(visitor).toHaveURL(/\/login/);

  await page.goto(`/a/${a.id}`);
  await page.getByRole("button", { name: "Share", exact: true }).click();
  await presented(page.getByRole("dialog"));
  await page.getByRole("radiogroup", { name: "Who can open this" }).getByRole("radio", { name: /^Public link/ }).click();
  await expect.poll(() => visibilityOf(page, a.id)).toBe("public");
  await page.getByRole("button", { name: "Done" }).click();

  await visitor.goto(`/a/${a.id}`);
  await expect(visitor).toHaveURL(new RegExp(`/a/${a.id}$`));
  await expect(visitor.frameLocator("iframe[data-artifact-frame]").locator("h1")).toHaveText("Hello from the page");
  await expect(visitor.getByRole("radiogroup", { name: "Who can open this" })).toHaveCount(0);

  const aliceSees = page.getByRole("banner").getByRole("group", { name: "Viewing now" });
  const guests = aliceSees.getByRole("button", { name: / \(Guest\)$/ });
  const nickname = await ownGuestName(visitor);
  expect(nickname).toMatch(/^[A-Z][a-z]+ [A-Z][a-z]+( #[0-9a-f]+)?$/);
  await expect(aliceSees.getByRole("button", { name: `${nickname} (Guest)`, exact: true })).toBeVisible();
  await expect(guests).toHaveCount(1);

  await visitor.reload();
  const secondTab = watch(await anon.newPage());
  await secondTab.goto(`/a/${a.id}`);
  expect(await ownGuestName(visitor)).toBe(nickname);
  expect(await ownGuestName(secondTab)).toBe(nickname);
  await expect(guests).toHaveCount(1);

  const phone = await browser.newContext({ baseURL: base, viewport: { width: 390, height: 844 } });
  await phone.addInitScript(() => {
    Object.defineProperty(window, "localStorage", { get: () => { throw new DOMException("site data is blocked", "SecurityError"); } });
  });
  const other = watch(await phone.newPage());
  const react = await publish(page, { title: "Public React", kind: "react", content: 'export default function App() { return <h1>Hello from React</h1>; }\n', visibility: "public" });
  let phoneNickname = "";
  for (const [artifact, heading] of [[a, "Hello from the page"], [react, "Hello from React"]] as const) {
    await page.goto(`/a/${artifact.id}`);
    await other.goto(`/a/${artifact.id}`);
    await expect(other.frameLocator("iframe[data-artifact-frame]").locator("h1")).toHaveText(heading);
    await other.getByRole("button", { name: "More actions" }).click();
    const phoneGuest = other.getByRole("region", { name: "Viewing now" }).getByText(/ \(You\) · Guest$/);
    await expect(phoneGuest).toBeVisible();
    phoneNickname ||= (await phoneGuest.innerText()).replace(/ \(You\) · Guest$/, "");
    await expect(phoneGuest).toHaveText(`${phoneNickname} (You) · Guest`);
    await expect(aliceSees.getByRole("button", { name: `${phoneNickname} (Guest)`, exact: true })).toBeVisible();
    if (artifact === a) {
      await expect(guests).toHaveCount(2);
      const names = await guests.evaluateAll((els) => els.map((el) => el.querySelector(".visually-hidden")?.textContent ?? ""));
      expect(new Set(names).size).toBe(2);
    }
  }
  await phone.close();
  await expect(guests).toHaveCount(0, { timeout: goneSlack });
  await page.goto(`/a/${a.id}`);
  await expect(guests).toHaveCount(1, { timeout: goneSlack });

  // The second tab alone keeps the guest here, beat after beat, past the time a closed one is forgotten.
  await visitor.close();
  for (let beat = 0; beat < 5; beat++) await page.waitForResponse((r) => r.url().endsWith(`/api/artifacts/${a.id}/viewers`) && r.request().method() === "POST");
  await expect(aliceSees.getByRole("button", { name: `${nickname} (Guest)`, exact: true })).toBeVisible();

  await signIn(secondTab, "bob");
  await secondTab.goto(`/a/${a.id}`);
  let withBob: { name: string; guest: boolean }[] = [];
  await expect.poll(async () => (withBob = await seenByAlice(page, a.id)).some((e) => e.name === "BOB")).toBe(true);
  expect(withBob.filter((e) => e.guest)).toEqual([]);
  await expect(aliceSees.getByRole("button", { name: "BOB", exact: true })).toBeVisible();
  await expect(guests).toHaveCount(0);
  // Bob may not open the page once it is private, so his open tab would be refused its next heartbeat (a 404 the
  // browser logs) when the link closes below; that refusal belongs to losing access, not to this step.
  await secondTab.close();

  const late = await browser.newContext({ baseURL: base });
  const lastVisitor = watch(await late.newPage());
  await lastVisitor.goto(`/a/${a.id}`);
  await expect(guests).toHaveCount(1);
  // The last visitor's open page is refused its next heartbeat once the link closes, which the browser logs; that
  // refusal, of that page's heartbeat after the link closed, is the only one allowed.
  let closed = false;
  const refused: string[] = [];
  lastVisitor.on("response", (r) => {
    if (r.status() === 401) refused.push(`${closed ? "after" : "before"} ${r.request().method()} ${new URL(r.url()).pathname}`);
  });
  expectConsoleErrors(new RegExp(`status of 401 .*\\(${base.replace(/[.:/]/g, "\\$&")}/api/artifacts/${a.id}/viewers\\)$`));
  await page.getByRole("button", { name: "Share", exact: true }).click();
  await presented(page.getByRole("dialog"));
  closed = true;
  await page.getByRole("radiogroup", { name: "Who can open this" }).getByRole("radio", { name: /^Only me/ }).click();
  await expect.poll(() => visibilityOf(page, a.id)).toBe("private");
  // A heartbeat of every open page falls in this wait, so any other refusal than the expected one shows.
  await lastVisitor.waitForResponse((r) => r.url().endsWith(`/api/artifacts/${a.id}/viewers`) && r.status() === 401);
  await page.waitForResponse((r) => r.url().endsWith(`/api/artifacts/${a.id}/viewers`) && r.request().method() === "POST");
  expect((await seenByAlice(page, a.id)).filter((e) => e.guest)).toEqual([]);
  await lastVisitor.goto(`/a/${a.id}`);
  await expect(lastVisitor).toHaveURL(/\/login/);
  expect(refused.length > 0 && refused.every((r) => r === `after POST /api/artifacts/${a.id}/viewers`), refused.join(", ")).toBe(true);
  await late.close();
  await anon.close();
});
