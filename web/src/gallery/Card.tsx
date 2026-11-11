import { Tooltip } from "../Tooltip";
import type { Artifact } from "../api";
import { RelativeTime } from "../RelativeTime";
import { format, useT } from "../i18n";
import { inApp, useRouter } from "../router";
import { KindMark } from "../kinds/KindMark";
import { displayOf, kindsTable } from "../kinds/kinds";
import { Audience, AuthorFace, RenameField, useItemActions } from "./itemActions";
import { authorName } from "./author";
import { Preview } from "./Preview";

interface Props {
  artifact: Artifact;
  /** Called just before a card's pin changes, so the wall can slide the others to their new places. */
  beforePin: () => void;
}

export function Card({ artifact: a, beforePin }: Props) {
  const t = useT();
  const router = useRouter();
  const item = useItemActions(a, beforePin);
  const href = `/a/${a.id}`;
  const token = kindsTable[displayOf(a.kind)].token;

  const title = item.renaming ? (
    <RenameField title={a.title} onDone={(title) => void item.saveTitle(title)} />
  ) : (
    <Tooltip content={a.title}><h2 className="card-title" tabIndex={0}>
      {a.title}
    </h2></Tooltip>
  );

  const body = (
    <>
      <Preview id={a.id} version={a.version} />
      <div className="card-meta">
        <KindMark kind={a.kind} />
        <div className="card-text">
          {title}
          <div className="card-sub">
            <RelativeTime iso={a.updatedAt} sentence="time.edited" />
            <span className="card-author">
              <AuthorFace name={authorName(a)} />
              <span className="card-author-name">{authorName(a)}</span>
            </span>
          </div>
        </div>
        <Audience visibility={a.visibility} />
        {a.comments.unread > 0 && <span className="comments-new">{format(t("comments.newCount"), { count: String(a.comments.unread) })}</span>}
      </div>
    </>
  );

  return (
    <li className="card" data-artifact-id={a.id} style={{ "--wash": `var(--kind-${token}-bg)` } as React.CSSProperties}>
      {item.renaming ? (
        <div className="card-link">{body}</div>
      ) : (
        <a className="card-link" href={href} onClick={inApp(router, href)}>
          {body}
        </a>
      )}
      <div className="card-actions">
        {item.menu}
        {item.pin}
      </div>
      {item.dialog}
    </li>
  );
}
