import { Tooltip } from "../Tooltip";
import type { Artifact } from "../api";
import { RelativeTime } from "../RelativeTime";
import { format, useT } from "../i18n";
import { inApp, useRouter } from "../router";
import { KindMark } from "../kinds/KindMark";
import { Audience, RenameField, useItemActions } from "./itemActions";

interface Props {
  artifact: Artifact;
  /** Called just before a row's pin changes, so the wall can slide the others to their new places. */
  beforePin: () => void;
}

/** One artifact as a borderless row: the kind's tile, the title, and on the right what a card keeps in its corner and its meta line. */
export function Row({ artifact: a, beforePin }: Props) {
  const t = useT();
  const router = useRouter();
  const item = useItemActions(a, beforePin);
  const href = `/a/${a.id}`;
  const tile = <KindMark kind={a.kind} size="square" />;

  return (
    <li className="lib-row" data-artifact-id={a.id}>
      {item.renaming ? (
        <div className="row-link">
          {tile}
          <RenameField title={a.title} onDone={(title) => void item.saveTitle(title)} />
        </div>
      ) : (
        <a className="row-link" href={href} onClick={inApp(router, href)}>
          {tile}
          <Tooltip content={a.title}><h2 className="row-title" tabIndex={0}>
            {a.title}
          </h2></Tooltip>
          {a.comments.unread > 0 && (
            <span className="row-unread" role="img" aria-label={format(t("comments.newCount"), { count: String(a.comments.unread) })}>
              {a.comments.unread}
            </span>
          )}
        </a>
      )}
      <div className="row-side">
        {item.pin}
        <Audience visibility={a.visibility} />
        <RelativeTime iso={a.updatedAt} />
        {item.menu}
      </div>
      {item.dialog}
    </li>
  );
}
