import { readFileSync } from "node:fs";
import { expect, pickVersion, presented, publish, republish, signIn, test } from "./support";

// A page that reports what the bridge answers into #out, one line per step.
const bridgePage = (title: string) => `<!doctype html><html><head><title>${title}</title></head><body><h1>${title}</h1><pre id="out"></pre><script>
const out = document.getElementById("out");
const line = (s) => { out.textContent += s + "\\n"; };
(async () => {
  try {
    const me = await pictor.whoami();
    line("whoami " + me.username);
  } catch (e) { line("whoami failed " + e.message); }
  try {
    await pictor.db.set("notes", "n1", { text: "hello" });
    line("set ok");
  } catch (e) { line("set refused " + e.message); }
  try {
    const rows = await pictor.db.list("notes");
    line("list " + rows.length);
  } catch (e) { line("list failed " + e.message); }
  line("done");
})();
</script></body></html>`;

// A page that offers two files, one after the other: the viewer asks about each.
const downloadPage = `<!doctype html><html><body><button id="csv">Offer CSV</button><button id="binary">Offer binary</button><pre id="out"></pre><script>
const out = document.getElementById("out");
const offerFiles = async (binary) => {
  const offer = async (name) => {
    try {
      const r = await pictor.download({ name, type: binary ? "application/zip" : "text/csv", data: binary ? new Uint8Array([0,255,1,254,80,75,3,4,128]) : "a,b\\n1,2\\n" });
      out.textContent += "saved " + r.name + "\\n";
    } catch (e) { out.textContent += "not saved " + e.message + "\\n"; }
  };
  await offer(binary ? "pack.zip" : "votes.csv");
  await offer(binary ? "again.zip" : "again.csv");
};
document.getElementById("csv").onclick = () => offerFiles(false);
document.getElementById("binary").onclick = () => offerFiles(true);
</script></body></html>`;

const frameOf = (page: import("@playwright/test").Page) => page.frameLocator("iframe[data-artifact-frame]");

test("the bridge answers whoami and the shared db through postMessage", async ({ page, watch }) => {
  await signIn(page, "alice");
  const a = await publish(page, { title: "Bridge", content: bridgePage("Bridge") });
  await page.goto(`/a/${a.id}`);
  const out = frameOf(page).locator("#out");
  await expect(out).toContainText("done");
  await expect(out).toContainText("whoami alice");
  await expect(out).toContainText("set ok");
  await expect(out).toContainText("list 1");

  // The same bridge hands the viewer a file to save, once the person says yes.
  const d = await publish(page, { title: "Download", content: downloadPage });
  const downloaded: string[] = [];
  page.on("download", (file) => downloaded.push(file.suggestedFilename()));
  const first = page.waitForEvent("download");
  await page.goto(`/a/${d.id}`);
  await frameOf(page).getByRole("button", { name: "Offer CSV" }).click();
  const ask = page.getByRole("alertdialog");
  await presented(ask);
  await expect(ask).toContainText("votes.csv");
  expect(downloaded).toEqual([]);
  await ask.getByRole("button", { name: "Save" }).click();
  const csv = await first;
  expect(csv.suggestedFilename()).toBe("votes.csv");
  const csvPath = await csv.path();
  expect(csvPath).not.toBeNull();
  expect(readFileSync(csvPath ?? "").toString("utf8")).toBe("a,b\n1,2\n");
  // The second file needs its own yes; here the person says no, and nothing is saved.
  await expect(ask).toContainText("again.csv");
  await presented(ask);
  await ask.getByRole("button", { name: "Cancel" }).click();
  await expect(frameOf(page).locator("#out")).toContainText("not saved the viewer refused the download");
  expect(downloaded).toEqual(["votes.csv"]);

  // The person's ordinary new-tab action keeps the trusted host, but gives the page the whole viewport.
  await page.getByRole("button", { name: "More actions" }).click();
  const opening = page.waitForEvent("popup");
  await page.getByRole("menuitem", { name: "Open in new tab" }).click();
  const full = watch(await opening);
  await expect(full).toHaveURL(new RegExp(`/a/${d.id}\\?view=full$`));
  await expect(full.getByRole("banner")).toHaveCount(0);
  const bounds = await full.locator("iframe[data-artifact-frame]").boundingBox(); // page-level
  expect(bounds).toMatchObject({ x: 0, y: 0, ...full.viewportSize() });
  const fullDownloads: string[] = [];
  full.on("download", (file) => fullDownloads.push(file.suggestedFilename()));
  const binaryFile = full.waitForEvent("download");
  await frameOf(full).getByRole("button", { name: "Offer binary" }).click();
  const fullAsk = full.getByRole("alertdialog");
  await presented(fullAsk);
  expect(fullDownloads).toEqual([]);
  await fullAsk.getByRole("button", { name: "Save" }).click();
  const binary = await binaryFile;
  expect(binary.suggestedFilename()).toBe("pack.zip");
  const binaryPath = await binary.path();
  expect(binaryPath).not.toBeNull();
  expect([...readFileSync(binaryPath ?? "")]).toEqual([0, 255, 1, 254, 80, 75, 3, 4, 128]);
  await expect(fullAsk).toContainText("again.zip");
  await presented(fullAsk);
  await fullAsk.getByRole("button", { name: "Cancel" }).click();
  await expect(frameOf(full).locator("#out")).toContainText("not saved the viewer refused the download");
  expect(fullDownloads).toEqual(["pack.zip"]);

  // Existing bridge operations also work in full-page mode; a pinned old version remains read-only.
  await full.goto(`/a/${a.id}?view=full`);
  await expect(frameOf(full).locator("#out")).toContainText("whoami alice");
  await expect(frameOf(full).locator("#out")).toContainText("set ok");
  await republish(page, a.id, { title: "Bridge", content: bridgePage("New bridge"), baseVersion: a.version });
  await full.goto(`/a/${a.id}?view=full&version=1`);
  await expect(frameOf(full).locator("h1")).toHaveText("Bridge");
  await expect(frameOf(full).locator("#out")).toContainText("set refused viewing an old version; restore it to make changes");
  await expect(frameOf(full).locator("#out")).toContainText("list 1");
  await full.close();
});

