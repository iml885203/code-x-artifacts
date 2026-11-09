// The viewer's title: for the author and the editors it is a button that turns into a field. Renaming
// changes the title only, never the document's own heading.
import { useEffect, useRef, useState } from "react";
import { type Artifact } from "../api";
import { useT } from "../i18n";
import { useToast } from "../Toast";
import { renameArtifact } from "./api";
import "./create.css";

/** The most characters a title takes (service.MaxTitleRunes). */
export const maxTitleLength = 200;

interface Props {
  artifact: Pick<Artifact, "id" | "title" | "canEdit">;
  /** The artifact as the server has it after a rename. */
  onRenamed: (saved: Artifact) => void;
  /** Opens as the field, for a caller that starts renaming from somewhere else (the viewer's title menu). */
  begun?: boolean;
  /** The field closed, kept or not. */
  onFinished?: () => void;
}

export function RenameTitle({ artifact, onRenamed, begun = false, onFinished }: Props) {
  const t = useT();
  const toast = useToast();
  const [editing, setEditing] = useState(begun);
  const [text, setText] = useState(artifact.title);
  const input = useRef<HTMLInputElement>(null);
  const done = useRef(false);

  useEffect(() => {
    if (editing) input.current?.select();
  }, [editing]);

  if (!artifact.canEdit) return <h1>{artifact.title}</h1>;

  function begin() {
    done.current = false;
    setText(artifact.title);
    setEditing(true);
  }

  function finish(keep: boolean) {
    if (done.current) return;
    done.current = true;
    setEditing(false);
    onFinished?.();
    const title = text.trim();
    if (!keep || title === "" || title === artifact.title || /[\r\n]/.test(title)) return;
    renameArtifact(artifact.id, title).then(onRenamed, (err: unknown) => {
      console.error("rename the artifact", err);
      toast({ message: t("create.renameFailed"), error: true });
    });
  }

  return (
    <h1 className="rename">
      {editing ? (
        <input
          ref={input}
          className="rename-field"
          value={text}
          maxLength={maxTitleLength}
          aria-label={t("create.titleField")}
          autoFocus
          onChange={(e) => {
            setText(e.target.value);
          }}
          onKeyDown={(e) => {
            if (e.key === "Enter") finish(true);
            if (e.key === "Escape") finish(false);
          }}
          onBlur={() => {
            finish(true);
          }}
        />
      ) : (
        <button key={artifact.title} type="button" className="rename-button fade-in" title={t("create.rename")} onClick={begin}>
          {artifact.title}
        </button>
      )}
    </h1>
  );
}
