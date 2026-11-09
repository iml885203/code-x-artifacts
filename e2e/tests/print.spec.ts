import { expect, presented, png, publish, republish, signIn, test, upload, type Person } from "./support";
import { document } from "./editor-support";
import { pdfPages, pdfPageSizes } from "./pdf";
import type { Page } from "@playwright/test";

const frameOf = (page: Page) => page.frameLocator("iframe[data-artifact-frame]");

/** A doc with Traditional Chinese, a table taller than a page whose rows run over several lines, and a picture-less ending. */
function longDoc(rows: number): string {
  const lines = ["# 季度計畫", "", "這是一份給同事的文件，包含繁體中文與一張很長的表格。", "", "| 編號 | 內容 |", "| :-- | :-- |"];
  for (let n = 1; n <= rows; n++) {
    lines.push(`| R${String(n)} | START${String(n)} ${"這一列的內容很長，會折成好幾行。 ".repeat(6)}END${String(n)} |`);
  }
  lines.push("", "完。");
  return lines.join("\n");
}

async function signedInWith(page: Page, who: Person, content: string, title = "季度計畫") {
  await signIn(page, who);
  return publish(page, { title, kind: "doc", content });
}

test("PDF in the Export menu asks a doc, a deck or a web page to print, not the viewer; a deck prints one 16:9 page per slide in order and a web page prints with its own print styles", async ({ page }) => {
  await page.addInitScript(() => {
    const count = window as unknown as { printed?: number };
    window.print = () => {
      count.printed = (count.printed ?? 0) + 1;
    };
  });
  const printedOnce = async (id: string) => {
    await page.getByRole("button", { name: "More actions" }).click();
    await presented(page.getByRole("menu"));
    await page.getByRole("menuitem", { name: "PDF" }).click();
    // The page, not a filmstrip thumbnail (the deck's preview).
    const inFrame = () => page.frames().find((f) => f.url().includes(`/raw/${id}`) && !f.url().includes("?preview"));
    await expect.poll(async () => inFrame()?.evaluate(() => (window as unknown as { printed?: number }).printed)).toBe(1);
    expect(await page.evaluate(() => (window as unknown as { printed?: number }).printed)).toBeUndefined();
  };
  const a = await signedInWith(page, "alice", "# Plan\n\nWords.\n");
  await page.goto(`/a/${a.id}`);
  await expect(page.locator(".ed[data-connection=live]")).toBeVisible();
  await printedOnce(a.id);

  const slides = [
    { layout: "title", title: "FIRSTWORD", notes: "NOTEWORD" },
    { title: "SECONDWORD", elements: [{ type: "bullets", items: ["a point"] }], notes: "NOTEWORD" },
    { title: "THIRDWORD", elements: [{ type: "text", text: "the end" }], notes: "NOTEWORD" },
  ];
  const deck = await publish(page, { title: "Quarter", kind: "slides", content: JSON.stringify({ version: 1, slides }) });
  // The deck is open at its second slide; the PDF still holds every slide from the first.
  await page.goto(`/a/${deck.id}?slide=2`);
  await expect(frameOf(page).getByRole("heading", { name: "SECONDWORD" })).toBeVisible();
  await printedOnce(deck.id);
  // Playwright prints a top-level page. Open the page the viewer asked to print and inspect its PDF.
  await page.goto(`/raw/${deck.id}?slide=2`);
  await expect(page.getByRole("heading", { name: "SECONDWORD" })).toBeVisible();
  // As the print dialog does: the page's own @page size.
  const pdf = await page.pdf({ preferCSSPageSize: true });
  const sizes = await pdfPageSizes(pdf);
  expect(sizes).toHaveLength(3);
  for (const size of sizes) {
    // A PDF counts in points, so a size can come back a pixel off.
    expect(Math.abs(size.width - 1920), "page width").toBeLessThanOrEqual(1);
    expect(Math.abs(size.height - 1080), "page height").toBeLessThanOrEqual(1);
  }
  const pages = await pdfPages(pdf);
  ["FIRSTWORD", "SECONDWORD", "THIRDWORD"].forEach((word, i) => {
    expect(pages[i], `page ${String(i + 1)}`).toContain(word);
  });
  const all = pages.join("\n");
  for (const left of ["NOTEWORD", "Full screen", "2 / 3"]) expect(all, `the PDF holds ${left}`).not.toContain(left);

  // A web page with files and an uploaded picture prints as its author styled it for paper.
  await page.goto("/"); // the API is called from a page of the site, not the opaque-origin /raw
  const report = await publish(page, { title: "週報", content: "<!doctype html><p>draft</p>" });
  await upload(page, report.id, "chart.png", png);
  await republish(page, report.id, {
    title: "週報",
    baseVersion: report.version,
    content: `<!doctype html><html><head><meta charset="utf-8"><link rel="stylesheet" href="print.css"></head>
<body><h1>本週營運報告</h1><button class="controls">SCREENONLY</button><img id="chart" loading="lazy" style="margin-top:3000px" src="/files/${report.id}/chart.png" alt="Chart"></body></html>`,
    files: { "print.css": "@media print { .controls { display: none; } }" },
  });
  await page.goto(`/a/${report.id}`);
  await expect(frameOf(page).getByRole("heading", { name: "本週營運報告" })).toBeVisible();
  await printedOnce(report.id);
  const shown = page.frames().find((f) => f.url().includes(`/raw/${report.id}`));
  expect(await shown?.evaluate(() => (window.document.getElementById("chart") as HTMLImageElement).naturalWidth), "the lazy picture loaded before printing").toBe(1);
  await page.goto(`/raw/${report.id}`);
  await expect(page.getByRole("heading", { name: "本週營運報告" })).toBeVisible();
  const reportText = (await pdfPages(await page.pdf({ preferCSSPageSize: true }))).join("\n");
  expect(reportText).toContain("本週營運報告");
  expect(reportText, "a control hidden by the page's print styles").not.toContain("SCREENONLY");

  await page.goto("/");
  const component = await publish(page, { title: "Clicks", kind: "react", content: "export default function App() { return <h1>REACTWORD</h1>; }" });
  await page.goto(`/a/${component.id}`);
  await expect(frameOf(page).getByRole("heading", { name: "REACTWORD" })).toBeVisible();
  await printedOnce(component.id);
});

