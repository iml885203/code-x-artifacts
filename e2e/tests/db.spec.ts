import { expect, publish, signIn, test } from "./support";

// A sign-up slot. Each person's page reads the slot with its version, then
// claims it only if nobody has in the meantime: the claim carries the version
// it read as ifVersion.
const slotPage = `<!doctype html><html><head><title>Slot</title></head><body><h1>Slot</h1><button id="claim" disabled>Claim</button><pre id="out"></pre><script>
const out = document.getElementById("out");
const say = (s) => { out.textContent = s; };
(async () => {
  const me = await pictor.whoami();
  const [slot] = await pictor.db.query("slots", { id: "mon" });
  const version = slot.version;
  const claim = document.getElementById("claim");
  claim.disabled = false;
  say("ready v" + version);
  claim.addEventListener("click", async () => {
    try {
      const done = await pictor.db.update("slots", "mon", { who: me.username }, { ifVersion: version });
      say("won v" + done.version);
    } catch (e) { say("refused " + e.message); }
  });
})().catch((e) => say("failed " + e.message));
</script></body></html>`;

const frameOf = (page: import("@playwright/test").Page) => page.frameLocator("iframe[data-artifact-frame]");

test("two browsers claim the same slot at once and exactly one wins", async ({ page, browser, watch, expectConsoleErrors }) => {
  // The refused claim is a 409 the browser logs.
  expectConsoleErrors(/status of 409|Failed to load resource/);
  await signIn(page, "alice");
  const a = await publish(page, { title: "Slot", content: slotPage, visibility: "organisation" });
  const seeded = await page.evaluate(async (id) => {
    const res = await fetch(`/api/artifacts/${id}/db/slots/mon`, { method: "PUT", headers: { "X-Pictor": "1", "Content-Type": "application/json" }, body: JSON.stringify({ who: null }) });
    return res.status;
  }, a.id);
  expect(seeded).toBe(200);

  const bobContext = await browser.newContext({ baseURL: new URL(page.url()).origin });
  const bob = watch(await bobContext.newPage());
  await signIn(bob, "bob");

  await page.goto(`/a/${a.id}`);
  await bob.goto(`/a/${a.id}`);
  // Both have read version 1 before either writes.
  await expect(frameOf(page).locator("#out")).toHaveText("ready v1");
  await expect(frameOf(bob).locator("#out")).toHaveText("ready v1");

  await Promise.all([frameOf(page).locator("#claim").click(), frameOf(bob).locator("#claim").click()]);

  const outcomes = async () => [await frameOf(page).locator("#out").textContent(), await frameOf(bob).locator("#out").textContent()];
  await expect.poll(async () => (await outcomes()).every((o) => /^(won|refused)/.test(o ?? ""))).toBe(true);
  const [alice, bobs] = (await outcomes()) as [string, string];
  const winners = [alice, bobs].filter((o) => o.startsWith("won v2"));
  const losers = [alice, bobs].filter((o) => o.startsWith("refused"));
  expect(winners, `${alice} / ${bobs}`).toHaveLength(1);
  expect(losers, `${alice} / ${bobs}`).toHaveLength(1);
  expect(losers[0]).toContain("slots/mon was written against version 1 but now is at version 2");

  // The slot holds the winner's name and nothing of the loser's.
  const winner = alice.startsWith("won") ? "alice" : "bob";
  const res = await page.evaluate(async (id) => (await fetch(`/api/artifacts/${id}/db/slots/mon`, { headers: { "X-Pictor": "1" } })).json(), a.id);
  expect(res).toMatchObject({ version: 2, data: { who: winner } });
  await bobContext.close();
});
