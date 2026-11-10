import { AgentTabs } from "./AgentTabs";
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
    <AgentTabs>{active => <>
      {active === "claude" && <><Code text={oauthClaudeCommand(baseUrl)} /><p className="hint">{t("connect.oauth.claude")}</p></>}
      {active === "codex" && <><Code text={oauthCodexCommand(baseUrl)} /><p className="hint">{t("connect.oauth.codex")}</p></>}
      {active === "other" && <p className="hint">{t("connect.oauth.others")}</p>}
    </>}</AgentTabs>
  </section>;
}
