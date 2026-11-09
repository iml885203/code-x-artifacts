import { Tooltip } from "../Tooltip";
import { Icon } from "../icons/Icon";
import type { CommentSummary } from "../api";
import { format, useT } from "../i18n";

interface Props {
  summary: CommentSummary;
  open: boolean;
  onClick: () => void;
}

/** Opens the comments. The badge shows unread comments for the author, otherwise the total; unread is bold and inverted. */
export function CommentsButton({ summary, open, onClick }: Props) {
  const t = useT();
  const fresh = summary.unread > 0;
  return (
    <Tooltip content={fresh ? format(t("comments.openWithNew"), { count: String(summary.unread) }) : t("comments.title")} disabled={open}><button
      type="button"
      className="comments-button icon-button"
      aria-pressed={open}
      aria-label={fresh ? format(t("comments.openWithNew"), { count: String(summary.unread) }) : t("comments.title")}
      onClick={onClick}
    >
      <Icon><path d="M2 3h12v8H6l-4 3V3Z" /></Icon>
      {summary.count > 0 && <span className={`comments-count mono${fresh ? " fresh" : ""}`}>{fresh ? summary.unread : summary.count}</span>}
    </button></Tooltip>
  );
}
