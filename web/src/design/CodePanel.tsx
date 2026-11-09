import { Close } from "../icons/chrome";
import { format, useT } from "../i18n";
import type { ElementReport } from "./inspect";

interface Props {
  /** What the frame said about the last element selected; absent until it answers. */
  report: ElementReport | undefined;
  /** How many elements are selected. */
  count: number;
  onClose: () => void;
  /** Shown inside another panel (the side panel's Code tab): no frame and no title of its own. */
  embedded?: boolean;
}

/**
 * What the selected element is, read only: its attributes, its inline style and
 * the values it ends up with. Everyone who can open the design sees it; nothing
 * here changes the design.
 */
export function CodePanel({ report, count, onClose, embedded = false }: Props) {
  const t = useT();
  const content = (
    <div className="code-body">
      {count > 1 && <p className="code-note">{format(t("design.selectedCount"), { count: String(count) })}</p>}
      {report ? (
        <>
          <h3 className="mono">
            &lt;{report.tag}&gt; <span>{report.pid}</span>
          </h3>
          <Section title={t("design.attributes")} rows={report.attrs} />
          <Section title={t("design.inlineStyle")} rows={report.inline} />
          <Section title={t("design.computed")} rows={Object.entries(report.computed)} />
        </>
      ) : (
        <p className="code-note">{t("design.codeLoading")}</p>
      )}
    </div>
  );
  if (embedded) return content;
  return (
    <aside className="code-panel" role="region" aria-label={t("design.code")}>
      <div className="versions-head">
        <h2>{t("design.code")}</h2>
        <button type="button" aria-label={t("design.closeCode")} title={t("design.closeCode")} onClick={onClose}>
          <Close />
        </button>
      </div>
      {content}
    </aside>
  );
}

function Section({ title, rows }: { title: string; rows: [string, string][] }) {
  if (rows.length === 0) return null;
  return (
    <section>
      <h4>{title}</h4>
      <dl className="mono">
        {rows.map(([name, value]) => (
          <div key={name} className="code-row">
            <dt>{name}</dt>
            <dd>{value}</dd>
          </div>
        ))}
      </dl>
    </section>
  );
}
