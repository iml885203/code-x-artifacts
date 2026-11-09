import { expect, mcpTool, nameEditors, personalToken, presented, publish, signIn, test } from "./support";

test("alice names bob, bob publishes from MCP and restores a version in the viewer, and has no Editors item or Delete", async ({ page, browser, watch }) => {
  const origin = (p: import("@playwright/test").Page) => p.url().split("/").slice(0, 3).join("/");
  await signIn(page, "alice");
  const a = await publish(page, { title: "Shared notes", content: "<!doctype html><p>one</p>" });
  await nameEditors(page, a.id, ["bob"]);

  const bobContext = await browser.newContext({ baseURL: origin(page) });
  const bob = watch(await bobContext.newPage());
  await signIn(bob, "bob");
  const token = await personalToken(bob);

  // Bob's agent publishes version 2 over MCP.
  const published = await mcpTool(origin(page), token, "publish", { id: a.id, title: "Shared notes", content: "<!doctype html><p>two</p>", baseVersion: 1 });
  expect(published.isError, published.text).toBe(false);
  expect(published.structured["version"]).toBe(2);
  expect(published.structured["role"]).toBe("editor");

  // A stale publish is refused, and says how to get the newest version.
  const stale = await mcpTool(origin(page), token, "publish", { id: a.id, title: "Shared notes", content: "<p>late</p>", baseVersion: 1 });
  expect(stale.isError).toBe(true);
  expect(stale.text).toContain("Call read with");

  // Bob restores version 1 in the viewer.
  await bob.goto(`/a/${a.id}`);
  await bob.getByRole("button", { name: "More actions" }).click();
  await expect(bob.getByRole("menuitem", { name: "Editors" })).toHaveCount(0);
  await expect(bob.getByRole("menuitem", { name: "Delete" })).toHaveCount(0);
  await bob.getByRole("menuitem", { name: "Versions" }).click();
  const versions = bob.getByRole("complementary", { name: "Versions" });
  await versions.getByRole("button", { name: /v1/ }).click();
  await versions.getByRole("button", { name: "Restore" }).click();
  await presented(bob.getByRole("alertdialog"));
  await bob.getByRole("alertdialog").getByRole("button", { name: "Restore" }).click();
  await expect(bob.getByText("Restored v1 as v3")).toBeVisible();

  // Alice sees the saved editor role and manages it in Share.
  await page.goto(`/a/${a.id}`);
  await page.getByRole("button", { name: "Share", exact: true }).click();
  await presented(page.getByRole("dialog"));
  await expect(page.getByRole("dialog").getByRole("listitem").filter({ hasText: "bob" })).toContainText("Can edit");
  await expect(page.getByRole("dialog").getByRole("button", { name: "Remove bob" })).toBeVisible();
  await page.getByRole("button", { name: "Done" }).click();
  await bobContext.close();
});
