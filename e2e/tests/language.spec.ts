import { expect, test } from "./support";

test("the language is picked in the sign-in footer and in the avatar menu, and sticks", async ({ page }) => {
  await page.goto("/login");
  await expect(page.locator("html")).toHaveAttribute("lang", "en");
  await expect(page.getByRole("button", { name: "Sign in" })).toBeVisible();
  await page.getByRole("button", { name: "Interface language: English" }).click();
  await page.getByRole("menuitemradio", { name: "中文" }).click();
  await expect(page.locator("html")).toHaveAttribute("lang", "zh-Hant");
  await expect(page.getByRole("button", { name: "登入" })).toBeVisible();

  await page.getByLabel("帳號").fill("alice");
  await page.getByLabel("密碼").fill("pw-a");
  await page.getByRole("button", { name: "登入" }).click();
  await expect(page.getByRole("heading", { name: "Artifacts" })).toHaveCount(0);
  await expect(page.locator("html")).toHaveAttribute("lang", "zh-Hant");

  // The avatar menu changes it without leaving the page.
  await page.getByRole("button", { name: "帳號" }).click();
  await page.getByRole("menuitemradio", { name: "English" }).click();
  await expect(page.locator("html")).toHaveAttribute("lang", "en");
  await expect(page.getByRole("button", { name: "Account" })).toBeVisible();
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("lang", "en");
});
