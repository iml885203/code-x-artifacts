import { describe, expect, it } from "vitest";
import { boardFile, picturesToLoad, uploadPicture } from "./files";

describe("a board's pictures", () => {
  it("are loaded from the board's own artifact only, as what their name says", () => {
    expect(boardFile("abc", "shot 1.png", () => 7)).toEqual({ id: "shot 1.png", mimeType: "image/png", dataURL: "/files/abc/shot%201.png", created: 7 });
    expect(boardFile("abc", "plan.svg").mimeType).toBe("image/svg+xml");
    expect(boardFile("abc", "../x.jpg").dataURL).toBe("/files/abc/..%2Fx.jpg");
  });

  it("are the files the images show that the editor lacks and the artifact has; a file that is gone is left to the placeholder", () => {
    const els = [
      { type: "image", fileId: "a.png" },
      { type: "image", fileId: "a.png" },
      { type: "image", fileId: "b.png" },
      { type: "image", fileId: "gone.png" },
      { type: "image", fileId: "c.png", isDeleted: true },
      { type: "rectangle", fileId: "d.png" },
    ];
    expect(picturesToLoad(els, new Set(["b.png"]), new Set(["a.png", "b.png", "c.png", "d.png"]))).toEqual(["a.png"]);
  });

  it("are uploaded under a name of their own, and a refusal says the server's reason and adds nothing", async () => {
    const sent: File[] = [];
    const name = await uploadPicture(new File(["x"], "image.png", { type: "image/png" }), (f) => {
      sent.push(f);
      return Promise.resolve({ name: f.name });
    }, (r) => {
      throw new Error(`refused: ${r}`);
    });
    expect(name).toMatch(/^image-[0-9a-f]{8}\.png$/);
    expect(sent[0]?.name).toBe(name);

    const reasons: string[] = [];
    const refusal = Object.assign(new Error("refused"), { reason: "a file is at most 15 MB" });
    await expect(uploadPicture(new File(["x"], "big.png", { type: "image/png" }), () => Promise.reject(refusal), (r) => reasons.push(r))).rejects.toBe(refusal);
    expect(reasons).toEqual(["a file is at most 15 MB"]);
  });
});
