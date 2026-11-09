import { useEffect, useRef, useState } from "react";
import { addComment, deleteComment, markCommentsSeen, maxCommentChars, type Artifact, type Comment, type CommentSummary } from "../api";
import { RelativeTime } from "../RelativeTime";
import { format, useT } from "../i18n";
import { ConfirmDialog } from "./ConfirmDialog";
import type { CommentTarget } from "../design/comments/types";
import type { Comments } from "./useComments";

interface Props {
  artifact: Artifact;
  onClose: () => void;
  /** What the artifact's comments add up to now, for the button that opened this and for the overview. */
  onSummary: (s: CommentSummary) => void;
  /** The comments, read by the viewer: the doc editor draws the same ones. */
  comments: Comments;
  /** What the comment being written points at, when it points at a place (the words selected in the doc). */
  draft: Record<string, unknown> | null;
  /** The person gave up on the comment on the selection, or it was sent. */
  onDraftDone: () => void;
  /** The thread that is open: from a click on its words in the doc, or on its quote here. */
  active: string | null;
  onActivate: (id: string) => void;
  /** The threads whose words are no longer in the doc, by comment id. */
  changed: ReadonlySet<string>;
  /** The human-readable source target for a kind whose anchors are not text quotes. */
  targets?: Record<string, CommentTarget> | undefined;
  anchorLabel?: ((anchor: Record<string, unknown> | undefined) => string | null) | undefined;
}

/** The words a comment is about, when its anchor quotes some. */
function quoteOf(anchor: Record<string, unknown> | undefined): string | null {
  return typeof anchor?.quote === "string" && anchor.quote !== "" ? anchor.quote : null;
}

/** Top-level comments in order, each followed by the replies of its thread. */
function threads(list: Comment[]): { comment: Comment; replies: Comment[] }[] {
  return list
    .filter((c) => !c.parentId)
    .map((comment) => ({ comment, replies: list.filter((r) => r.parentId === comment.id) }));
}

/** The words a thread is about, above it; a button that opens the thread in the doc, and a note when those words are gone from it. */
function Quote({ comment, changed, onShow, anchorLabel, target }: { comment: Comment; changed: boolean; onShow: () => void; anchorLabel?: Props["anchorLabel"]; target?: CommentTarget | undefined }) {
  const t = useT();
  const quote = quoteOf(comment.anchor);
  const source = anchorLabel?.(comment.anchor) ?? null;
  if (quote === null && source === null) return null;
  return (
    <div className="comment-anchor">
      <button type="button" className="quiet comment-quote" disabled={target === "removed"} title={t(source === null ? "comments.showInDoc" : "comments.showOnCanvas")} onClick={onShow}>
        {source === null ? quote : format(t("comments.onDesign"), { name: source })}
      </button>
      {target && target !== "available" && <span className="comment-changed">{t(target === "removed" ? "comments.targetRemoved" : "comments.targetUnavailable")}</span>}
      {source && !comment.anchor?.pid && <span className="comment-changed">{t("comments.framePosition")}</span>}
      {changed && <span className="comment-changed">{t("comments.quoteChanged")}</span>}
    </div>
  );
}

/**
 * The conversation under an artifact, beside the page. Opening it as the author
 * marks what it showed as seen, up to the newest comment it listed, so a
 * comment that arrives meanwhile still counts as new.
 */
