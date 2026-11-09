import { useEffect, useRef, useState, type DragEvent } from "react";
import { ApiError, deleteAsset, listAssets, uploadAsset, type Artifact, type Asset } from "../api";
import { CopyButton } from "../Copy";
import { formatSize } from "../format";
import { format, useT } from "../i18n";
import { RelativeTime } from "../RelativeTime";
import { ConfirmDialog } from "./ConfirmDialog";

interface Props {
  artifact: Artifact;
  onClose: () => void;
}

type Load = { status: "loading" } | { status: "failed" } | { status: "ready"; files: Asset[] };

/** A file name is the key: uploading one that exists replaces it. */
function withFile(files: Asset[], added: Asset): Asset[] {
  return [...files.filter((f) => f.name !== added.name), added].sort((a, b) => a.name.localeCompare(b.name));
}

/**
 * The files an artifact holds, beside the page: add by dropping or choosing,
 * copy the URL to put in the page, delete. Only the author is given this; the
 * server refuses anyone else and says why it refuses a file.
 */
export function FilesPanel({ artifact, onClose }: Props) {
  const t = useT();
  const [load, setLoad] = useState<Load>({ status: "loading" });
  const [uploading, setUploading] = useState<string | null>(null);
  const [refused, setRefused] = useState<string[]>([]);
  const [dragging, setDragging] = useState(false);
  const [removing, setRemoving] = useState<Asset | null>(null);
  const input = useRef<HTMLInputElement>(null);
  const id = artifact.id;

  useEffect(() => {
    let current = true;
    listAssets(id).then(
      (files) => {
        if (current) setLoad({ status: "ready", files });
      },
      (err: unknown) => {
        console.error("list assets", err);
        if (current) setLoad({ status: "failed" });
      },
    );
    return () => {
      current = false;
    };
  }, [id]);

  // One at a time, so a refusal names its file and the list changes in the order the person chose.
  async function upload(chosen: File[]) {
    const failures: string[] = [];
    for (const file of chosen) {
      setUploading(file.name);
      try {
        const saved = await uploadAsset(id, file);
        setLoad((l) => (l.status === "ready" ? { status: "ready", files: withFile(l.files, saved) } : l));
      } catch (err) {
        const reason = err instanceof ApiError ? err.reason : "";
        if (!(err instanceof ApiError)) console.error("upload asset", err);
        failures.push(reason === "" ? format(t("files.uploadFailedNoReason"), { name: file.name }) : format(t("files.uploadFailed"), { name: file.name, reason }));
      }
    }
    setUploading(null);
    setRefused(failures);
  }

  function dropped(e: DragEvent) {
    e.preventDefault();
    setDragging(false);
    void upload([...e.dataTransfer.files]);
  }

  async function remove() {
    if (!removing) return;
    try {
      await deleteAsset(id, removing.name);
    } catch (err) {
      console.error("delete asset", err);
      throw err;
    }
    const gone = removing.name;
    setLoad((l) => (l.status === "ready" ? { status: "ready", files: l.files.filter((f) => f.name !== gone) } : l));
    setRemoving(null);
  }

  return (
    <aside
      className="versions files"
      aria-label={t("files.title")}
      onKeyDown={(e) => {
        if (e.key === "Escape" && removing === null) onClose();
      }}
    >
      <header className="versions-head">
        <h2>{t("files.title")}</h2>
        <button type="button" className="quiet" onClick={onClose}>
          {t("files.close")}
        </button>
      </header>
      <label
        className="drop"
        data-dragging={dragging}
        onDragOver={(e) => {
          e.preventDefault();
          setDragging(true);
        }}
        onDragLeave={() => {
          setDragging(false);
        }}
        onDrop={dropped}
      >
        <input
          ref={input}
          type="file"
          multiple
          className="visually-hidden"
          onChange={(e) => {
            const chosen = [...(e.target.files ?? [])];
            e.target.value = "";
            void upload(chosen);
          }}
        />
        <span>{uploading === null ? t("files.drop") : format(t("files.uploading"), { name: uploading })}</span>
      </label>
      {refused.map((message) => (
        <p key={message} className="error" role="alert">
          {message}
        </p>
      ))}
      {load.status === "failed" && (
        <p className="error" role="alert">
          {t("files.loadFailed")}
        </p>
      )}
      {load.status === "ready" && load.files.length === 0 && <p className="comments-empty">{t("files.empty")}</p>}
      {load.status === "ready" && load.files.length > 0 && (
        <ul className="file-list">
          {load.files.map((f) => (
            <li key={f.name} className="file-row">
              <span className="file-name mono">{f.name}</span>
              <span className="facts mono">
                <span>{formatSize(f.size)}</span>
                <RelativeTime iso={f.createdAt} />
              </span>
              <span className="file-actions">
                <CopyButton className="quiet" text={() => f.url}>
                  {t("files.copyUrl")}
                </CopyButton>
                <button
                  type="button"
                  className="quiet"
                  aria-label={`${t("files.delete")} ${f.name}`}
                  onClick={() => {
                    setRemoving(f);
                  }}
                >
                  {t("files.delete")}
                </button>
              </span>
            </li>
          ))}
        </ul>
      )}
      <ConfirmDialog
        open={removing !== null}
        onOpenChange={(open) => {
          if (!open) setRemoving(null);
        }}
        title={format(t("files.confirmDelete"), { name: removing?.name ?? "" })}
        hint={t("files.confirmDeleteHint")}
        confirm={t("files.delete")}
        failure={t("files.deleteFailed")}
        danger
        onConfirm={remove}
      />
    </aside>
  );
}
