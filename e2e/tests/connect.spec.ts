import { createServer } from "node:http";
import { createHash, randomBytes } from "node:crypto";
import { expect, signIn, test } from "./support";

test.use({ permissions: ["clipboard-read", "clipboard-write"] });

test("Connect your agent: a token made in step 1 is filled into every client, copies, works on MCP, and is gone after a reload", async ({ page, request, baseURL }) => {
  await signIn(page, "alice");
  await page.getByRole("button", { name: "Account" }).click();
  await page.getByRole("menuitem", { name: "Connect your agent" }).click();
  await expect(page).toHaveURL(/\/connect$/);
  await expect(page.getByRole("heading", { name: "Connect your agent" })).toBeVisible();

  await page.locator("details.connect-manual > summary").click();

  // Before a token exists the snippets carry a placeholder and this server's own address.
  const manual = page.locator("details.connect-manual");
  const nav = page.getByRole("navigation", { name: "Library" });
  await expect(nav.getByRole("link", { name: "Connect your agent", exact: true })).toHaveAttribute("aria-current", "page");
  const claude = page.locator("details .code pre", { hasText: "claude mcp add" });
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
  await manual.getByRole("tab", { name: "Codex" }).click();
  await expect(manual.getByRole("tabpanel")).toContainText(`export PICTOR_TOKEN=${token}`);
  await expect(manual.getByRole("tabpanel")).toContainText("codex mcp add pictor");
  await manual.getByRole("tab", { name: "Cursor & others" }).click();
  await expect(manual.getByRole("tabpanel")).toContainText(`"Authorization": "Bearer ${token}"`);

  // A snippet's own copy button copies the snippet as written.
  await manual.getByRole("tabpanel").getByRole("button", { name: "Copy" }).click();
  await expect(manual.getByRole("tabpanel").getByRole("button", { name: "Copied" })).toBeVisible();
  expect(JSON.parse(await page.evaluate(() => navigator.clipboard.readText()))).toMatchObject({ mcpServers: { pictor: { url: `${baseURL ?? ""}/api/mcp`, headers: { Authorization: `Bearer ${token}` } } } });

  // The token is a working one: the MCP endpoint takes it, and refuses a made-up one.
  const initialize = { jsonrpc: "2.0", id: 1, method: "initialize", params: { protocolVersion: "2025-03-26", capabilities: {}, clientInfo: { name: "e2e", version: "1" } } };
  const headers = { "Content-Type": "application/json", Accept: "application/json, text/event-stream" };
  expect((await request.post("/api/mcp", { data: initialize, headers: { ...headers, Authorization: `Bearer ${token}` } })).status()).toBe(200);
  expect((await request.post("/api/mcp", { data: initialize, headers: { ...headers, Authorization: "Bearer pct_nope" } })).status()).toBe(401);

  // The secret is nowhere in the address, and a reload does not bring it back.
  expect(page.url()).not.toContain(token);
  await page.reload();
  await page.locator("details.connect-manual > summary").click();
  await expect(page.getByRole("textbox", { name: "Token name" })).toHaveValue("");
  await expect(page.locator("body")).not.toContainText(token);
  await expect(page.locator("details .code pre", { hasText: "claude mcp add" })).toContainText("<YOUR_TOKEN>");

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
  await nav.getByRole("link", { name: "Artifacts", exact: true }).click();
  await expect(page).toHaveURL(/\/$/);
  await page.getByRole("navigation", { name: "Library" }).getByRole("link", { name: "Connect your agent", exact: true }).click();
  await expect(page).toHaveURL(/\/connect$/);
  await expect(page.getByRole("navigation", { name: "Library" }).getByRole("link", { name: "Connect your agent", exact: true })).toHaveAttribute("aria-current", "page");
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
  const filter = activity.getByLabel("Access", { exact: true });
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


test("OAuth browser consent connects two clients and revokes only the chosen client", async ({ page, request, baseURL, watch }) => {
  await signIn(page, "alice");
  const managementPage = watch(await page.context().newPage());
  await managementPage.goto("/connect");
  await expect(managementPage.getByRole("button", { name: "Refresh authorizations" })).toBeEnabled();
  const resource = `${baseURL ?? ""}/api/mcp`;
  const metadata = await (await request.get("/.well-known/oauth-authorization-server")).json() as { registration_endpoint: string; authorization_endpoint: string; token_endpoint: string };
  const callbackServer = createServer((_req, res) => { res.setHeader("Content-Type", "text/html"); res.end("Connected. Return to your agent."); });
  await new Promise<void>((resolve) => callbackServer.listen(0, "127.0.0.1", resolve));
  const address = callbackServer.address();
  if (!address || typeof address === "string") throw new Error("no callback listener");
  const callback = `http://127.0.0.1:${String(address.port)}/callback`;
  try {
  async function authorize(name: string) {
    const registration = await request.post(metadata.registration_endpoint, { data: { client_name: name, redirect_uris: [callback], token_endpoint_auth_method: "none", grant_types: ["authorization_code", "refresh_token"], response_types: ["code"] } });
    expect(registration.status()).toBe(201);
    const { client_id: clientId } = await registration.json() as { client_id: string };
    const verifier = randomBytes(32).toString("base64url");
    const state = randomBytes(16).toString("hex");
    const query = new URLSearchParams({ response_type: "code", client_id: clientId, redirect_uri: callback, resource, state, code_challenge: createHash("sha256").update(verifier).digest("base64url"), code_challenge_method: "S256" });
    if (name === "Browser OAuth first client") await page.context().clearCookies();
    await page.goto(`${metadata.authorization_endpoint}?${query.toString()}`);
    if (name === "Browser OAuth first client") {
      await expect(page).toHaveURL(/\/login\?next=/);
      await page.getByLabel("Username", { exact: true }).fill("alice");
      await page.getByLabel("Password", { exact: true }).fill("pw-a");
      await page.getByRole("button", { name: "Sign in", exact: true }).click();
    }
    await expect(page.getByRole("heading", { name: `Connect ${name}?` })).toBeVisible();
    await expect(page.getByText(clientId, { exact: true })).toBeVisible();
    await page.getByRole("button", { name: "Allow access", exact: true }).click();
    await expect(page).toHaveURL((url) => url.origin + url.pathname === callback);
    const returned = new URL(page.url());
    expect(returned.searchParams.get("state")).toBe(state);
    const exchanged = await request.post(metadata.token_endpoint, { form: { grant_type: "authorization_code", client_id: clientId, redirect_uri: callback, code: returned.searchParams.get("code") ?? "", code_verifier: verifier, resource } });
    expect(exchanged.status()).toBe(200);
    return await exchanged.json() as { access_token: string };
  }
  const first = await authorize("Browser OAuth first client");
  const second = await authorize("Browser OAuth second client");
  async function initialize(token: string) {
    return (await request.post("/api/mcp", { headers: { Authorization: `Bearer ${token}`, Accept: "application/json, text/event-stream" }, data: { jsonrpc: "2.0", id: 1, method: "initialize", params: { protocolVersion: "2025-06-18", capabilities: {}, clientInfo: { name: "OAuth browser test", version: "1" } } } })).status();
  }
  expect(await initialize(first.access_token)).toBe(200);
  expect(await initialize(second.access_token)).toBe(200);
  await managementPage.getByRole("button", { name: "Refresh authorizations" }).click();
  const grants = managementPage.locator("#oauth-grants");
  const row = grants.getByRole("row").filter({ hasText: "Browser OAuth first client" });
  await row.getByRole("button", { name: "Revoke" }).click();
  const dialog = managementPage.getByRole("alertdialog");
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button", { name: "Revoke", exact: true }).click();
  await expect(row).toHaveCount(0);
  expect(await initialize(first.access_token)).toBe(401);
  expect(await initialize(second.access_token)).toBe(200);
  await managementPage.reload();
  await expect(grants.getByRole("row").filter({ hasText: "Browser OAuth second client" })).toBeVisible();
  await managementPage.setViewportSize({ width: 390, height: 844 });
  expect(await managementPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  } finally { await new Promise<void>((resolve, reject) => callbackServer.close((err) => { if (err) reject(err); else resolve(); })); }
});
