// The list of a doc's tabs beside the page: choose one, and (for people who may edit) add, rename, delete and move.
import { useEffect, useRef, useState, type ReactNode } from "react";
import { Plus } from "../icons/chrome";
import { Pencil, TabDown, TabUp, Trash } from "../icons/tabs";
import { format, useT } from "../i18n";
import { ConfirmDialog } from "../viewer/ConfirmDialog";
import { cleanTitle, maxTitleLength, type TabInfo } from "./tabs";

interface Props {
  tabs: TabInfo[];
  /** The tab being shown. */
  shown: string;
  /** The person may change the tabs; the others only choose which one to read. */
  editable: boolean;
  onShow: (id: string) => void;
  onAdd: () => void;
  onRename: (id: string, title: string) => void;
  onMove: (id: string, by: -1 | 1) => void;
  onDelete: (id: string) => void;
}

export function TabList({ tabs, shown, editable, onShow, onAdd, onRename, onMove, onDelete }: Props) {
  const t = useT();
  const [renaming, setRenaming] = useState<string | null>(null);
  const [deleting, setDeleting] = useState<TabInfo | null>(null);
  const nameOf = (tab: TabInfo) => (tab.title === "" ? t("tabs.untitled") : tab.title);

  return (
    <nav className="ed-tabs" aria-label={t("tabs.label")}>
      <ul className="ed-tab-list">
        {tabs.map((tab, i) => {
          const current = tab.id === shown;
          return (
            <li key={tab.id} className={current ? "ed-tab-item ed-tab-current" : "ed-tab-item"}>
              {renaming === tab.id ? (
                <TabName
                  initial={tab.title}
                  onDone={(title) => {
                    setRenaming(null);
                    if (title !== null) onRename(tab.id, title);
                  }}
                />
              ) : (
                <button
                  type="button"
                  className="ed-tab-name"
                  aria-current={current ? "true" : undefined}
                  title={nameOf(tab)}
                  onClick={() => {
                    onShow(tab.id);
                  }}
                  onDoubleClick={() => {
                    if (editable) setRenaming(tab.id);
                  }}
                >
                  {nameOf(tab)}
                </button>
              )}
              {editable && current && renaming !== tab.id && (
                <span className="ed-tab-tools">
                  <ToolButton label={t("tabs.rename")} onClick={() => { setRenaming(tab.id); }}>
                    <Pencil />
                  </ToolButton>
                  <ToolButton label={t("tabs.moveUp")} disabled={i === 0} onClick={() => { onMove(tab.id, -1); }}>
                    <TabUp />
                  </ToolButton>
                  <ToolButton label={t("tabs.moveDown")} disabled={i === tabs.length - 1} onClick={() => { onMove(tab.id, 1); }}>
                    <TabDown />
                  </ToolButton>
                  <ToolButton label={t("tabs.delete")} disabled={tabs.length < 2} onClick={() => { setDeleting(tab); }}>
                    <Trash />
                  </ToolButton>
                </span>
              )}
            </li>
          );
        })}
      </ul>
      {editable && (
        <button type="button" className="quiet ed-tab-add" onClick={onAdd}>
          <Plus />
          {t("tabs.add")}
        </button>
      )}
      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(open) => {
          if (!open) setDeleting(null);
        }}
        title={t("tabs.confirmDelete")}
        hint={format(t("tabs.confirmDeleteHint"), { name: deleting ? nameOf(deleting) : "" })}
        confirm={t("tabs.deleteConfirm")}
        failure={t("tabs.deleteFailed")}
        danger
        onConfirm={() => {
          const tab = deleting;
          setDeleting(null);
          if (tab) onDelete(tab.id);
          return Promise.resolve();
        }}
      />
    </nav>
  );
}

function ToolButton({ label, disabled = false, onClick, children }: { label: string; disabled?: boolean; onClick: () => void; children: ReactNode }) {
  return (
    <button type="button" className="quiet icon-button ed-tab-tool" aria-label={label} title={label} disabled={disabled} onClick={onClick}>
      {children}
    </button>
  );
}

/** The tab's name as a field: Enter and leaving keep what was typed (when it is a title the doc can hold), Escape keeps the old one. */
function TabName({ initial, onDone }: { initial: string; onDone: (title: string | null) => void }) {
  const t = useT();
  const [text, setText] = useState(initial);
  const input = useRef<HTMLInputElement>(null);
  const finished = useRef(false);
  useEffect(() => {
    input.current?.select();
  }, []);
  const finish = (keep: boolean) => {
    if (finished.current) return;
    finished.current = true;
    onDone(keep ? cleanTitle(text) : null);
  };
  return (
    <input
      ref={input}
      className="ed-tab-field"
      value={text}
      maxLength={maxTitleLength}
      aria-label={t("tabs.renameField")}
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
  );
}
