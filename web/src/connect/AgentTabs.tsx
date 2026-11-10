import { useId, useState, type KeyboardEvent, type ReactNode } from "react";
import { useT, type Key } from "../i18n";
import { clients, type Client } from "./snippets";

const labels: Record<Client, Key> = {
  claude: "connect.client.claude",
  codex: "connect.client.codex",
  other: "connect.client.other",
};

/** Browser authorization and personal tokens use the same accessible client chooser. */
export function AgentTabs({ children }: { children: (client: Client) => ReactNode }) {
  const t = useT();
  const id = useId();
  const [active, setActive] = useState<Client>("claude");
  const onKey = (event: KeyboardEvent) => {
    let index = clients.indexOf(active);
    if (event.key === "ArrowRight") index++;
    else if (event.key === "ArrowLeft") index--;
    else if (event.key === "Home") index = 0;
    else if (event.key === "End") index = clients.length - 1;
    else return;
    event.preventDefault();
    const next = clients[(index + clients.length) % clients.length];
    if (!next) return;
    setActive(next);
    document.getElementById(`${id}-tab-${next}`)?.focus();
  };
  return <>
    <div className="tabs agent-tabs" role="tablist" aria-label={t("connect.clients")} onKeyDown={onKey}>
      {clients.map(client => <button key={client} type="button" role="tab" className="tab" id={`${id}-tab-${client}`} aria-selected={active === client} aria-controls={`${id}-panel`} tabIndex={active === client ? 0 : -1} onClick={() => { setActive(client); }}>
        {t(labels[client])}
      </button>)}
    </div>
    <div className="tab-panel" key={active} id={`${id}-panel`} role="tabpanel" aria-labelledby={`${id}-tab-${active}`} tabIndex={0}>
      {children(active)}
    </div>
  </>;
}
