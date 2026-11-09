import { Tooltip } from "../Tooltip";
import { useEffect, useRef, useState } from "react";
import { format, useT } from "../i18n";
import { colorClassOf, initialOf, keyOf, type Person } from "./people";
import "./presence.css";

/** How many avatars show before the rest fold into "+N". */
export const visibleAvatars = 4;

function Face({ p }: { p: Person }) {
  return (
    <span className={`pr-face ${colorClassOf(p)}${p.agent ? " pr-agent" : ""}${p.guest === true ? " pr-guest" : ""}${p.self ? " pr-self" : ""}`} aria-hidden="true">
      {p.agent ? "AI" : initialOf(p.name)}
    </span>
  );
}

/**
 * Who is here, as one overlapping row of avatars: the first few, "+N" for the rest, an AI mark on an agent, a guest mark on a guest. The one
 * look for every page, Docs and Design included. Hovering or focusing an avatar names it; clicking any of them opens
 * a list of everyone, which Esc, an outside click or leaving it with the keyboard closes. The non-interactive name hint uses the same primitive as other interface hints.
 */
export function AvatarStack({ people, limit = visibleAvatars }: { people: Person[]; limit?: number }) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const away = (e: Event) => {
      if (!root.current?.contains(e.target as Node)) setOpen(false);
    };
    const escape = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      setOpen(false);
    };
    document.addEventListener("pointerdown", away);
    document.addEventListener("keydown", escape);
    return () => {
      document.removeEventListener("pointerdown", away);
      document.removeEventListener("keydown", escape);
    };
  }, [open]);
  if (people.length === 0) return null;
  const shown = people.slice(0, limit);
  const more = people.length - shown.length;
  const marks = (p: Person) => [p.agent ? t("presence.agent") : null, p.guest === true ? t("presence.guest") : null, p.self ? t("presence.you") : null].filter((m) => m !== null);
  const label = (p: Person) => (marks(p).length === 0 ? p.name : `${p.name} (${marks(p).join(", ")})`);
  const moreLabel = format(t("presence.more"), { count: String(more) });
  const toggle = () => {
    setOpen(!open);
  };
  return (
    <div
      className="pr-stack"
      ref={root}
      role="group"
      aria-label={t("presence.label")}
      onBlur={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget)) setOpen(false);
      }}
    >
      <ul className="pr-avatars">
        {shown.map((p) => (
          <li key={keyOf(p)} className="pr-item">
            <Tooltip disabled={open} content={<>{p.name}{marks(p).map((m) => <span key={m} className="pr-mark"> {m}</span>)}</>}>
              <button type="button" className="pr-avatar" aria-haspopup="dialog" aria-expanded={open} onClick={toggle}>
                <Face p={p} />
                <span className="visually-hidden">{label(p)}</span>
              </button>
            </Tooltip>
          </li>
        ))}
      </ul>
      {more > 0 && (
        <span className="pr-item">
          <Tooltip disabled={open} content={moreLabel}>
            <button type="button" className="pr-more" aria-haspopup="dialog" aria-expanded={open} aria-label={moreLabel} onClick={toggle}>
              +{more}
            </button>
          </Tooltip>
        </span>
      )}
      {open && (
        <div className="pr-names" role="dialog" aria-label={t("presence.everyone")}>
          <ul>
            {people.map((p) => (
              <li key={keyOf(p)}>
                <Face p={p} />
                <span className="pr-name">{p.name}</span>
                {marks(p).map((m) => (
                  <span key={m} className="pr-mark">{m}</span>
                ))}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
