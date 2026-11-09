import { expect, signIn, test } from "./support";

test.use({ permissions: ["clipboard-read", "clipboard-write"] });

test("Connect your agent: a token made in step 1 is filled into every client, copies, works on MCP, and is gone after a reload", async ({ page, request, baseURL }) => {
  await signIn(page, "alice");
  await page.getByRole("button", { name: "Account" }).click();
  await page.getByRole("menuitem", { name: "Connect your agent" }).click();
  await expect(page).toHaveURL(/\/connect$/);
  await expect(page.getByRole("heading", { name: "Connect your agent" })).toBeVisible();

  // Before a token exists the snippets carry a placeholder and this server's own address.
  const claude = page.locator(".code pre", { hasText: "claude mcp add" });
  await expect(claude).toContainText(`${baseURL ?? ""}/api/mcp`);
  await expect(claude).toContainText("Bearer <YOUR_TOKEN>");

  await page.getByRole("textbox", { name: "Token name" }).fill("e2e laptop");
  await page.getByRole("button", { name: "Create token" }).click();
  const field = page.getByRole("textbox", { name: "Your token" });
  await expect(field).toHaveValue(/^pct_[0-9a-f]+$/);
  const token = await field.inputValue();
  await expect(page.getByText("Shown only once. It is already filled into step 2.")).toBeVisible();
  await expect(page.getByText("<YOUR_TOKEN>")).toHaveCount(0);

  // The copy button says it copied, and what it copied is the token.
  const copy = page.locator("form.row").getByRole("button", { name: "Copy" });
  await copy.click();
  await expect(page.locator("form.row").getByRole("button", { name: "Copied" })).toBeVisible();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(token);

  // Every client's snippet has the token in it.
  await expect(claude).toContainText(`Authorization: Bearer ${token}`);
  await page.getByRole("tab", { name: "Codex" }).click();
  await expect(page.getByRole("tabpanel")).toContainText(`export PICTOR_TOKEN=${token}`);
  await expect(page.getByRole("tabpanel")).toContainText("codex mcp add pictor");
  await page.getByRole("tab", { name: "Cursor & others" }).click();
  await expect(page.getByRole("tabpanel")).toContainText(`"Authorization": "Bearer ${token}"`);

  // A snippet's own copy button copies the snippet as written.
  await page.getByRole("tabpanel").getByRole("button", { name: "Copy" }).click();
  await expect(page.getByRole("tabpanel").getByRole("button", { name: "Copied" })).toBeVisible();
  expect(JSON.parse(await page.evaluate(() => navigator.clipboard.readText()))).toMatchObject({ mcpServers: { pictor: { url: `${baseURL ?? ""}/api/mcp`, headers: { Authorization: `Bearer ${token}` } } } });

  // The token is a working one: the MCP endpoint takes it, and refuses a made-up one.
  const initialize = { jsonrpc: "2.0", id: 1, method: "initialize", params: { protocolVersion: "2025-03-26", capabilities: {}, clientInfo: { name: "e2e", version: "1" } } };
  const headers = { "Content-Type": "application/json", Accept: "application/json, text/event-stream" };
  expect((await request.post("/api/mcp", { data: initialize, headers: { ...headers, Authorization: `Bearer ${token}` } })).status()).toBe(200);
  expect((await request.post("/api/mcp", { data: initialize, headers: { ...headers, Authorization: "Bearer pct_nope" } })).status()).toBe(401);

  // The secret is nowhere in the address, and a reload does not bring it back.
  expect(page.url()).not.toContain(token);
  await page.reload();
  await expect(page.getByRole("textbox", { name: "Token name" })).toHaveValue("");
  await expect(page.locator("body")).not.toContainText(token);
  await expect(page.locator(".code pre", { hasText: "claude mcp add" })).toContainText("<YOUR_TOKEN>");

  // The token is listed, by name, in Your tokens: the link from step 1 goes there, and the list has no secret.
  await page.getByRole("link", { name: "Manage tokens" }).click();
  await expect(page).toHaveURL(/\/connect#tokens$/);
  const row = page.getByRole("row", { name: /e2e laptop/ });
  await expect(row).toBeVisible();
  await expect(page.locator("body")).not.toContainText(token);

  // Revoking asks first, and after it the token no longer works on MCP.
  await row.getByRole("button", { name: "Revoke" }).click();
  await expect(page.getByRole("alertdialog")).toContainText("Revoke e2e laptop?");
  await page.getByRole("alertdialog").getByRole("button", { name: "Revoke" }).click();
  await expect(page.getByRole("row", { name: /e2e laptop/ })).toHaveCount(0);
  expect((await request.post("/api/mcp", { data: initialize, headers: { ...headers, Authorization: `Bearer ${token}` } })).status()).toBe(401);

  // The list is the server's: the token is still gone after a reload.
  await page.reload();
  await expect(page.getByRole("heading", { name: "Your tokens" })).toBeVisible();
  await expect(page.getByRole("row", { name: /e2e laptop/ })).toHaveCount(0);
});

test("Your tokens: a token made in step 1 appears at once, and the old /tokens address and the account menu lead to Connect", async ({ page, request }) => {
  await signIn(page, "alice");
  await page.goto("/tokens");
  await expect(page).toHaveURL(/\/connect#tokens$/);
  await expect(page.getByRole("heading", { name: "Your tokens" })).toBeVisible();
  await page.getByRole("textbox", { name: "Token name" }).fill("ci runner");
  await page.getByRole("button", { name: "Create token" }).click();
  await expect(page.getByRole("row", { name: /ci runner/ })).toBeVisible();
  const token = await page.getByRole("textbox", { name: "Your token" }).inputValue();
  const connected = await request.post("/api/mcp", {
    headers: { Authorization: `Bearer ${token}`, Accept: "application/json, text/event-stream" },
    data: { jsonrpc: "2.0", id: 1, method: "initialize", params: { protocolVersion: "2025-06-18", capabilities: {}, clientInfo: { name: "Connect narrow layout client", version: "1" } } },
  });
  expect(connected.status()).toBe(200);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.reload();
  const activity = page.locator("#recent-activity");
  const row = activity.getByRole("listitem").filter({ hasText: "Connect narrow layout client" });
  await expect(row).toBeVisible();
  await row.scrollIntoViewIfNeeded();
  const filter = activity.getByLabel("Token", { exact: true });
  await expect(filter).toBeVisible();
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  for (const item of [row, filter, row.locator("time")]) {
    const box = await item.boundingBox(); // page-level
    expect(box).not.toBeNull();
    expect(box!.x).toBeGreaterThanOrEqual(0);
    expect(box!.x + box!.width).toBeLessThanOrEqual(390);
  }
  await filter.selectOption({ label: "ci runner" });
  await expect(row).toBeVisible();
  await page.getByRole("button", { name: "Account" }).click();
  await expect(page.getByRole("menuitem", { name: "Tokens" })).toHaveCount(0);
});
