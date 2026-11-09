import { execFile } from "node:child_process";
import { createHash } from "node:crypto";
import { readFile, writeFile } from "node:fs/promises";
import { promisify } from "node:util";
import { expect, signIn, test } from "./support";

const run = promisify(execFile);

// What an agent does with a page that is already a file: hash it, announce it,
// send the bytes with curl to the one-use URL, commit. No browser is involved
// after the token exists, and the page never passes through a tool call.
test("a script publishes a 1 MB page from a file with begin, curl and commit and the bytes match", async ({ page, baseURL }, testInfo) => {
  await signIn(page, "alice");
  const made = await page.evaluate(async () => {
    const res = await fetch("/api/tokens", { method: "POST", headers: { "X-Pictor": "1", "Content-Type": "application/json" }, body: JSON.stringify({ name: "e2e publish from file" }) });
    return (await res.json()) as { token: string };
  });
  const auth = { Authorization: `Bearer ${made.token}`, "Content-Type": "application/json" };

  const file = testInfo.outputPath("report.html");
  const html = `<!doctype html><html><head><meta charset="utf-8"><title>From a file</title></head><body>${"<p>0123456789abcdef0123456789abcdef</p>\n".repeat(30000)}</body></html>`;
  await writeFile(file, html);
  const bytes = await readFile(file);
  expect(bytes.length).toBeGreaterThan(1 << 20);
  const sha256 = createHash("sha256").update(bytes).digest("hex");

  const begun = await fetch(`${baseURL}/api/publish-sessions`, {
    method: "POST",
    headers: auth,
    body: JSON.stringify({ title: "From a file", files: [{ path: "content", sha256, size: bytes.length }] }),
  });
  expect(begun.status).toBe(201);
  const session = (await begun.json()) as { session: string; missing: { url: string }[] };
  expect(session.missing).toHaveLength(1);

  const upload = await run("curl", ["-sS", "--fail-with-body", "-X", "PUT", "--data-binary", `@${file}`, session.missing[0]!.url]);
  expect(JSON.parse(upload.stdout)).toEqual({ uploaded: true });

  const committed = await fetch(`${baseURL}/api/publish-sessions/${session.session}/commit`, { method: "POST", headers: auth });
  expect(committed.status).toBe(201);
  const published = (await committed.json()) as { id: string; version: number; author: string };
  expect(published).toMatchObject({ version: 1, author: "alice" });

  const stored = await fetch(`${baseURL}/api/artifacts/${published.id}/export/html`, { headers: auth });
  expect(createHash("sha256").update(Buffer.from(await stored.arrayBuffer())).digest("hex")).toBe(sha256);
});
