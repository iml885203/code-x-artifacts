// Pictures: the Image button (a file picker), the tools of the selected picture
// (its alt text, its size, Delete), pasting and dropping, and how a picture is drawn.
import { useRef, useState } from "react";
import type { EditorState } from "prosemirror-state";
import { useT, type Key } from "../../i18n";
import { ImageIcon } from "../../icons/image";
import type { Feature, ToolContext } from "../feature";
import { deleteImage, ImageView, imageInfo, imageInput, imageSizes, setImage, type ImageInfo, type ImageSize } from "../image";

function ImageButton({ ctx }: { ctx: ToolContext }) {
  const t = useT();
  const picker = useRef<HTMLInputElement>(null);
  return (
    <>
      <button
        type="button"
        className="quiet icon-button"
        aria-label={t("editor.image")}
        title={t("editor.image")}
        onClick={() => {
          picker.current?.click();
        }}
      >
        <ImageIcon />
      </button>
      <input
        ref={picker}
        type="file"
        accept="image/*"
        multiple
        hidden
        aria-label={t("editor.image")}
        data-testid="image-picker"
        onChange={(e) => {
          const files = [...(e.target.files ?? [])];
          e.target.value = "";
          if (files.length > 0) ctx.addImages(files);
        }}
      />
    </>
  );
}

const sizeLabels: Record<ImageSize, Key> = { small: "editor.sizeSmall", medium: "editor.sizeMedium", large: "editor.sizeLarge", xlarge: "editor.sizeXlarge" };

/** The tools of the selected picture: its alt text, its size and Delete. The alt text is kept when the field is left or Enter is pressed. */
function ImageTools({ image, ctx }: { image: ImageInfo; ctx: ToolContext }) {
  const t = useT();
  const [alt, setAlt] = useState(image.alt);
  const keep = () => {
    if (alt !== image.alt) ctx.run(setImage({ alt }));
  };
  return (
    <div className="ed-toolbar ed-image-tools" role="toolbar" aria-label={t("editor.imageTools")}>
      <label className="ed-alt">
        {t("editor.imageAlt")}
        <input
          type="text"
          value={alt}
          onChange={(e) => {
            setAlt(e.target.value);
          }}
          onBlur={keep}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              keep();
            }
          }}
        />
      </label>
      <div className="ed-group" role="group" aria-label={t("editor.imageSize")}>
        {(Object.keys(imageSizes) as ImageSize[]).map((size) => (
          <button
            key={size}
            type="button"
            className="quiet"
            aria-pressed={image.size === size}
            onClick={() => {
              ctx.run(setImage({ size: image.size === size ? null : size }));
            }}
          >
            {t(sizeLabels[size])}
          </button>
        ))}
      </div>
      <button
        type="button"
        className="quiet"
        onClick={() => {
          ctx.run(deleteImage);
        }}
      >
        {t("editor.imageDelete")}
      </button>
    </div>
  );
}

export const image: Feature = {
  id: "image",
  tools: [{ id: "image", group: "insert", render: (_state, ctx) => <ImageButton ctx={ctx} /> }],
  rows: [
    {
      id: "image-tools",
      render: (state: EditorState, ctx) => {
        const selected = imageInfo(state);
        return selected === null ? null : <ImageTools key={selected.pos} image={selected} ctx={ctx} />;
      },
    },
  ],
  plugins: (hooks) => [imageInput(hooks.onImages)],
  nodeViews: (hooks) => ({ image: (node) => new ImageView(node, hooks.say) }),
};
