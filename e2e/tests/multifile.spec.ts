import type { Page } from "@playwright/test";
import { expect, pickVersion, publish, republish, signIn, test } from "./support";

const frameOf = (page: Page) => page.frameLocator("iframe[data-artifact-frame]");

// A page that comes with a script, a stylesheet, data, a module with a static
// and a dynamic import, and a stylesheet that refers to another file: every way
// a page names a file, by a relative path from inside a sandbox with no origin.
const indexPage = `<!doctype html><html><head><meta charset="utf-8"><title>Two files</title>
<link rel="stylesheet" href="styles/site.css"></head>
<body><p id="out">not run</p><p id="styled">styled</p><a id="next" href="other.html">other</a><a id="nested" href="sub/page.html">nested</a>
<script src="app.js"></script><script type="module" src="./lib/main.mjs"></script></body></html>`;

const files = {
  "app.js": `document.getElementById("out").textContent = "js ran";`,
  "styles/site.css": `#styled { color: rgb(1, 2, 3); background: url(../img/dot.svg); }`,
  "lib/main.mjs": `import { word } from "./dep.mjs";
const rows = await (await fetch(new URL("../data/rows.json", import.meta.url))).json();
const lazy = await import("./lazy.mjs");
document.body.dataset["module"] = [word, rows.length, lazy.word].join(",");`,
  "lib/dep.mjs": `export const word = "static";`,
  "lib/lazy.mjs": `export const word = "dynamic";`,
  "data/rows.json": `[1, 2, 3]`,
  "other.html": `<!doctype html><html><head><title>Other</title></head><body><h1 id="h">other page</h1><a id="back" href="index.html">back</a><a id="in-page" href="#bottom">jump</a><p id="bottom">bottom</p></body></html>`,
  "sub/page.html": `<!doctype html><html><head><title>Nested</title><link rel="stylesheet" href="../styles/site.css"></head><body><h1 id="h">nested page</h1><a id="up" href="../other.html">up</a><p id="out">x</p><script src="../app.js"></script></body></html>`,
};

test("a two-file page runs: its script, stylesheet, module imports and data all load from inside the sandbox", async ({ page }) => {
  await signIn(page, "alice");
  const a = await publish(page, { title: "Two files", content: indexPage, files });
  await page.goto(`/a/${a.id}`);
  const frame = frameOf(page);
  await expect(frame.locator("#out")).toHaveText("js ran");
  await expect(frame.locator("#styled")).toHaveCSS("color", "rgb(1, 2, 3)");
  await expect(frame.locator("body")).toHaveAttribute("data-module", "static,3,dynamic");

  // The page is still in an opaque origin: it has no storage of its own.
  const origin = await frame.locator("body").evaluate(() => window.origin);
  expect(origin).toBe("null");
});

test("links between the pages of one artifact work, a nested page reaches its neighbours, and an in-page anchor stays on the page", async ({ page }) => {
  await signIn(page, "alice");
  const a = await publish(page, { title: "Two files", content: indexPage, files });
  await page.goto(`/a/${a.id}`);
  const frame = frameOf(page);
  await frame.locator("#next").click();
  await expect(frame.locator("#h")).toHaveText("other page");
  // window.pictor is in every page of the artifact.
  expect(await frame.locator("body").evaluate(() => typeof (window as unknown as { pictor?: unknown }).pictor)).toBe("object");
  await frame.locator("#in-page").click();
  await expect(frame.locator("#h")).toHaveText("other page");
  await frame.locator("#back").click();
  await expect(frame.locator("#out")).toHaveText("js ran");
  await frame.locator("#nested").click();
  await expect(frame.locator("#h")).toHaveText("nested page");
  await expect(frame.locator("#out")).toHaveText("js ran");
  await frame.locator("#up").click();
  await expect(frame.locator("#h")).toHaveText("other page");
});

