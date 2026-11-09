import { describe, expect, it } from "vitest";
import { ALLOWED_TYPES, MAX_BYTES, MAX_NAME, checkDownload, cleanName } from "./rules";

describe("the name a page may save a file under", () => {
  const cases: [string, string, string, string][] = [
    ["a plain name with the extension of its type", "report.csv", "text/csv", "report.csv"],
    ["path parts, either slash, are dropped", "../../etc/passwd.txt", "text/plain", "passwd.txt"],
    ["a windows path", "C:\\Users\\me\\sheet.csv", "text/csv", "sheet.csv"],
    ["dot dot alone leaves nothing, so the name is made up", "..", "application/json", "download.json"],
    ["leading dots are dropped so it is never hidden", "...hidden.json", "application/json", "hidden.json"],
    ["control characters and a null byte are dropped", "a\u0000b\u0007c\u001b\u007f.md", "text/markdown", "abc.md"],
    ["characters a file system refuses become nothing", 'a<b>:"c"|?*.txt', "text/plain", "abc.txt"],
    ["text that flips the reading direction cannot disguise the extension", "gnp.\u202Etxt", "text/plain", "gnp.txt"],
    ["an empty name", "", "text/csv", "download.csv"],
    ["only blanks", "   ", "text/csv", "download.csv"],
    ["no extension gets the one of the type", "summary", "application/pdf", "summary.pdf"],
    ["an extension that is not the type's is kept inside and the right one added", "report.exe", "text/csv", "report.exe.csv"],
    ["a dot in the name is not taken for an extension", "Q3 v1.2", "text/csv", "Q3 v1.2.csv"],
    ["every extension of a type is its own", "photo.jpeg", "image/jpeg", "photo.jpeg"],
    ["the extension is compared without regard to case", "PHOTO.JPG", "image/jpeg", "PHOTO.JPG"],
    ["a name that is not Latin", "週報.csv", "text/csv", "週報.csv"],
  ];
  it.each(cases)("%s", (_label, name, type, want) => {
    expect(cleanName(name, type)).toBe(want);
  });

  it("is cut to the limit and still ends in the extension of its type", () => {
    const got = cleanName(`${"x".repeat(500)}.csv`, "text/csv");
    expect(got).toHaveLength(MAX_NAME);
    expect(got.endsWith("x.csv")).toBe(true);
    expect(cleanName(`${"y".repeat(500)}.exe`, "text/csv").endsWith(".csv")).toBe(true);
  });

  it("is never cut in the middle of a character", () => {
    const got = cleanName(`${"😀".repeat(200)}.txt`, "text/plain");
    expect(got.length).toBeLessThanOrEqual(MAX_NAME);
    expect(got).not.toMatch(/[\uD800-\uDBFF]\.txt$/);
    expect(new TextDecoder().decode(new TextEncoder().encode(got))).toBe(got);
  });
});

describe("what a page may ask to have saved", () => {
  const ok = (over: Record<string, unknown> = {}) => checkDownload({ name: "a.csv", type: "text/csv", data: "x,y", ...over });

  it.each(ALLOWED_TYPES)("accepts %s", (type) => {
    expect(ok({ type, name: "file" })).toMatchObject({ ok: true, type });
  });

  it.each([
    ["text/html", "it would run as a page when opened"],
    ["image/svg+xml", "it can carry script"],
    ["application/x-msdownload", "a program"],
    ["application/octet-stream", "anything"],
    ["", "no type"],
    ["csv", "not a type"],
  ])("refuses %s (%s) and says what is allowed", (type) => {
    const r = ok({ type });
    expect(r).toMatchObject({ ok: false });
    expect(r.ok ? "" : r.error).toContain("text/csv");
  });

  it("reads a type without its parameters or its case", () => {
    expect(ok({ type: "Text/CSV; charset=utf-8" })).toMatchObject({ ok: true, type: "text/csv" });
  });

  it("measures text as the bytes it is saved as", () => {
    expect(ok({ data: "é" })).toMatchObject({ ok: true, size: 2 });
    expect(ok({ data: "" })).toMatchObject({ ok: true, size: 0 });
  });

  it("measures bytes as they are", () => {
    expect(ok({ type: "application/zip", data: new Uint8Array(5) })).toMatchObject({ ok: true, size: 5 });
    expect(ok({ type: "application/zip", data: new ArrayBuffer(7) })).toMatchObject({ ok: true, size: 7 });
    expect(ok({ type: "application/zip", data: new Uint8Array(new ArrayBuffer(10), 2, 3) })).toMatchObject({ ok: true, size: 3 });
  });

  it("accepts a file of exactly the limit and refuses one byte more, naming the limit", () => {
    expect(ok({ type: "application/zip", data: new Uint8Array(MAX_BYTES) })).toMatchObject({ ok: true, size: MAX_BYTES });
    const over = ok({ type: "application/zip", data: new Uint8Array(MAX_BYTES + 1) });
    expect(over).toMatchObject({ ok: false });
    expect(over.ok ? "" : over.error).toContain("10 MB");
  });

  it("refuses text over the limit by its bytes, not its characters", () => {
    expect(ok({ data: "é".repeat(MAX_BYTES / 2 + 1) })).toMatchObject({ ok: false });
    expect(ok({ data: "é".repeat(MAX_BYTES / 2) })).toMatchObject({ ok: true });
  });

  it.each([[undefined], [null], [42], [{ a: 1 }], [["x"]]])("refuses data that is not text or bytes: %j", (data) => {
    expect(ok({ data })).toMatchObject({ ok: false });
  });

  it("refuses a call that is not an object", () => {
    expect(checkDownload(undefined)).toMatchObject({ ok: false });
    expect(checkDownload("a.csv")).toMatchObject({ ok: false });
  });

  it("hands back the cleaned name", () => {
    expect(ok({ name: "../x" })).toMatchObject({ ok: true, name: "x.csv" });
  });

  it("refuses a name that is not text", () => {
    expect(ok({ name: 5 })).toMatchObject({ ok: false });
  });
});
