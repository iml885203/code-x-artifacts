import { expect, png, publish, republish, signIn, test } from "./support";

const frameOf = (page: import("@playwright/test").Page) => page.frameLocator("iframe[data-artifact-frame]");

async function openFiles(page: import("@playwright/test").Page) {
  await page.getByRole("button", { name: "More actions" }).click();
  await page.getByRole("menuitem", { name: "Files" }).click();
  return page.getByRole("complementary", { name: "Files" });
}

test("the author uploads a picture from the viewer, puts its URL in the page and it shows, then deletes it", async ({ page, context, expectConsoleErrors }) => {
  // The refused upload is answered 400, which the browser logs.
  expectConsoleErrors(/status of 400/);
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await signIn(page, "alice");
  const a = await publish(page, { title: "Picture", content: "<!doctype html><p>x</p>" });
  await page.goto(`/a/${a.id}`);
  const files = await openFiles(page);
  await expect(files.getByText("No files yet")).toBeVisible();

  await files.getByLabel("Drop files here, or choose files").setInputFiles({ name: "chart.png", mimeType: "image/png", buffer: png });
  await expect(files.locator(".file-name", { hasText: "chart.png" })).toBeVisible();

  // The URL the button copies is the one the page uses to show the file.
  await files.getByRole("button", { name: "Copy URL" }).click();
  const url = await page.evaluate(() => navigator.clipboard.readText());
  expect(url).toMatch(new RegExp(`/files/${a.id}/chart\\.png$`));
  await republish(page, a.id, {
    title: "Picture",
    baseVersion: a.version,
    content: `<!doctype html><html><head></head><body><img id="chart" src="${url}" alt="Chart"></body></html>`,
  });
  await page.goto(`/a/${a.id}`);
  const loaded = frameOf(page).locator("#chart");
  await expect.poll(() => loaded.evaluate((img: HTMLImageElement) => img.naturalWidth)).toBe(1);

  const again = await openFiles(page);
  await again.getByLabel("Drop files here, or choose files").setInputFiles({ name: "notes.png", mimeType: "image/png", buffer: Buffer.from("this is text, not a picture") });
  await expect(again.getByRole("alert")).toContainText("Couldn't upload notes.png");
  await expect(again.locator(".file-name", { hasText: "notes.png" })).toHaveCount(0);

  await again.getByRole("button", { name: "Delete chart.png" }).click();
  await page.getByRole("alertdialog").getByRole("button", { name: "Delete" }).click();
  await expect(again.getByText("No files yet")).toBeVisible();
});

test("someone who is not the author has no Files in the menu", async ({ page, browser, watch }) => {
  await signIn(page, "alice");
  const a = await publish(page, { title: "Theirs", content: "<!doctype html><p>x</p>", visibility: "organisation" });
  const bobContext = await browser.newContext({ baseURL: page.url().split("/").slice(0, 3).join("/") });
  const bob = watch(await bobContext.newPage());
  await signIn(bob, "bob");
  await bob.goto(`/a/${a.id}`);
  await bob.getByRole("button", { name: "More actions" }).click();
  await expect(bob.getByRole("menuitem", { name: "Open in new tab" })).toBeVisible();
  await expect(bob.getByRole("menuitem", { name: "Files" })).toHaveCount(0);
  await bobContext.close();
});
