import { Fragment, useRef, useState, type ReactNode } from "react";
import type { EditorState } from "prosemirror-state";
import { useT, type Translate } from "../i18n";
import { PageWidth } from "../icons/editor";
import { Tooltip } from "../Tooltip";
import { InsertionMenu } from "./InsertionMenu";
import { Plus } from "../icons/chrome";
import { More } from "../icons/menu";
import { groups, type ToolContext } from "./feature";
import { features } from "./features";
import "./insert.css";
import { RowPopover } from "./RowPopover";

interface Props {
  /** What the caret is in; the tools of every feature draw themselves from it. */
  state: EditorState;
  ctx: ToolContext;
  /** The tabs menu, which needs the doc's tabs (DocEditor builds it). */
  tabs?: ReactNode;
  /** Whether the page is wide, and how it is toggled; no button without the handler. */
  wide?: boolean;
  onWide?: () => void;
}

const tools = features.flatMap((f) => f.tools ?? []);
const rows = features.flatMap((f) => f.rows ?? []);

/** The tools of the groups the row does not show itself, in the order of `groups`. */
function MoreTools({ state, ctx, t }: { state: EditorState; ctx: ToolContext; t: Translate }) {
  return (
    <div className="ed-more">
      {groups
        .filter((group) => group.id !== "insert")
        .map((group) => {
          const drawn = tools.filter((tool) => tool.group === group.id && tool.bubble === undefined).map((tool) => <Fragment key={tool.id}>{tool.render(state, ctx, t)}</Fragment>);
          if ("bare" in group) return <Fragment key={group.id}>{drawn}</Fragment>;
          return "label" in group ? (
            <div key={group.id} className="ed-group" role="group" aria-label={t(group.label)}>
              {drawn}
            </div>
          ) : (
            <div key={group.id} className="ed-group">
              {drawn}
            </div>
          );
        })}
    </div>
  );
}

/**
 * The slim row under the top bar: the tabs menu, the page width, the blocks a doc is built from (table, picture,
 * checklist) and More. It is assembled from the features' tools (features.ts); nothing here names one. The tools
 * that belong to the caret's block (the table's, the picture's) show in a row of their own under it.
 */
export function InsertRow({ state, ctx, tabs, wide = false, onWide }: Props) {
  const t = useT();
  const [inserting, setInserting] = useState(false);
  const insertButton = useRef<HTMLButtonElement>(null);
  return (
    <>
      <div className="ed-toolbar ed-insert" role="toolbar" aria-label={t("editor.toolbar")}>
        {tabs}
        {onWide && (
          <button type="button" className="quiet icon-button" aria-label={t("editor.pageWidth")} title={t("editor.pageWidth")} aria-pressed={wide} onClick={onWide}>
            <PageWidth />
          </button>
        )}
        {tools
          .filter((tool) => tool.group === "insert")
          .map((tool) => (
            <Fragment key={tool.id}>{tool.render(state, ctx, t)}</Fragment>
          ))}
        <div className="ed-pop">
          <Tooltip content={t("editor.insertBlock")} disabled={inserting}><button ref={insertButton} type="button" className="quiet icon-button" aria-label={t("editor.insertBlock")} aria-expanded={inserting} aria-haspopup="listbox" onClick={() => { setInserting(!inserting); }}><Plus /></button></Tooltip>
          {inserting && <InsertionMenu state={state} ctx={ctx} trigger={insertButton} onClose={() => { setInserting(false); }} onCancel={() => { insertButton.current?.focus(); }} />}
        </div>
        <RowPopover label={t("editor.more")} face={<More />}>
          {() => <MoreTools state={state} ctx={ctx} t={t} />}
        </RowPopover>
      </div>
      {rows.map((row) => (
        <Fragment key={row.id}>{row.render(state, ctx, t)}</Fragment>
      ))}
    </>
  );
}
