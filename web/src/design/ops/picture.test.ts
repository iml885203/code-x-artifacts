import { describe, expect, it } from "vitest";
import { ApiError, type Asset } from "../../api";
import { addPicture, altOf, pictureName, sizeOf } from "./picture";

const file = (name: string, type: string, body = "x") => new File([body], name, { type });
const asset = (name: string): Asset => ({ name, contentType: "image/png", size: 1, url: `/files/abc/${name}`, createdBy: "a", createdAt: "" });

describe("a picture for a frame", () => {
  it.each([
    ["photo.PNG", "image/png", "photo-r4nd.png"],
    ["My holiday (1).jpeg", "image/jpeg", "My-holiday-1-r4nd.jpeg"],
    ["noext", "image/webp", "noext-r4nd.webp"],
    ["", "image/svg+xml", "picture-r4nd.svg"],
    ["日本語.gif", "image/gif", "日本語-r4nd.gif"],
  ])("is uploaded as %s (%s): its own name and a suffix no other upload has, so %s", (name, type, want) => {
    expect(pictureName(file(name, type), () => "r4nd")).toBe(want);
  });

  it("starts with the file's own name as its alt", () => {
    expect(altOf(file("my-cat_photo.png", "image/png"))).toBe("my cat photo");
  });

  it("reads the size an svg says it has, or its viewBox, or gives it a picture's worth", async () => {
    const svg = (attrs: string) => file("a.svg", "image/svg+xml", `<svg xmlns="http://www.w3.org/2000/svg" ${attrs}></svg>`);
    expect(await sizeOf(svg('width="40" height="20"'))).toEqual({ width: 40, height: 20 });
    expect(await sizeOf(svg('viewBox="0 0 100 50"'))).toEqual({ width: 100, height: 50 });
    expect(await sizeOf(svg(""))).toEqual({ width: 320, height: 240 });
  });

  it("says where the artifact keeps it, with the path only and not the host, and what it is", async () => {
    const uploaded: string[] = [];
    const made = await addPicture("art", file("dot.png", "image/png"), {
      size: () => Promise.resolve({ width: 3, height: 2 }),
      upload: (_id, f) => {
        uploaded.push(f.name);
        return Promise.resolve({ ...asset(f.name), url: `https://pictor.test/files/abc/${f.name}` });
      },
    });
    expect(made).toMatchObject({ alt: "dot", width: 3, height: 2 });
    expect(made.src).toMatch(/^\/files\/abc\/dot-[0-9a-f]{8}\.png$/);
    expect(uploaded).toHaveLength(1);
  });

  it("is refused with the server's own reason, and nothing is left over", async () => {
    const refusal = new ApiError("PUT: 413", 413, "a file is at most 15 MB");
    await expect(addPicture("art", file("big.png", "image/png"), { size: () => Promise.resolve({ width: 1, height: 1 }), upload: () => Promise.reject(refusal) })).rejects.toBe(refusal);
  });
});
