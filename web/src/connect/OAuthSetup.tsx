import { AgentTabs } from "./AgentTabs";
import { Code } from "../Code";
import { useT } from "../i18n";
import { oauthClaudeCommand, oauthCodexCommand } from "./snippets";
import "./oauth.css";

/** The selected client determines the primary setup path; no credential to copy. */
export function OAuthSetup({ baseUrl }: { baseUrl: string }) {
  const t = useT();
  return <section className="step oauth-setup" aria-labelledby="oauth-setup-title">
    <h2 id="oauth-setup-title">{t("connect.oauth.title")}</h2>
    <AgentTabs>{active => <>
      {active !== "other" ? <ol className="connect-instructions">
        <li><p>{t("connect.oauth.run")}</p><Code text={active === "claude" ? oauthClaudeCommand(baseUrl) : oauthCodexCommand(baseUrl)} /></li>
        <li><p>{t(active === "claude" ? "connect.oauth.claude" : "connect.oauth.codex")}</p></li>
      </ol> : <ol className="connect-instructions">
        <li><p>{t("connect.oauth.instructions")}</p><Code text={`${baseUrl}/api/mcp`} /></li>
        <li><p>{t("connect.oauth.others")}</p></li>
      </ol>}
    </>}</AgentTabs>
  </section>;
}
