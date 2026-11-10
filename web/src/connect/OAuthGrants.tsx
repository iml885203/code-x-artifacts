import { useEffect, useState } from "react";
import { call } from "../api";
import { RelativeTime } from "../RelativeTime";
import { format, useT } from "../i18n";
import { ConfirmDialog } from "../viewer/ConfirmDialog";

interface Grant { id: string; clientId: string; clientName: string; createdAt: string }
type GrantsState = { status: "loading" } | { status: "failed" } | { status: "ready"; items: Grant[] };

/** The server's owner-scoped grants, never the client's access or refresh tokens. */
export function OAuthGrants({ onRefresh }: { onRefresh?: () => void }) {
  const t = useT();
  const [snapshot, setSnapshot] = useState<GrantsState>({ status: "loading" });
  const [retry, setRetry] = useState(0);
  const [revoking, setRevoking] = useState<Grant | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    void call("GET", "/api/oauth/grants", undefined, controller.signal).then(async (response) => {
      const items = await response.json() as Grant[];
      if (!controller.signal.aborted) setSnapshot({ status: "ready", items });
    }).catch(() => {
      if (!controller.signal.aborted) setSnapshot({ status: "failed" });
    });
    return () => { controller.abort(); };
  }, [retry]);

  function refresh() {
    setSnapshot({ status: "loading" });
    setRetry((value) => value + 1);
    onRefresh?.();
  }

  async function revoke() {
    if (!revoking) return;
    await call("DELETE", `/api/oauth/grants/${encodeURIComponent(revoking.id)}`);
    setSnapshot((current) => current.status === "ready" ? { status: "ready", items: current.items.filter((item) => item.id !== revoking.id) } : current);
    setRevoking(null);
    onRefresh?.();
  }

  return <section className="step" id="oauth-grants" aria-labelledby="oauth-grants-title" aria-busy={snapshot.status === "loading"}>
    <div className="activity-heading">
      <h2 id="oauth-grants-title">{t("connect.oauth.grants")}</h2>
      <button type="button" onClick={refresh} disabled={snapshot.status === "loading"}>
        {t(snapshot.status === "loading" ? "connect.oauth.refreshing" : "connect.oauth.refresh")}
      </button>
    </div>
    <p className="hint">{t("connect.oauth.grantsHint")}</p>
    {snapshot.status === "loading" && <p role="status">{t("connect.oauth.loading")}</p>}
    {snapshot.status === "failed" && <><p className="error" role="alert">{t("connect.oauth.failed")}</p><button type="button" onClick={refresh}>{t("connect.activity.retry")}</button></>}
    {snapshot.status === "ready" && (snapshot.items.length === 0 ? <p className="lead">{t("connect.oauth.empty")}</p> : <div className="table-wrap"><table>
      <thead><tr><th>{t("connect.oauth.client")}</th><th>{t("connect.oauth.authorized")}</th><th /></tr></thead>
      <tbody>{snapshot.items.map((item) => <tr key={item.id}>
        <td><span>{item.clientName}</span><small className="oauth-client-id mono muted">{item.clientId}</small></td>
        <td className="muted mono"><RelativeTime iso={item.createdAt} /></td>
        <td className="end"><button type="button" className="danger quiet" aria-label={format(t("connect.oauth.revokeNamed"), { name: item.clientName })} onClick={() => { setRevoking(item); }}>{t("connect.tokens.revoke")}</button></td>
      </tr>)}</tbody>
    </table></div>)}
    <ConfirmDialog open={revoking !== null} onOpenChange={(open) => { if (!open) setRevoking(null); }} title={format(t("connect.tokens.confirm"), { name: revoking?.clientName ?? "" })} hint={t("connect.oauth.revokeHint")} confirm={t("connect.tokens.revoke")} failure={t("connect.oauth.revokeFailed")} danger onConfirm={revoke} />
  </section>;
}