test("a page in the viewer keeps no cookie and cannot call the API as the person", async ({ page, expectConsoleErrors }) => {
  // The refused fetch is logged by the browser as a CORS failure.
  expectConsoleErrors(/CORS|Failed to load resource|net::ERR_FAILED/);
  await signIn(page, "alice");
  const probe = `<!doctype html><html><body><pre id="out"></pre><script>
const out = document.getElementById("out");
const result = { origin: self.origin };
try { result.cookie = "readable " + document.cookie; } catch (e) { result.cookie = "blocked"; }
try { localStorage.getItem("x"); result.storage = "readable"; } catch (e) { result.storage = "blocked"; }
fetch("/api/artifacts", { credentials: "include", headers: { "X-Pictor": "1" } })
  .then((r) => { result.api = "answered " + r.status; }, () => { result.api = "blocked"; })
  .finally(() => { out.textContent = JSON.stringify(result); });
</script></body></html>`;
  const a = await publish(page, { title: "Probe", content: probe });

  await page.goto(`/a/${a.id}`);
  const out = frameOf(page).locator("#out");
  await expect(out).toContainText("api");
  expect(JSON.parse((await out.textContent()) ?? "{}")).toEqual({ origin: "null", cookie: "blocked", storage: "blocked", api: "blocked" });

  // The same page opened on its own address is held in the same sandbox: the
  // signed-in session is still not reachable from it.
  await page.goto(`/raw/${a.id}`);
  await expect(page.locator("#out")).toContainText("api");
  expect(JSON.parse((await page.locator("#out").textContent()) ?? "{}")).toEqual({ origin: "null", cookie: "blocked", storage: "blocked", api: "blocked" });
});

test("an older version in the frame reads the shared data but cannot write it", async ({ page }) => {
  await signIn(page, "alice");
  const v1 = await publish(page, { title: "Notes", content: bridgePage("First") });
  await republish(page, v1.id, { title: "Notes", content: bridgePage("Second"), baseVersion: v1.version });

  await page.goto(`/a/${v1.id}`);
  await expect(frameOf(page).locator("h1")).toHaveText("Second");
  await expect(frameOf(page).locator("#out")).toContainText("set ok");

  await page.getByRole("button", { name: "More actions" }).click();
  await page.getByRole("menuitem", { name: "Versions" }).click();
  const panel = page.getByRole("complementary", { name: "Versions" });
  await pickVersion(panel, 1);

  await expect(page.getByText("Viewing v1, read only")).toBeVisible();
  const older = frameOf(page);
  await expect(older.locator("h1")).toHaveText("First");
  await expect(older.locator("#out")).toContainText("done");
  await expect(older.locator("#out")).toContainText("whoami alice");
  await expect(older.locator("#out")).toContainText("set refused viewing an old version; restore it to make changes");
  // Reading still works: the write of the newest version is there.
  await expect(older.locator("#out")).toContainText("list 1");

  await panel.getByRole("button", { name: "Back to the newest" }).click();
  await expect(frameOf(page).locator("h1")).toHaveText("Second");
});
