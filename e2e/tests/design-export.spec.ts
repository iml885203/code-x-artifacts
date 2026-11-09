import { mkdtempSync, readFileSync, readdirSync, statSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";
import { expect, publish, republish, signIn, test, upload, png } from "./support";
import { pdfPages, pdfPageSizes } from "./pdf";
import { extractZip } from "./zip";

const frame = (body: string) => `<!doctype html><html><head><meta charset="utf-8"></head><body style="margin:0">${body}</body></html>`;

const manifest = JSON.stringify({
  version: 1,
  pages: [
    {
      id: "home",
      name: "Home",
      rows: [
        [
          { id: "hero", name: "Hero", file: "frames/hero.html", w: 800, h: 600 },
          { id: "mobile", name: "Mobile", file: "frames/mobile.html", w: 390, h: 844 },
        ],
      ],
    },
    { id: "extra", name: "Extra", rows: [[{ id: "footer", name: "Footer", file: "frames/footer.html", w: 800, h: 200 }]] },
  ],
});

const plain = {
  "frames/hero.html": frame(`<h1 style="font-size:40px">HEROWORD</h1>`),
  "frames/mobile.html": frame(`<h1 style="font-size:28px">MOBILEWORD</h1>`),
  "frames/footer.html": frame(`<p>FOOTERWORD</p>`),
};

test("a design's zip opens from file:// and shows every frame with its picture, without asking any server", async ({ page, browser }) => {
  await signIn(page, "alice");
  const a = await publish(page, { title: "Landing page", kind: "design", content: manifest, files: plain });
  await upload(page, a.id, "logo.png", png);
  await republish(page, a.id, {
    title: "Landing page",
    kind: "design",
    content: manifest,
    baseVersion: a.version,
    files: { "frames/hero.html": frame(`<h1>HEROWORD</h1><img id="logo" src="/files/${a.id}/logo.png" alt="logo" width="40" height="40">`) },
  });

  await page.goto(`/a/${a.id}`);
  await page.getByRole("button", { name: "More actions" }).click();
  const downloading = page.waitForEvent("download");
  await page.getByRole("menuitem", { name: "Zip (.zip)" }).click();
  const download = await downloading;
  expect(download.suggestedFilename()).toBe("Landing page.zip");
  const dir = mkdtempSync(join(tmpdir(), "pictor-design-zip-"));
  const archive = join(dir, "landing.zip");
  await download.saveAs(archive);
  extractZip(archive, join(dir, "out"));
  const root = join(dir, "out");
  expect(readdirSync(root).sort()).toEqual(expect.arrayContaining(["index.html", "design.json", "frames", "assets"]));
  expect(statSync(join(root, "assets", "logo.png")).size).toBeGreaterThan(0);
  expect(readFileSync(join(root, "frames", "hero.html"), "utf8")).toContain("../assets/logo.png");

  // A browser with nothing to reach but the files: every request it makes is for a file: address.
  const context = await browser.newContext({ offline: true });
  const offline = await context.newPage();
  const requested: string[] = [];
  offline.on("request", (r) => requested.push(r.url()));
  await offline.goto(pathToFileURL(join(root, "index.html")).href);
  for (const [name, word] of [["Hero", "HEROWORD"], ["Mobile", "MOBILEWORD"], ["Footer", "FOOTERWORD"]] as const) {
    await expect(offline.frameLocator(`iframe[title="${name}"]`).locator("body")).toContainText(word);
  }
  await expect(offline.frameLocator('iframe[title="Hero"]').locator("#logo")).toHaveJSProperty("naturalWidth", 1);
  expect(requested.filter((u) => !u.startsWith("file:")), "nothing is requested from a server").toEqual([]);
  await context.close();
});

test("a design prints one page per frame at the size of the frame, with the text still text", async ({ page }) => {
  await page.addInitScript(() => {
    window.print = () => {
      (window as unknown as { printed?: string }).printed = document.fonts.status;
    };
  });
  await signIn(page, "alice");
  const a = await publish(page, { title: "Landing page", kind: "design", visibility: "organisation", content: manifest, files: plain });
  await upload(page, a.id, "logo.png", png);
  await republish(page, a.id, {
    title: "Landing page", kind: "design", content: manifest, baseVersion: a.version,
    files: { "frames/footer.html": frame(`<style>@font-face { font-family: PrintFont; src: url('/static/fonts/FamiljenGrotesk.woff2'); } p { font-family: PrintFont; }</style><p>FOOTERWORD</p><img src="/files/${a.id}/logo.png" alt="logo" width="40" height="40">`) },
  });
  // A reader can export too; the PDF comes from the whole overview, not the canvas's current page.
  await signIn(page, "bob");
  await page.goto(`/a/${a.id}`);
  await page.getByRole("button", { name: "More actions" }).click();
  await page.getByRole("menuitem", { name: "PDF" }).click();
  const printing = page.locator("iframe.print-frame");
  await expect(printing).toBeAttached();
  // It must be off-screen even when the lazy Docs editor (which has its own CSS) has never loaded.
  expect(await printing.evaluate((el) => el.getBoundingClientRect().right)).toBeLessThanOrEqual(0);
  await expect.poll(async () => printing.contentFrame().locator("body").evaluate(() => (window as unknown as { printed?: string }).printed)).toBe("loaded");
  expect(await page.evaluate(() => (window as unknown as { printed?: string }).printed)).toBeUndefined();
  const overview = printing.contentFrame();
  await expect(overview.frameLocator('iframe[title="Footer"]').getByAltText("logo")).toHaveJSProperty("naturalWidth", 1);
  expect(await overview.frameLocator('iframe[title="Footer"]').locator("p").evaluate(() => document.fonts.check("16px PrintFont"))).toBe(true);
  // Playwright prints a top-level page. Open the exact overview the menu printed and inspect its PDF.
  const printedURL = await overview.locator("body").evaluate(() => location.href);
  await page.goto(printedURL);
  for (const [name, word] of [["Hero", "HEROWORD"], ["Mobile", "MOBILEWORD"], ["Footer", "FOOTERWORD"]] as const) {
    await expect(page.frameLocator(`iframe[title="${name}"]`).locator("body")).toContainText(word);
  }
  // As the print dialog does: the page's own @page sizes.
  await page.emulateMedia({ media: "print" });
  const pdf = await page.pdf({ preferCSSPageSize: true, printBackground: true });
  // A PDF counts in points, so a size can come back a pixel off.
  const sizes = await pdfPageSizes(pdf);
  expect(sizes).toHaveLength(3);
  [[800, 600], [390, 844], [800, 200]].forEach(([w = 0, h = 0], i) => {
    expect(Math.abs((sizes[i]?.width ?? 0) - w), `page ${String(i + 1)} width`).toBeLessThanOrEqual(1);
    expect(Math.abs((sizes[i]?.height ?? 0) - h), `page ${String(i + 1)} height`).toBeLessThanOrEqual(1);
  });
  const pages = await pdfPages(pdf);
  expect(pages[0]).toContain("HEROWORD");
  expect(pages[1]).toContain("MOBILEWORD");
  expect(pages[2]).toContain("FOOTERWORD");
  expect(pdf.includes("FamiljenGrotesk"), "the frame's loaded font is embedded in the PDF").toBe(true);
});