export function CommentsPanel({ artifact, onClose, onSummary, comments, draft, onDraftDone, active, onActivate, changed, anchorLabel, targets }: Props) {
  const t = useT();
  const load = comments.load;
  const [deleting, setDeleting] = useState<Comment | null>(null);
  const [text, setText] = useState("");
  const [replyTo, setReplyTo] = useState<Comment | null>(null);
  const [sending, setSending] = useState(false);
  const [failure, setFailure] = useState<string | null>(null);
  const composer = useRef<HTMLTextAreaElement>(null);
  const threadElements = useRef(new Map<string, HTMLLIElement>());
  useEffect(() => {
    if (active) threadElements.current.get(active)?.scrollIntoView({ block: "nearest" });
  }, [active]);
  // Choosing Comment on some words puts the writer in the box.
  useEffect(() => {
    if (draft) composer.current?.focus();
  }, [draft]);
  const summary = useRef(onSummary);
  useEffect(() => {
    summary.current = onSummary;
  });

  const id = artifact.id;
  const mine = artifact.canChange;
  const { refresh } = comments;
  useEffect(() => {
    let current = true;
    void refresh().then((list) => {
      const newest = list?.[list.length - 1];
      if (!current || !list || !mine || !newest) return;
      markCommentsSeen(id, newest.id).then(
        () => {
          if (current) summary.current({ count: list.filter((c) => !c.deleted).length, unread: 0 });
        },
        (err: unknown) => {
          console.error("mark comments seen", err);
        },
      );
    });
    return () => {
      current = false;
    };
  }, [id, mine, refresh]);

  const body = text.trim();
  const tooLong = Array.from(body).length > maxCommentChars;

  async function send() {
    if (body === "" || tooLong || sending || load.status !== "ready") return;
    setSending(true);
    setFailure(null);
    try {
      // A reply to a reply joins the thread of the one it answers.
      const saved = await addComment(id, body, replyTo ? (replyTo.parentId ?? replyTo.id) : undefined, replyTo ? undefined : (draft ?? undefined));
      comments.add(saved);
      setText("");
      setReplyTo(null);
      if (draft) onDraftDone();
      summary.current({ count: load.list.filter((c) => !c.deleted).length + 1, unread: 0 });
    } catch (err) {
      console.error("add comment", err);
      setFailure(t("comments.sendFailed"));
    } finally {
      setSending(false);
    }
  }

  return (
    <aside
      className="versions comments"
      aria-label={t("comments.title")}
      onKeyDown={(e) => {
        // A draft is never thrown away by a stray Escape.
        if (e.key === "Escape" && text === "" && !deleting) onClose();
      }}
    >
      <header className="versions-head">
        <h2>{t("comments.title")}</h2>
        <button type="button" className="quiet" onClick={onClose}>
          {t("comments.close")}
        </button>
      </header>
      {load.status === "failed" && (
        <p className="error" role="alert">
          {t("comments.loadFailed")}
        </p>
      )}
      {load.status === "ready" && load.list.length === 0 && <p className="comments-empty">{t("comments.empty")}</p>}
      {load.status === "ready" && load.list.length > 0 && (
        <ol className="comment-list">
          {threads(load.list).map(({ comment, replies }) => (
            <li key={comment.id} ref={(el) => { if (el) threadElements.current.set(comment.id, el); else threadElements.current.delete(comment.id); }} className={`comment-thread${comment.id === active ? " active" : ""}`}>
              <Quote target={targets?.[comment.id]} anchorLabel={anchorLabel} comment={comment} changed={changed.has(comment.id)} onShow={() => { onActivate(comment.id); }} />
              {[comment, ...replies].map((c) => (
                <article key={c.id} className={`comment${c.parentId ? " reply" : ""}${c.unread ? " unread" : ""}`}>
                  <p className="facts mono">
                    <span>{c.author}</span>
                    <RelativeTime iso={c.createdAt} />
                    {c.unread && <span className="comment-new">{t("comments.new")}</span>}
                  </p>
                  <p className="comment-body">{c.deleted ? t("comments.deleted") : c.body}</p>
                  {c.canDelete && !c.deleted && <button type="button" className="quiet" onClick={() => { setDeleting(c); }}>{t("comments.delete")}</button>}
                </article>
              ))}
              <button
                type="button"
                className="quiet comment-reply"
                onClick={() => {
                  if (draft) onDraftDone();
                  setReplyTo(comment);
                  composer.current?.focus();
                }}
              >
                {t("comments.reply")}
              </button>
            </li>
          ))}
        </ol>
      )}
      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(open) => { if (!open) setDeleting(null); }}
        title={t("comments.deleteTitle")}
        hint={t("comments.deleteHint")}
        confirm={t("comments.delete")}
        failure={t("comments.deleteFailed")}
        danger
        onConfirm={async () => {
          if (!deleting || load.status !== "ready") return;
          await deleteComment(id, deleting.id);
          comments.remove(deleting.id);
          summary.current({ count: load.list.filter((c) => !c.deleted && c.id !== deleting.id).length, unread: 0 });
          if (replyTo?.id === deleting.id) setReplyTo(null);
          setDeleting(null);
        }}
      />
      <form
        className="versions-foot comment-form"
        onSubmit={(e) => {
          e.preventDefault();
          void send();
        }}
      >
        {draft && !replyTo && (
          <p className="comment-replying comment-anchoring">
            <span>
              {t("comments.commentingOn")} <q>{anchorLabel?.(draft) ?? quoteOf(draft)}</q>
            </span>
            <button type="button" className="quiet" onClick={onDraftDone}>
              {t("comments.cancelAnchor")}
            </button>
          </p>
        )}
        {replyTo && (
          <p className="comment-replying">
            <span>{format(t("comments.replyingTo"), { author: replyTo.author })}</span>
            <button
              type="button"
              className="quiet"
              onClick={() => {
                setReplyTo(null);
              }}
            >
              {t("comments.cancelReply")}
            </button>
          </p>
        )}
        <textarea
          ref={composer}
          aria-label={t("comments.label")}
          rows={3}
          value={text}
          onChange={(e) => {
            setText(e.target.value);
          }}
          onKeyDown={(e) => {
            if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
              e.preventDefault();
              void send();
            }
          }}
        />
        {tooLong && <p className="error">{format(t("comments.tooLong"), { max: String(maxCommentChars) })}</p>}
        {failure && (
          <p className="error" role="alert">
            {failure}
          </p>
        )}
        <div className="dialog-actions">
          <button type="submit" className="primary" disabled={body === "" || tooLong || sending || load.status !== "ready"}>
            {t("comments.send")}
          </button>
        </div>
      </form>
    </aside>
  );
}
