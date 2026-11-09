import { useEffect, useMemo, useState } from "react";
import { ApiError, listVersions, restoreVersion, type Artifact, type VersionRow } from "../api";
import { RelativeTime } from "../RelativeTime";
import { format, useT } from "../i18n";
import { useToast } from "../Toast";
import { ConfirmDialog } from "./ConfirmDialog";
import { groupVersions } from "./versionGroups";

interface Props {
  artifact: Artifact;
  /** The version shown in the frame, or null for the newest. */
  shown: number | null;
  onShow: (version: number | null) => void;
  onClose: () => void;
  /** The restore was saved: this is the artifact now, its newest version the copy. */
  onRestored: (a: Artifact, from: number) => void;
  /** The person asked for their own copy of this version. */
  onCopy: (version: number) => void;
}

type Load = { status: "loading" } | { status: "failed" } | { status: "ready"; rows: VersionRow[] };

/**
 * The history of an artifact, newest first, beside the page. Picking a row shows
 * that version in the frame, read only; restoring it saves a new version on top.
 */
export function VersionsPanel({ artifact, shown, onShow, onClose, onRestored, onCopy }: Props) {
  const t = useT();
  const toast = useToast();
  const [load, setLoad] = useState<Load>({ status: "loading" });
  const [confirming, setConfirming] = useState(false);
  // The groups the person opened, by the newest version in each.
  const [opened, setOpened] = useState<ReadonlySet<number>>(new Set());
  const rows = load.status === "ready" ? load.rows : null;
  const groups = useMemo(() => (rows ? groupVersions(rows) : []), [rows]);

  useEffect(() => {
    let current = true;
    listVersions(artifact.id).then(
      (rows) => {
        if (current) setLoad({ status: "ready", rows });
      },
      (err: unknown) => {
        console.error("list versions", err);
        if (current) setLoad({ status: "failed" });
      },
    );
    return () => {
      current = false;
    };
    // A restore adds a version, so the list is read again when the artifact moves to a new one.
  }, [artifact.id, artifact.version]);

  // What the viewer sees as the newest is the one it was opened on: a restore
  // is refused (409) if someone saved another since, and the person is told.
  // The room can make a version while the editor is open, so the list may know a newer one than the artifact this viewer holds.
  const newest = Math.max(artifact.version, load.status === "ready" ? (load.rows[0]?.version ?? 0) : 0);
  const picked = shown !== null && shown !== newest ? shown : null;

  async function restore() {
    if (picked === null) return;
    try {
      const saved = await restoreVersion(artifact.id, picked, newest);
      setConfirming(false);
      onRestored(saved, picked);
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        setConfirming(false);
        toast({ message: t("versions.restoreConflict"), error: true });
        return;
      }
      console.error("restore version", err);
      throw err;
    }
  }

  return (
    <aside
      className="versions"
      aria-label={t("versions.title")}
      onKeyDown={(e) => {
        if (e.key === "Escape" && !confirming) onClose();
      }}
    >
      <header className="versions-head">
        <h2>{t("versions.title")}</h2>
        <button type="button" className="quiet" onClick={onClose}>
          {t("versions.close")}
        </button>
      </header>
      {load.status === "failed" && (
        <p className="error" role="alert">
          {t("versions.loadFailed")}
        </p>
      )}
      {load.status === "ready" && (
        <ol className="version-list">
          {groups.map((group) => {
            const open = opened.has(group.newest.version);
            const earlier = group.rows.slice(1);
            return (
              <li key={group.newest.version} className="version-group">
                <VersionRow row={group.newest} newest={newest} picked={picked} onShow={onShow} onCopy={onCopy} />
                {earlier.length > 0 && (
                  <>
                    <button
                      type="button"
                      className="quiet version-more"
                      aria-expanded={open}
                      onClick={() => {
                        setOpened((was) => {
                          const next = new Set(was);
                          if (!next.delete(group.newest.version)) next.add(group.newest.version);
                          return next;
                        });
                      }}
                    >
                      {format(t("versions.earlier"), { count: String(earlier.length) })}
                    </button>
                    {open && (
                      <ol className="version-earlier">
                        {earlier.map((row) => (
                          <li key={row.version}>
                            <VersionRow row={row} newest={newest} picked={picked} onShow={onShow} onCopy={onCopy} />
                          </li>
                        ))}
                      </ol>
                    )}
                  </>
                )}
              </li>
            );
          })}
        </ol>
      )}
      {picked !== null && (
        <footer className="versions-foot">
          <p>{format(t("versions.viewing"), { number: String(picked) })}</p>
          <div className="dialog-actions">
            <button
              type="button"
              onClick={() => {
                onShow(null);
              }}
            >
              {t("versions.backToNewest")}
            </button>
            <button
              type="button"
              className="primary"
              onClick={() => {
                setConfirming(true);
              }}
            >
              {t("versions.restore")}
            </button>
          </div>
        </footer>
      )}
      <ConfirmDialog
        open={confirming && picked !== null}
        onOpenChange={setConfirming}
        title={format(t("versions.confirmRestore"), { number: String(picked ?? "") })}
        hint={t("versions.confirmRestoreHint")}
        confirm={t("versions.restore")}
        failure={t("versions.restoreFailed")}
        onConfirm={restore}
      />
    </aside>
  );
}

interface RowProps {
  row: VersionRow;
  /** The version the viewer takes as the newest. */
  newest: number;
  picked: number | null;
  onShow: (version: number | null) => void;
  onCopy: (version: number) => void;
}

/** One version: who made it (the people, and each agent as the person's), when, and what it is a restore of. */
function VersionRow({ row, newest, picked, onShow, onCopy }: RowProps) {
  const t = useT();
  const people = row.authors?.length ? row.authors : [row.author];
  const agents = (row.agents ?? []).map((a) => format(t("versions.agent"), { name: a.by, client: a.client }));
  return (
    <div className="version-item">
      <button
        type="button"
        className="version-row"
        aria-pressed={row.version === (picked ?? newest)}
        onClick={() => {
          onShow(row.version === newest ? null : row.version);
        }}
      >
        <span className="mono version-number">v{row.version}</span>
        <span className="version-title">{row.title}</span>
        <span className="facts mono">
          <span>{[...people, ...agents].join(", ")}</span>
          <RelativeTime iso={row.createdAt} />
        </span>
        {row.version === newest && <span className="version-note">{t("versions.current")}</span>}
        {row.restoredFrom ? <span className="version-note">{format(t("versions.restoredFrom"), { from: String(row.restoredFrom) })}</span> : null}
      </button>
      <button
        type="button"
        className="quiet version-copy"
        onClick={() => {
          onCopy(row.version);
        }}
      >
        {t("versions.copy")}
      </button>
    </div>
  );
}
