import { format, useT } from "../../i18n";
import type { ElementReport } from "../inspect";
import type { Placed } from "../manifest";
import { keyOf, type Selected } from "../useSelection";
import type { View } from "../view";
import { anchorAt } from "./anchor";
import type { DesignComments } from "./types";
import type { CommentPin } from "./useCommentPins";
import "./comments.css";

interface Props {
  comments: DesignComments;
  frames: Placed[];
  pins: CommentPin[];
  selected: Selected[];
  reports: Record<string, ElementReport>;
  view: View;
}

/** The layer lives outside the scaled world so pins and touch targets keep their screen size. */
export function CommentsLayer({ comments, frames, pins, selected, reports, view }: Props) {
  const t = useT();
  const last = selected[selected.length - 1];
  const report = last ? reports[keyOf(last)] : undefined;
  const frame = last ? frames.find((at) => at.frame.id === last.frameId) : undefined;
  const atScreen = (x: number, y: number) => ({ left: view.x + x * view.z, top: view.y + y * view.z });
  return (
    <div className="design-comments" aria-label={t("comments.canvasPins")}>
      {report && frame && last && <button type="button" className="design-comment-selection" onClick={() => {
        const name = report.attrs.find(([key]) => key === "data-name")?.[1] ?? frame.frame.name;
        comments.onCompose(anchorAt(frame.frame, { x: report.rect.x + report.rect.w, y: report.rect.y }, last.pid, name));
      }}>{t("comments.onSelection")}</button>}
      {pins.map((pin) => <button key={pin.id} type="button" className={`design-comment-pin${comments.active === pin.id ? " active" : ""}`} style={atScreen(pin.x, pin.y)} data-comment-pin={pin.id} data-fallback={pin.fallback || undefined} aria-pressed={comments.active === pin.id} aria-label={format(t("comments.pin"), { number: String(pin.number), name: pin.name })} title={format(t(pin.fallback ? "comments.pinFallback" : "comments.pin"), { number: String(pin.number), name: pin.name })} onClick={() => { comments.onOpen(pin.id); }}>{pin.number}</button>)}
    </div>
  );
}
