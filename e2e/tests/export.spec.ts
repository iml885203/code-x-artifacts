import { expect, publish, signIn, test } from "./support";

test("Export, Markdown downloads a file named after the title, with the doc's own text, and the menu opens again for the next format", async ({ page }) => {
  await signIn(page, "alice");
  const a = await publish(page, { title: "Q3 計畫", kind: "doc", content: "# Plan\n\n- [x] ship\n\n| a | b |\n| :-- | --: |\n| 1 | 2 |\n" });
  await page.goto(`/a/${a.id}`);
  await page.getByRole("button", { name: "More actions" }).click();
  const downloading = page.waitForEvent("download");
  await page.getByRole("menuitem", { name: "Markdown (.md)" }).click();
  const download = await downloading;
  expect(download.suggestedFilename()).toBe("Q3 計畫.md");
  const stream = await download.createReadStream();
  const chunks: Buffer[] = [];
  for await (const chunk of stream) chunks.push(chunk as Buffer);
  const text = Buffer.concat(chunks).toString("utf8");
  expect(text).toContain("# Plan");
  expect(text).toContain("- [x] ship");
  expect(text).toContain("| :-- | --: |");

  // The menu opens again after a download, with no reload in between.
  await page.getByRole("button", { name: "More actions" }).click();
  const again = page.waitForEvent("download");
  await page.getByRole("menuitem", { name: "Word (.docx)" }).click();
  expect((await again).suggestedFilename()).toBe("Q3 計畫.docx");
});

test("the Export menu of an html page lists HTML and nothing else", async ({ page }) => {
  await signIn(page, "alice");
  const a = await publish(page, { title: "Plain page", content: "<!doctype html><p>x</p>" });
  await page.goto(`/a/${a.id}`);
  await page.getByRole("button", { name: "More actions" }).click();
  await expect(page.getByRole("menuitem", { name: "HTML (.html)" })).toBeVisible();
  await expect(page.getByRole("menuitem", { name: "Markdown (.md)" })).toHaveCount(0);
});

test("Export, Word downloads a .docx that is a zip with the document parts", async ({ page }) => {
  await signIn(page, "alice");
  const a = await publish(page, { title: "週報 Report", kind: "doc", content: "# 標題 😀\n\n- one\n- two\n\n| a | b |\n| - | - |\n| 1 | 2 |\n" });
  await page.goto(`/a/${a.id}`);
  await page.getByRole("button", { name: "More actions" }).click();
  const downloading = page.waitForEvent("download");
  await page.getByRole("menuitem", { name: "Word (.docx)" }).click();
  const download = await downloading;
  expect(download.suggestedFilename()).toBe("週報 Report.docx");
  const stream = await download.createReadStream();
  const chunks: Buffer[] = [];
  for await (const chunk of stream) chunks.push(chunk as Buffer);
  const file = Buffer.concat(chunks);
  expect(file.subarray(0, 4).toString("latin1")).toBe("PK\x03\x04");
  for (const part of ["[Content_Types].xml", "word/document.xml", "word/styles.xml", "word/numbering.xml"]) {
    expect(file.includes(Buffer.from(part))).toBe(true);
  }
});

for (const title of ["Landing", "週報"]) {
  test(`Export saves a doc titled "${title}", a name with no space to quote, under that name`, async ({ page }) => {
    await signIn(page, "alice");
    const a = await publish(page, { title, kind: "doc", content: "# Plan\n\nWords.\n" });
    await page.goto(`/a/${a.id}`);
    await page.getByRole("button", { name: "More actions" }).click();
    const downloading = page.waitForEvent("download");
    await page.getByRole("menuitem", { name: "Markdown (.md)" }).click();
    expect((await downloading).suggestedFilename()).toBe(`${title}.md`);
  });
}
