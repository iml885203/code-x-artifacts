import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join, resolve, sep } from "node:path";
import { inflateRawSync } from "node:zlib";

/**
 * Writes the files of a zip archive under `into`, reading the central directory. The machine that runs the specs
 * need not have an `unzip`: the CI image has none. Stored and deflated entries only, which is what the server writes.
 */
export function extractZip(archive: string, into: string): void {
  const zip = readFileSync(archive);
  let end = zip.length - 22;
  while (end >= 0 && zip.readUInt32LE(end) !== 0x06054b50) end--;
  if (end < 0) throw new Error(`${archive} is not a zip archive`);
  const count = zip.readUInt16LE(end + 10);
  let at = zip.readUInt32LE(end + 16);
  const root = resolve(into);
  for (let i = 0; i < count; i++) {
    if (zip.readUInt32LE(at) !== 0x02014b50) throw new Error("the zip's central directory is damaged");
    const method = zip.readUInt16LE(at + 10);
    const size = zip.readUInt32LE(at + 20);
    const nameLength = zip.readUInt16LE(at + 28);
    const extraLength = zip.readUInt16LE(at + 30);
    const commentLength = zip.readUInt16LE(at + 32);
    const local = zip.readUInt32LE(at + 42);
    const name = zip.toString("utf8", at + 46, at + 46 + nameLength);
    at += 46 + nameLength + extraLength + commentLength;
    const target = resolve(join(root, name));
    if (target !== root && !target.startsWith(root + sep)) throw new Error(`${name} leaves the folder it is extracted to`);
    if (name.endsWith("/")) {
      mkdirSync(target, { recursive: true });
      continue;
    }
    const start = local + 30 + zip.readUInt16LE(local + 26) + zip.readUInt16LE(local + 28);
    const data = zip.subarray(start, start + size);
    if (method !== 0 && method !== 8) throw new Error(`${name} uses compression method ${method}`);
    mkdirSync(dirname(target), { recursive: true });
    writeFileSync(target, method === 0 ? data : inflateRawSync(data));
  }
}
