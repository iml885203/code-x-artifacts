import { expect, presented, signIn, test } from "./support";
import { blocksOf, browse, openEditor, originOf, publishDoc, typeAtEnd } from "./editor-support";

// The version panel of a doc being edited (docs/editor.md): versions made in a
// row by the same person are one entry, a preview only looks, and a restore
// reaches everyone who has the doc open.

const skeleton = "# Plan\n\nFirst paragraph.\n\nSecond paragraph.\n";

test("three stretches of editing are one entry, a preview is read only, and a restore shows live in a second browser", async ({ page, browser, watch }) => {
  test.setTimeout(90_000);
  await signIn(page, "alice");
  const made = await publishDoc(page, "Stretches", skeleton, ["bob"]);
  const bob = await browse(browser, watch, originOf(page), "bob");
  await openEditor(page, made.id);
  await openEditor(bob.page, made.id);

  await page.getByRole("button", { name: "More actions" }).click();
  await page.getByRole("menuitem", { name: "Versions" }).click();
  const versions = page.getByRole("complementary", { name: "Versions" });
  const newest = async () => Number((await versions.locator(".version-number").first().textContent())?.slice(1));
  await expect.poll(newest).toBe(1);

  // Each stretch is cut into a version when the room goes quiet; the next starts when that one is in the list.
  for (const words of [" One.", " Two.", " Three."]) {
    const before = await newest();
    await typeAtEnd(page, "First paragraph.", words);
    await expect.poll(newest, { timeout: 40_000 }).toBeGreaterThan(before);
  }

  // Everything is by alice within minutes: one entry, the rest behind it.
  await expect(versions.locator(".version-group")).toHaveCount(1);
  const last = await newest();
  await versions.getByRole("button", { name: `${String(last - 1)} earlier` }).click();
  await expect(versions.getByRole("button", { name: /^v1/ })).toBeVisible();

  // Previewing the first version shows its page, with nothing to edit it by.
  await versions.getByRole("button", { name: /^v1/ }).click();
  const frame = page.frameLocator("iframe[data-artifact-frame]");
  await expect(frame.getByText("First paragraph.", { exact: true })).toBeVisible();
  await expect(frame.locator("[contenteditable]")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Done" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Edit" })).toHaveCount(0);

  // Restore asks once; bob's open editor shows it without a reload, and so does alice's.
  await versions.getByRole("button", { name: "Restore" }).click();
  await presented(page.getByRole("alertdialog"));
  await page.getByRole("alertdialog").getByRole("button", { name: "Restore" }).click();
  await expect(page.getByRole("alertdialog")).toBeHidden();
  await expect.poll(() => blocksOf(bob.page), { timeout: 15_000 }).toEqual(["Plan", "First paragraph.", "Second paragraph."]);
  await expect.poll(() => blocksOf(page)).toEqual(["Plan", "First paragraph.", "Second paragraph."]);
  await expect(versions.getByText("Restored from v1")).toBeVisible();
  await bob.context.close();
});
