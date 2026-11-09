// Adding pictures for the image feature (features/image.tsx): uploading them to
// the artifact, what the person is told while that happens and why one was refused.
import { useState, type RefObject } from "react";
import type { EditorView } from "prosemirror-view";
import { uploadAsset } from "../api";
import { format, useT } from "../i18n";
import { addPictures, insertImage } from "./image";
import { uploadName } from "../uploadName";

/** `addImages` for the picture button, drop and paste; the `words` a picture says when it cannot be shown; and the `banners` to draw. */
export function useImageAdding(view: RefObject<EditorView | null>, artifactId: string) {
  const t = useT();
  // How many pictures are on their way, and why the last one was refused.
  const [adding, setAdding] = useState(0);
  const [refusal, setRefusal] = useState<string | null>(null);
  const addImages = (files: File[]) => {
    setAdding((n) => n + files.length);
    void addPictures(files, {
      upload: async (file) => {
        try {
          const made = await uploadAsset(artifactId, new File([file], uploadName(file), { type: file.type }));
          return new URL(made.url, window.location.origin).pathname;
        } finally {
          setAdding((n) => n - 1);
        }
      },
      insert: (src) => {
        const v = view.current;
        if (v) insertImage(src)(v.state, v.dispatch);
      },
      refused: setRefusal,
    });
  };
  const words = { deleted: t("editor.imageDeleted"), unavailable: t("editor.imageUnavailable") };
  const banners = (
    <>
      {adding > 0 && <p className="ed-banner" role="status">{t("editor.imageUploading")}</p>}
      {refusal !== null && (
        <p className="ed-banner ed-refused" role="alert">
          {refusal === "" ? t("editor.imageRefusedNoReason") : format(t("editor.imageRefused"), { reason: refusal })}
        </p>
      )}
    </>
  );
  return { addImages, words, banners };
}
