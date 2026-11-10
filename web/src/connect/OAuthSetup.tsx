import { Code } from "../Code";
import { useT } from "../i18n";
import { oauthClaudeCommand, oauthCodexCommand } from "./snippets";
import "./oauth.css";

/** Start from the workspace URL. The client owns discovery and the browser sign-in. */
export function OAuthSetup({ baseUrl }: { baseUrl: string }) {
  const t = useT();
  return <section className="step oauth-setup" aria-labelledby="oauth-setup-title">
    <h2 id="oauth-setup-title">{t("connect.oauth.title")}</h2>
    <p className="lead">{t("connect.oauth.instructions")}</p>
    <Code text={`${baseUrl}/api/mcp`} />
    <div className="oauth-clients">
      <div><h3>{t("connect.client.claude")}</h3><Code text={oauthClaudeCommand(baseUrl)} /><p className="hint">{t("connect.oauth.claude")}</p></div>
      <div><h3>{t("connect.client.codex")}</h3><Code text={oauthCodexCommand(baseUrl)} /><p className="hint">{t("connect.oauth.codex")}</p></div>
    </div>
    <p className="hint">{t("connect.oauth.others")}</p>
  </section>;
}
