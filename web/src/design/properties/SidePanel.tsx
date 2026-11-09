import { useState, type ReactNode } from "react";
import { Tooltip } from "../../Tooltip";
import { Close } from "../../icons/chrome";
import { useT } from "../../i18n";
import { CodePanel } from "../CodePanel";
import type { Target } from "../controller";
import type { DesignEdits } from "../edits";
import type { ElementReport } from "../inspect";
import { ImageSection } from "../ops/ImageSection";
import { TokenSection } from "./TokenSection";
import { PropertiesPanel } from "./PropertiesPanel";

interface Props {
  /** What the frame said about the last element selected; absent until it answers. */
  report: ElementReport | undefined;
  count: number;
  layers?: ReactNode;
  artboard?: ReactNode;
  onClose: () => void;
  /** Present for someone who can change the design: the properties are theirs to edit. Everyone else has only the code. */
  edit?: { edits: DesignEdits; targets: Target[]; artifactId: string };
}

/**
 * The panel at the right of the canvas when something is selected. Someone who
 * can edit the newest version has two tabs, the element's properties to change
 * and its code to read; everyone else has the code alone.
 */
export function SidePanel({ report, count, onClose, edit, layers, artboard }: Props) {
  const t = useT();
  const [tab, setTab] = useState<"design" | "code">("design");
  const active = edit || artboard ? tab : "code";
  if (!edit && !layers && !artboard) return <CodePanel report={report} count={count} onClose={onClose} />;
  return (
    <aside className="code-panel side-panel" role="region" aria-label={t("props.panel")}>
      <div className="versions-head">
        <div className="side-tabs" role="tablist" aria-label={t("props.panel")}>
          {(edit || artboard ? ["design", "code"] as const : ["code"] as const).map((id) => (
            <button
              key={id}
              type="button"
              role="tab"
              aria-selected={active === id}
              onClick={() => {
                setTab(id);
              }}
            >
              {t(id === "design" ? "props.tabDesign" : "props.tabCode")}
            </button>
          ))}
        </div>
        <Tooltip content={t("props.close")}><button type="button" aria-label={t("props.close")} onClick={onClose}>
          <Close />
        </button></Tooltip>
      </div>
      {layers && <details className="shell-layers" open><summary>{t("design.outline")}</summary>{layers}</details>}
      {active === "design" ? (
        <div className="code-body" role="tabpanel">
          {artboard}
          {edit && <PropertiesPanel edits={edit.edits} targets={edit.targets} />}
          {edit && <TokenSection edits={edit.edits} />}
          {edit && <ImageSection artifactId={edit.artifactId} edits={edit.edits} targets={edit.targets} />}
        </div>
      ) : (
        <CodePanel report={report} count={count} onClose={onClose} embedded />
      )}
    </aside>
  );
}