test("the print preview of a doc in the editor hides the bar and the insert row and keeps the text", async ({ page }) => {
  const a = await signedInWith(page, "alice", longDoc(3));
  await page.goto(`/a/${a.id}`);
  await expect(page.locator(".vbar")).toBeVisible();
  await expect(page.getByRole("toolbar", { name: "Editing tools" })).toBeVisible();
  await expect(document(page).getByRole("heading", { name: "季度計畫" })).toBeVisible();
  await page.emulateMedia({ media: "print" });
  await expect(page.locator(".vbar")).toBeHidden();
  await expect(page.getByRole("toolbar", { name: "Editing tools" })).toBeHidden();
  await expect(document(page).getByRole("heading", { name: "季度計畫" })).toBeVisible();
});

test("a printed doc has no row of a long table cut between pages, no address in the margins and the loaded Chinese font", async ({ page, baseURL }) => {
  const rows = 40;
  const a = await signedInWith(page, "alice", longDoc(rows));
  await page.goto(`/raw/${a.id}`);
  await expect(page.getByRole("heading", { name: "季度計畫" })).toBeVisible();
  // As the print dialog does: the page's own @page size and margins, and the headers and footers a browser adds when a page leaves room for them.
  const pdf = await page.pdf({ displayHeaderFooter: true, preferCSSPageSize: true, printBackground: true });
  const pages = await pdfPages(pdf);
  expect(pages.length, "a table taller than a page").toBeGreaterThan(1);
  for (let n = 1; n <= rows; n++) {
    const start = pages.findIndex((p) => p.includes(`START${String(n)} `));
    const end = pages.findIndex((p) => p.includes(`END${String(n)}`));
    expect(start, `row ${String(n)} is printed`).toBeGreaterThanOrEqual(0);
    expect(end, `row ${String(n)} starts and ends on the same page`).toBe(start);
  }
  const all = pages.join("\n");
  const host = new URL(baseURL ?? "http://localhost").host;
  for (const leak of [host, "/raw/", "/a/" + a.id, a.id]) expect(all, `the printed text names ${leak}`).not.toContain(leak);
  expect(all.replace(/\s+/g, "")).toContain("季度計畫");
  expect(pdf.includes("NotoSansTC"), "the Chinese text is set in the font the page loaded").toBe(true);
});
