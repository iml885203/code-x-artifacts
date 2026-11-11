import { createServer } from "node:http";
import { createHash, randomBytes } from "node:crypto";
import { expect, test } from "./support";

test("sign-in errors preserve the username, valid credentials open the workspace, and a pinned theme survives reload", async ({ page, expectConsoleErrors }) => {
  expectConsoleErrors(/status of 401.*\/login/);
  await page.goto("/login");
  await expect(page.getByRole("heading", { name: "Turn ideas into artifacts. Keep creating value." })).toBeVisible();
  await page.getByLabel("Username").fill("alice");
  await page.getByLabel("Password").fill("not-the-password");
  await page.getByRole("button", { name: "Animate CodeXArtifacts mark", exact: true }).click();
  await expect(page).toHaveURL(/\/login$/);
  await expect(page.getByLabel("Username")).toHaveValue("alice");
  await expect(page.getByLabel("Password")).toHaveValue("not-the-password");
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

test("OIDC signs in explicitly, returns to MCP consent and logs out of the application", async ({ page, request }) => {
  const fixture = await (await request.get("/test/oidc-context")).json() as { appUrl: string };
  const origin = fixture.appUrl;
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(`${origin}/login?next=%2Fconnect`);
  const provider = page.getByRole("link", { name: "Continue with Test identity", exact: true });
  await expect(provider).toBeVisible();
  await expect(page.getByLabel("Password", { exact: true })).toHaveCount(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await provider.click();
  await expect(page).toHaveURL(`${origin}/connect`);
  const retainedCookies = (await page.context().cookies(origin)).filter((cookie) => cookie.name === "pictor_session");
  expect(retainedCookies).toHaveLength(1);
  const retainedCookie = retainedCookies.map((cookie) => `${cookie.name}=${cookie.value}`).join("; ");
  await page.getByRole("button", { name: "Account: Alice OIDC", exact: true }).click();
  await page.getByRole("menuitem", { name: "Sign out", exact: true }).click();
  await expect(page).toHaveURL(`${origin}/login`);
  await expect(provider).toBeVisible();
  const retained = await request.get(`${origin}/api/artifacts`, { headers: { Cookie: retainedCookie, "X-Pictor": "1" } });
  expect(retained.status()).toBe(401);
  await page.reload();
  await expect(page).toHaveURL(`${origin}/login`);
  await expect(provider).toBeVisible();

  // The same login returns to the agent's original request, not a generic home.
  const metadata = await (await request.get(`${origin}/.well-known/oauth-authorization-server`)).json() as { registration_endpoint: string; authorization_endpoint: string; token_endpoint: string };
  const callbackServer = createServer((_req, res) => { res.setHeader("Content-Type", "text/html"); res.end("Connected. Return to your agent."); });
  await new Promise<void>((resolve) => callbackServer.listen(0, "127.0.0.1", resolve));
  const address = callbackServer.address();
  if (!address || typeof address === "string") throw new Error("no callback listener");
  const callback = `http://127.0.0.1:${String(address.port)}/callback`;
  try {
    const registration = await request.post(metadata.registration_endpoint, { data: { client_name: "OIDC browser agent", redirect_uris: [callback], token_endpoint_auth_method: "none" } });
    expect(registration.status()).toBe(201);
    const { client_id: client } = await registration.json() as { client_id: string };
    const verifier = randomBytes(32).toString("base64url");
    const state = randomBytes(16).toString("hex");
    const resource = `${origin}/api/mcp`;
    const query = new URLSearchParams({ response_type: "code", client_id: client, redirect_uri: callback, resource, state, code_challenge: createHash("sha256").update(verifier).digest("base64url"), code_challenge_method: "S256" });
    await page.goto(`${metadata.authorization_endpoint}?${query.toString()}`);
    await expect(provider).toBeVisible();
    await provider.click();
    await expect(page.getByRole("heading", { name: "Connect OIDC browser agent?", exact: true })).toBeVisible();
    await page.getByRole("button", { name: "Allow access", exact: true }).click();
    await expect(page).toHaveURL((url) => url.origin + url.pathname === callback);
    const returned = new URL(page.url());
    expect(returned.searchParams.get("state")).toBe(state);
    const exchanged = await request.post(metadata.token_endpoint, { form: { grant_type: "authorization_code", client_id: client, redirect_uri: callback, code: returned.searchParams.get("code") ?? "", code_verifier: verifier, resource } });
    expect(exchanged.status()).toBe(200);
    const { access_token: token } = await exchanged.json() as { access_token: string };
    const initialized = await request.post(resource, { headers: { Authorization: `Bearer ${token}`, Accept: "application/json, text/event-stream" }, data: { jsonrpc: "2.0", id: 1, method: "initialize", params: { protocolVersion: "2025-06-18", capabilities: {}, clientInfo: { name: "OIDC browser agent", version: "1" } } } });
    expect(initialized.status()).toBe(200);
    await page.goto(`${origin}/connect`);
    await expect(page.locator("#oauth-grants").getByRole("row").filter({ hasText: "OIDC browser agent" })).toBeVisible();
  } finally {
    await new Promise<void>((resolve, reject) => callbackServer.close((err) => { if (err) reject(err); else resolve(); }));
  }
});
