import { expect, png, publish, republish, signIn, test, upload } from "./support";

const frameOf = (page: import("@playwright/test").Page) => page.frameLocator("iframe[data-artifact-frame]");

test("bob copies alice's organisation artifact with its picture, lands on his private copy and edits it; alice's is unchanged", async ({ page, browser, watch }) => {
  await signIn(page, "alice");
  const a = await publish(page, { title: "Team plan", visibility: "organisation", content: "<!doctype html><p>original</p>" });
  await upload(page, a.id, "chart.png", png);
  await republish(page, a.id, {
    title: "Team plan",
    baseVersion: a.version,
    content: `<!doctype html><html><head></head><body><p>original</p><img id="chart" src="/files/${a.id}/chart.png" alt="Chart"></body></html>`,
  });

  const bobContext = await browser.newContext({ baseURL: page.url().split("/").slice(0, 3).join("/") });
  const bob = watch(await bobContext.newPage());
  await signIn(bob, "bob");
  await bob.goto(`/a/${a.id}`);
  await bob.getByRole("button", { name: "More actions" }).click();
  await bob.getByRole("menuitem", { name: "Make a copy" }).click();

  // He lands on his own copy, private, that says where it came from.
  await expect(bob).toHaveURL(new RegExp(`/a/(?!${a.id})[a-z0-9]+$`));
  const copyId = bob.url().split("/").pop() ?? "";
  await expect(bob.getByRole("heading", { name: "Team plan (Copy)" })).toBeVisible();
  await bob.getByRole("button", { name: "Share", exact: true }).click();
  await expect(bob.getByRole("dialog").getByRole("radio", { name: /^Only me/ })).toBeChecked();
  await bob.getByRole("button", { name: "Done" }).click();
  await expect(bob.getByRole("link", { name: "Copied from Team plan" })).toHaveAttribute("href", `/a/${a.id}`);
  await expect(frameOf(bob).getByText("original")).toBeVisible();
  // The picture is the copy's own file.
  const chart = frameOf(bob).locator("#chart");
  await expect(chart).toHaveAttribute("src", new RegExp(`/files/${copyId}/[^/]+/chart\\.png`));
  await expect.poll(() => chart.evaluate((img: HTMLImageElement) => img.naturalWidth)).toBe(1);

  // He edits his copy; alice's is as it was.
  await republish(bob, copyId, { title: "Bob's plan", baseVersion: 1, content: "<!doctype html><p>bob changed this</p>" });
  await bob.reload();
  await expect(frameOf(bob).getByText("bob changed this")).toBeVisible();
  await page.goto(`/a/${a.id}`);
  await expect(page.getByRole("heading", { name: "Team plan", exact: true })).toBeVisible();
  await expect(frameOf(page).getByText("original")).toBeVisible();
  await expect(frameOf(page).getByText("bob changed this")).toHaveCount(0);
  await bobContext.close();
});
