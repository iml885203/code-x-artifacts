import { useRef, useState } from "react";
import { ApiError } from "../../api";
import { format, useT } from "../../i18n";
import type { Target } from "../controller";
import type { DesignEdits } from "../edits";
import { Field, Section, Segmented } from "../properties/controls";
import { fill, setAttribute, type Fit } from "./nodes";
import { addPicture, type Seams } from "./picture";

interface Props {
  artifactId: string;
  edits: DesignEdits;
  targets: Target[];
  seams?: Seams;
}

const value = (pairs: [string, string][], name: string) => pairs.find(([k]) => k === name)?.[1] ?? "";

/**
 * The picture of the selected elements: an <img> says where it is from and what it
 * shows (its alt) and how it fits its box; any other element can be filled with a
 * picture, which fits the same way. The file goes to the artifact like any upload,
 * and what the server says when it refuses one is shown as it said it.
 */
export function ImageSection({ artifactId, edits, targets, seams }: Props) {
  const t = useT();
  const chooser = useRef<HTMLInputElement>(null);
  const [problem, setProblem] = useState("");
  const last = targets[targets.length - 1];
  if (!last) return null;
  const { tag, attrs, inline } = last.report;
  const isImage = tag === "img";
  const filled = value(inline, "background-image") !== "" && value(inline, "background-image") !== "none";
  const fitNow: Fit | null = isImage
    ? ({ cover: "cover", contain: "contain", none: "original" } as Record<string, Fit>)[value(inline, "object-fit")] ?? null
    : ({ cover: "cover", contain: "contain", auto: "original" } as Record<string, Fit>)[value(inline, "background-size")] ?? null;

  const choose = async (file: File) => {
    setProblem("");
    try {
      const picture = await addPicture(artifactId, file, seams);
      edits.transact((map) => {
        for (const x of targets) {
          if (x.report.tag === "img") {
            setAttribute(map, x.file, x.pid, "src", picture.src);
            if (value(x.report.attrs, "alt") === "") setAttribute(map, x.file, x.pid, "alt", picture.alt);
          } else fill(map, x.file, x.pid, picture.src);
        }
      });
    } catch (err) {
      if (err instanceof ApiError) setProblem(err.reason === "" ? format(t("ops.refusedNoReason"), { name: file.name }) : format(t("ops.refused"), { name: file.name, reason: err.reason }));
      else {
        console.error("add a picture", err);
        setProblem(format(t("ops.unreadable"), { name: file.name }));
      }
    }
  };
  const fit = (how: Fit) => {
    const style = isImage ? { "object-fit": { cover: "cover", contain: "contain", original: "none" }[how] } : { "background-size": { cover: "cover", contain: "contain", original: "auto" }[how] };
    edits.apply(targets.map((x) => ({ file: x.file, pid: x.pid, style })));
  };
  const removeFill = () => {
    edits.apply(targets.map((x) => ({ file: x.file, pid: x.pid, style: { "background-image": "", "background-size": "", "background-position": "", "background-repeat": "" } })));
  };
  const hasPicture = isImage || filled;
  return (
    <Section title={t("ops.imageSection")}>
      <div className="prop-row">
        <button
          type="button"
          className="prop-toggle"
          onClick={() => {
            chooser.current?.click();
          }}
        >
          {t(hasPicture ? "ops.replace" : "ops.fillWith")}
        </button>
        {!isImage && filled && (
          <button type="button" className="prop-toggle" onClick={removeFill}>
            {t("ops.removeFill")}
          </button>
        )}
        <input
          ref={chooser}
          type="file"
          accept="image/png,image/jpeg,image/gif,image/webp,image/svg+xml"
          hidden
          aria-label={t("ops.fillWith")}
          onChange={(e) => {
            const file = e.target.files?.[0];
            e.target.value = "";
            if (file) void choose(file);
          }}
        />
      </div>
      {problem !== "" && (
        <p className="prop-refused" role="alert">
          {problem}
        </p>
      )}
      {hasPicture && (
        <Segmented
          label={t("ops.fit")}
          value={fitNow}
          choices={[
            { value: "cover", label: t("ops.cover") },
            { value: "contain", label: t("ops.contain") },
            { value: "original", label: t("ops.original") },
          ]}
          onChoose={fit}
        />
      )}
      {isImage && (
        <Field
          label={t("ops.alt")}
          value={value(attrs, "alt")}
          wide
          placeholder={t("ops.altHint")}
          onCommit={(text) => {
            edits.transact((map) => {
              for (const x of targets) if (x.report.tag === "img") setAttribute(map, x.file, x.pid, "alt", text.trim());
            });
            return true;
          }}
        />
      )}
    </Section>
  );
}
