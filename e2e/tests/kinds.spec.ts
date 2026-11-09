import { expect, png, publish, republish, signIn, test, upload } from "./support";

const frameOf = (page: import("@playwright/test").Page) => page.frameLocator("iframe[data-artifact-frame]");

test("a react page renders in the sandbox from the libraries the server carries", async ({ page }) => {
  await signIn(page, "alice");
  const a = await publish(page, {
    title: "Counter",
    kind: "react",
    content: `import { useState } from "react";
import { Button } from "@/components/ui/button";

export default function App() {
  const [n, setN] = useState(0);
  return <div><h1>Clicked {n}</h1><Button onClick={() => setN(n + 1)}>Add one</Button></div>;
}
`,
  });
  await page.goto(`/a/${a.id}`);
  const frame = frameOf(page);
  await expect(frame.getByRole("heading", { name: "Clicked 0" })).toBeVisible();
  await frame.getByRole("button", { name: "Add one" }).click();
  await expect(frame.getByRole("heading", { name: "Clicked 1" })).toBeVisible();
});

test("a file uploaded to an html page loads inside the sandbox", async ({ page }) => {
  await signIn(page, "alice");
  const a = await publish(page, { title: "Picture", content: "<!doctype html><p>x</p>" });
  await upload(page, a.id, "chart.png", png);
  await republish(page, a.id, {
    title: "Picture",
    baseVersion: a.version,
    content: `<!doctype html><html><head></head><body><img id="chart" src="/files/${a.id}/chart.png" alt="Chart"></body></html>`,
  });
  await page.goto(`/a/${a.id}`);
  const loaded = frameOf(page).locator("#chart");
  await expect(loaded).toBeVisible();
  await expect.poll(() => loaded.evaluate((img: HTMLImageElement) => img.naturalWidth)).toBe(1);
});

test("a file uploaded to a react page loads inside the sandbox", async ({ page }) => {
  await signIn(page, "alice");
  const a = await publish(page, { title: "Chart", kind: "react", content: "export default function App() { return <p>x</p>; }\n" });
  await upload(page, a.id, "chart.png", png);
  await republish(page, a.id, {
    title: "Chart",
    kind: "react",
    baseVersion: a.version,
    content: `export default function App() { return <img id="chart" src="/files/${a.id}/chart.png" alt="Chart" />; }\n`,
  });
  await page.goto(`/a/${a.id}`);
  const loaded = frameOf(page).locator("#chart");
  await expect(loaded).toBeVisible();
  await expect.poll(() => loaded.evaluate((img: HTMLImageElement) => img.naturalWidth)).toBe(1);
});

test("a doc shows its text and an uploaded image in the sandbox, to someone who is not signed in, and runs no script it was not served with", async ({ page, browser, watch, expectConsoleErrors }) => {
  // The browser logs the inline script the policy refused; refusing it is the point.
  expectConsoleErrors(/inline script.* violates the following Content Security Policy directive/i);
  await signIn(page, "alice");
  const a = await publish(page, { title: "Notes", kind: "doc", content: "# Plan\n\nWords here.\n", visibility: "public" });
  await upload(page, a.id, "chart.png", png);
  await republish(page, a.id, { title: "Notes", kind: "doc", baseVersion: a.version, content: `# Plan\n\nWords here.\n\n![Chart](/files/${a.id}/chart.png)\n` });
  // A signed-in person opens a doc in the editor; the page in its sandboxed frame is what everyone else gets.
  const anon = await browser.newContext({ baseURL: page.url().split("/").slice(0, 3).join("/") });
  const visitor = watch(await anon.newPage());
  await visitor.goto(`/a/${a.id}`);
  const frame = frameOf(visitor);
  await expect(frame.getByRole("heading", { name: "Plan" })).toBeVisible();
  await expect.poll(() => frame.getByRole("img", { name: "Chart" }).evaluate((img: HTMLImageElement) => img.naturalWidth)).toBe(1);
  // Text that a rendering bug turned into markup is what script-src is for: an inline script the server did not inject does not run.
  const injected = await frame.locator("body").evaluate(
    () =>
      new Promise<string>((resolve) => {
        document.addEventListener("securitypolicyviolation", () => resolve("blocked"), { once: true });
        const s = document.createElement("script");
        s.textContent = "window.__injected = true;";
        document.body.append(s);
        if ("__injected" in window) resolve("ran");
      }),
  );
  expect(injected).toBe("blocked");
  await anon.close();
});

test("a react, a doc and a slides artifact open with the same chrome and none of it names a kind", async ({ page }) => {
  await signIn(page, "alice");
  const made = [
    await publish(page, { title: "Chrome check", kind: "react", content: "export default function App() { return <p>x</p>; }\n" }),
    await publish(page, { title: "Chrome check", kind: "doc", content: "# Plan\n\nWords here.\n" }),
    await publish(page, {
      title: "Chrome check",
      kind: "slides",
      content: JSON.stringify({ version: 1, slides: [{ layout: "title", title: "Hello", notes: "Hi" }] }),
    }),
  ];
  const chromes: string[] = [];
  for (const a of made) {
    await page.goto(`/a/${a.id}`);
    await expect(page.getByRole("heading", { name: "Chrome check" })).toBeVisible();
    await page.getByRole("button", { name: "More actions" }).click();
    await expect(page.getByRole("menuitem", { name: "Open in new tab" })).toBeVisible();
    const items = (await page.getByRole("menuitem").allTextContents()).filter((t) => !/^(HTML|Markdown|Word|PDF|PowerPoint)/.test(t));
    // What a doc says about being saved is the editor's, so it is left out of what the kinds must share.
    const facts = (await page.locator(".vtitle .facts").evaluate((el) => {
      const clone = el.cloneNode(true) as HTMLElement;
      clone.querySelectorAll("[role=status]").forEach((s) => { s.remove(); });
      return clone.textContent;
    })).replace(/\s+/g, " ");
    // Everything a person reads outside the artifact's own frame (not the boot data in script tags).
    const text = await page.locator("body").evaluate((body) => {
      const clone = body.cloneNode(true) as HTMLElement;
      clone.querySelectorAll("iframe, script, style").forEach((f) => { f.remove(); });
      return clone.textContent;
    });
    expect(text).not.toMatch(/(^|[^\w.])(react|doc|slides|html|kind|format)($|[^\w])/i);
    chromes.push(JSON.stringify({ facts, items }));
  }
  expect(new Set(chromes).size).toBe(1);
});
