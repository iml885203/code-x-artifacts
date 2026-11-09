import { useEffect, useRef, useState } from "react";
import { RelativeTime } from "../RelativeTime";
import { useT, type Key } from "../i18n";
import { activityTarget, readAgentActivity, type ActivityToken, type AgentActivityItem, type AgentActivityPage } from "./activity";
import "./RecentActivity.css";

const operationLabels: Record<AgentActivityItem["operation"], Key> = {
  connect: "connect.activity.connected",
  publish: "connect.activity.published",
  restore: "connect.activity.restored",
  doc_edit: "connect.activity.edited",
  board_edit: "connect.activity.edited",
};

function ActivityTime({ iso }: { iso: string }) {
  return <span className="activity-time"><RelativeTime iso={iso} /></span>;
}

interface Snapshot { key: string; page?: AgentActivityPage; failed: boolean }

export function RecentActivity({ tokenID, onTokenIDChange, refreshKey }: {
  tokenID: string;
  onTokenIDChange: (id: string) => void;
  refreshKey: number;
}) {
  const t = useT();
  const [retry, setRetry] = useState(0);
  const key = `${tokenID}/${String(refreshKey)}/${String(retry)}`;
  const [snapshot, setSnapshot] = useState<Snapshot>({ key: "", failed: false });
  const [tokens, setTokens] = useState<ActivityToken[]>([]);
  const [more, setMore] = useState<"idle" | "loading" | "failed">("idle");
  const pagination = useRef<AbortController | null>(null);
  const page = snapshot.key === key ? snapshot.page : undefined;
  const failed = snapshot.key === key && snapshot.failed;
  const loading = page === undefined && !failed;

  useEffect(() => {
    const controller = new AbortController();
    void readAgentActivity(tokenID, "", controller.signal).then((data) => {
      if (controller.signal.aborted) return;
      setSnapshot({ key, page: data, failed: false });
      setTokens(data.tokens);
      setMore("idle");
    }, () => {
      if (!controller.signal.aborted) setSnapshot({ key, failed: true });
    });
    return () => {
      controller.abort();
      pagination.current?.abort();
    };
  }, [key, tokenID]);

  async function loadMore() {
    if (!page?.nextCursor || more === "loading") return;
    const controller = new AbortController();
    pagination.current?.abort();
    pagination.current = controller;
    setMore("loading");
    try {
      const next = await readAgentActivity(tokenID, page.nextCursor, controller.signal);
      if (controller.signal.aborted) return;
      setSnapshot((current) => {
        if (current.key !== key || !current.page) return current;
        const ids = new Set(current.page.items.map((item) => item.id));
        return { key, failed: false, page: { ...next, items: [...current.page.items, ...next.items.filter((item) => !ids.has(item.id))] } };
      });
      setTokens(next.tokens);
      setMore("idle");
    } catch {
      if (!controller.signal.aborted) setMore("failed");
    }
  }

  const selected = tokens.filter((token) => tokenID === "" || token.id === tokenID);
  const latest = (field: "lastUsed" | "lastConnected") => selected.map((token) => token[field]).filter((iso): iso is string => iso !== undefined && iso !== "").sort((a, b) => Date.parse(a) - Date.parse(b)).at(-1);
  const used = latest("lastUsed");
  const connected = latest("lastConnected");

  return (
    <section className="step recent-activity" id="recent-activity" aria-labelledby="activity-heading">
      <div className="activity-heading">
        <h2 id="activity-heading">{t("connect.activity.title")}</h2>
        <div className="activity-filter">
          <label htmlFor="activity-token">{t("connect.activity.filter")}</label>
          <select id="activity-token" value={tokenID} onChange={(e) => { onTokenIDChange(e.target.value); }}>
            <option value="">{t("connect.activity.all")}</option>
            {tokens.map((token) => <option key={token.id} value={token.id}>{token.name}{token.revoked ? ` · ${t("connect.activity.revoked")}` : ""}</option>)}
            {tokenID !== "" && !tokens.some((token) => token.id === tokenID) && <option value={tokenID}>{t("connect.activity.selected")}</option>}
          </select>
        </div>
      </div>
      <p className="activity-note">{t("connect.activity.help")}</p>
      {loading && <p role="status">{t("connect.activity.loading")}</p>}
      {failed && <div className="activity-error"><p className="error" role="alert">{t("connect.activity.failed")}</p><button type="button" onClick={() => { setRetry((value) => value + 1); }}>{t("connect.activity.retry")}</button></div>}
      {page && <>
        <dl className="activity-summary">
          <div><dt>{t("connect.activity.lastUsed")}</dt><dd>{used ? <ActivityTime iso={used} /> : t("connect.activity.notRecorded")}</dd></div>
          <div><dt>{t("connect.activity.lastConnected")}</dt><dd>{connected ? <ActivityTime iso={connected} /> : t("connect.activity.notRecorded")}</dd></div>
        </dl>
        {page.items.length === 0 ? <p className="activity-empty">{t(tokenID === "" ? "connect.activity.empty" : "connect.activity.emptyToken")}</p> : <ol className="activity-list">
          {page.items.map((item) => {
            const target = activityTarget(item);
            return <li key={item.id}>
              <div className="activity-event">
                <span className="activity-client">{item.client === "" ? t("connect.activity.unknown") : item.client}{item.clientVersion && <span className="activity-version"> {item.clientVersion}</span>}</span>
                <span className="activity-operation">{t(operationLabels[item.operation])}{target ? <> · <a href={target.url}>{target.title}</a></> : item.operation !== "connect" ? <> · <span className="muted">{t("connect.activity.unavailable")}</span></> : null}</span>
                <span className="activity-token-label">{item.tokenLabel}{item.revoked && <span className="activity-revoked">{t("connect.activity.revoked")}</span>}</span>
              </div>
              <ActivityTime iso={item.occurredAt} />
            </li>;
          })}
        </ol>}
        {more === "failed" && <p className="error" role="alert">{t("connect.activity.moreFailed")}</p>}
        {page.nextCursor && <button type="button" className="activity-more" disabled={more === "loading"} onClick={() => { void loadMore(); }}>{t(more === "loading" ? "connect.activity.loading" : more === "failed" ? "connect.activity.retry" : "connect.activity.more")}</button>}
        <p className="activity-retention">{t("connect.activity.retention")}</p>
      </>}
    </section>
  );
}
