import type { Boot } from "../boot";
import { Header } from "../Header";
import { format, useT } from "../i18n";
import { useDocumentTitle } from "../title";
import "./oauth.css";

/** Native form: only the server validates the request, consent token and redirect. */
export function OAuthConsent({ boot }: { boot: Boot }) {
  const t = useT();
  useDocumentTitle(t("connect.oauth.consentTitle"));
  const request = boot.oauth;
  if (!request) throw new Error("the authorization page has no request");
  return <><Header boot={boot} /><main className="page oauth-consent">
    <form method="post" action={request.action} aria-labelledby="oauth-consent-title" aria-describedby="oauth-access oauth-warning">
      {Object.entries(request.fields).map(([name, value]) => <input key={name} type="hidden" name={name} value={value} />)}
      <p className="oauth-eyebrow">{t("connect.oauth.consentTitle")}</p>
      <h1 id="oauth-consent-title">{format(t("connect.oauth.allowClient"), { name: request.clientName })}</h1>
      <p className="lead" id="oauth-access">{format(t("connect.oauth.access"), { user: boot.displayName ?? boot.user })}</p>
      <dl className="oauth-request"><div><dt>{t("connect.oauth.clientId")}</dt><dd className="mono">{request.clientId}</dd></div><div><dt>{t("connect.oauth.callback")}</dt><dd className="mono">{request.fields.redirect_uri}</dd></div></dl>
      <p id="oauth-warning" className="oauth-warning">{t("connect.oauth.warning")}</p>
      <p className="hint">{t("connect.oauth.manageHint")}</p>
      <div className="oauth-actions"><button type="submit" name="decision" value="deny">{t("connect.oauth.deny")}</button><button type="submit" name="decision" value="allow" className="primary">{t("connect.oauth.allow")}</button></div>
    </form>
  </main></>;
}
