import { expect, test } from "./support";

test("sign-in errors preserve the username, valid credentials open the workspace, and a pinned theme survives reload", async ({ page, expectConsoleErrors }) => {
  expectConsoleErrors(/status of 401.*\/login/);
  await page.goto("/login");
  await expect(page.getByRole("heading", { name: "Turn ideas into artifacts. Keep creating value." })).toBeVisible();
  await page.getByLabel("Username").fill("alice");
  await page.getByLabel("Password").fill("not-the-password");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByRole("alert")).toContainText("Wrong username or password");
  await expect(page.getByLabel("Username")).toHaveValue("alice");
  await page.getByLabel("Password").fill("pw-a");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page).toHaveURL(/\/$/);
  await page.getByRole("button", { name: "Account" }).click();
  await page.getByRole("menuitemradio", { name: "Dark" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
});

test("the sign-in page says what it is in the tab and in its description", async ({ page }) => {
  await page.goto("/login");
  await expect(page).toHaveTitle("Sign in · CodeXArtifacts");
  await expect(page.locator('meta[name="description"]')).toHaveAttribute("content", /Sign in to CodeXArtifacts/);
});