test("the author looking at an older version sees that version's files, and a reader without access gets nothing", async ({ page, browser, watch, expectConsoleErrors }) => {
  // The reader's refused requests are answered 404, which the browser logs.
  expectConsoleErrors(/status of 404/);
  await signIn(page, "alice");
  const a = await publish(page, { title: "Versions", content: indexPage, files });
  await republish(page, a.id, { title: "Versions", content: indexPage, baseVersion: a.version, files: { "app.js": `document.getElementById("out").textContent = "second";` } });
  await page.goto(`/a/${a.id}`);
  await expect(frameOf(page).locator("#out")).toHaveText("second");
  await page.getByRole("button", { name: "More actions" }).click();
  await page.getByRole("menuitem", { name: "Versions" }).click();
  const panel = page.getByRole("complementary", { name: "Versions" });
  await pickVersion(panel, 1);
  await expect(page.getByText("Viewing v1, read only")).toBeVisible();
  await expect(frameOf(page).locator("#out")).toHaveText("js ran");
  await panel.getByRole("button", { name: "Back to the newest" }).click();
  await expect(frameOf(page).locator("#out")).toHaveText("second");

  // Bob cannot see the private artifact: neither its page nor any file of it.
  const bobContext = await browser.newContext({ baseURL: page.url().split("/").slice(0, 3).join("/") });
  const bob = watch(await bobContext.newPage());
  await signIn(bob, "bob");
  const statuses = await bob.evaluate(async (id) => {
    const out: number[] = [];
    for (const p of [`/raw/${id}`, `/raw/${id}/1.9999999999.x/`, `/raw/${id}/1.9999999999.x/app.js`, `/api/artifacts/${id}/files/app.js`]) {
      out.push((await fetch(p, { headers: { "X-Pictor": "1" } })).status);
    }
    return out;
  }, a.id);
  expect(statuses).toEqual([404, 404, 404, 404]);
  await bobContext.close();
});

test("another artifact's script cannot be loaded into a react page", async ({ page, expectConsoleErrors }) => {
  // The browser logs the load the policy refused; refusing it is the point.
  expectConsoleErrors(/Content Security Policy/);
  await signIn(page, "alice");
  const evil = await publish(page, { title: "Evil", content: `<!doctype html><p>x</p>`, files: { "evil.js": `window.__evil = true;` } });
  const evilUrl = (await grantAddress(page, evil.id)) + "evil.js";
  // The victim comes with a file, so its own /raw/{id}/ path is allowed: only its own.
  const victim = await publish(page, { title: "Victim", kind: "react", content: "export default function App() { return <p>victim</p>; }", files: { "own.js": `window.__own = true;` } });
  const ownUrl = (await grantAddress(page, victim.id)) + "own.js";
  await page.goto(`/a/${victim.id}`);
  const frame = frameOf(page);
  await expect(frame.getByText("victim")).toBeVisible();
  // Its own file is allowed by the same policy that refuses the other's.
  expect(await load(frame, ownUrl)).toBe("ran");
  expect(await frame.locator("body").evaluate(() => (window as unknown as { __own?: boolean }).__own)).toBe(true);
  // Whoever can write markup into the page can add a script element; the CSP is what stops it running.
  const outcome = await load(frame, evilUrl);
  expect(outcome).toBe("blocked");
  expect(await frame.locator("body").evaluate(() => "__evil" in window)).toBe(false);
});


/** The address the browser is sent to for an artifact that has files: the one that carries the grant. */
async function grantAddress(page: Page, id: string): Promise<string> {
  const url = await page.evaluate(async (id) => (await fetch(`/raw/${id}`)).url, id);
  expect(url).toMatch(new RegExp(`/raw/${id}/\\d+\\.\\d+\\.[\\w-]+/$`));
  return url;
}

/** Adds a script element for url to the frame's page and says whether it ran or the policy blocked it. */
function load(frame: ReturnType<typeof frameOf>, url: string): Promise<string> {
  return frame.locator("body").evaluate(
    (_body, src) =>
      new Promise<string>((resolve) => {
        const blocked = () => resolve("blocked");
        document.addEventListener("securitypolicyviolation", blocked, { once: true });
        const s = document.createElement("script");
        s.src = src;
        s.onload = () => resolve("ran");
        document.body.append(s);
      }),
    url,
  );
}

const reactApp = `import { useEffect, useState } from "react";
import Chart from "./Chart";
import "./styles.css";
export default function App() {
  const [fetched, setFetched] = useState("loading");
  useEffect(() => {
    fetch("extra.json").then((r) => r.json()).then((j) => setFetched(String(j.fetched)));
  }, []);
  return <main className="chart"><Chart /><p id="fetched">{fetched}</p></main>;
}`;

test("a react component imports its sibling files and its page fetches a relative JSON file", async ({ page }) => {
  await signIn(page, "alice");
  const a = await publish(page, {
    title: "Chart",
    kind: "react",
    content: reactApp,
    files: {
      "Chart.tsx": `import { fmt } from "./lib/format"; import rows from "./data/rows.json"; export default function Chart() { return <p id="out">{fmt(rows.length)}</p>; }`,
      "lib/format.ts": `export const fmt = (n: number): string => "rows=" + n;`,
      "data/rows.json": `[1, 2, 3]`,
      "styles.css": `.chart { color: rgb(1, 2, 3); }`,
      "extra.json": `{"fetched": true}`,
    },
  });
  await page.goto(`/a/${a.id}`);
  const frame = frameOf(page);
  await expect(frame.locator("#out")).toHaveText("rows=3");
  await expect(frame.locator("#fetched")).toHaveText("true");
  await expect(frame.locator("main")).toHaveCSS("color", "rgb(1, 2, 3)");
});
