import {
  useEffect,
  useRef,
  useState,
  type SyntheticEvent,
} from "react";
import { OAuthSetup } from "./OAuthSetup";
import { OAuthGrants } from "./OAuthGrants";
import { RecentActivity } from "./RecentActivity";
import { createToken, listTokens, revokeToken } from "../api";
import { Code } from "../Code";
import { CopyButton } from "../Copy";
import { RelativeTime } from "../RelativeTime";
import { Nav } from "../gallery/Nav";
import { AgentTabs } from "./AgentTabs";
import "./connect.css";
import { format, useT } from "../i18n";
import { useDocumentTitle } from "../title";
import type { Boot, TokenItem } from "../boot";
import { ConfirmDialog } from "../viewer/ConfirmDialog";
import {
  claudeCommand,
  codexCommand,
  codexConfig,
  codexEnv,
  mcpJson,
  tokenPlaceholder,
} from "./snippets";

type Step1 =
  | { status: "naming"; failed: boolean }
  | { status: "creating" }
  | { status: "made"; token: string };

/**
 * Connecting an agent in two steps, then the tokens that exist. The token made in step 1 lives only in this
 * component's state: it is not in the page the server sent, the address, the
 * log or browser storage, so a reload is the end of it.
 */
export function Connect({ boot }: { boot: Boot }) {
  const t = useT();
  useDocumentTitle(t("connect.title"));
  const data = boot.connect;
  if (!data) throw new Error("the connect page has no connect data");
  const [step, setStep] = useState<Step1>({ status: "naming", failed: false });
  const token = step.status === "made" ? step.token : tokenPlaceholder;
  const [tokens, setTokens] = useState(data.tokens);
  const [activityToken, setActivityToken] = useState("");
  const [activityRevision, setActivityRevision] = useState(0);

  // A token just made is in the list below it; the list is the server's, so ask again.
  useEffect(() => {
    if (step.status !== "made") return;
    void listTokens().then((items) => { setTokens(items); setActivityRevision((value) => value + 1); }, (err: unknown) => {
      console.error("list tokens", err);
    });
  }, [step.status]);
  return (
    <div className="library">
      <Nav boot={boot} />
      <main className="lib-main connect">
      <div className="connect-content">
        <h1>{t("connect.title")}</h1>
        <p className="lead">{t(data.oauthEnabled ? "connect.oauth.intro" : "connect.intro")}</p>
        {data.oauthEnabled && <OAuthSetup baseUrl={data.baseUrl} />}
        <details className="connect-manual" open={!data.oauthEnabled}>
          <summary>{t("connect.oauth.manual")}</summary>
        <section className="step" aria-labelledby="step-1">
          <h2 id="step-1">
            <span className="num mono">1</span>
            {t("connect.step1.title")}
          </h2>
          <TokenStep step={step} onStep={setStep} />
        </section>
        <section className="step" aria-labelledby="step-2">
          <h2 id="step-2">
            <span className="num mono">2</span>
            {t("connect.step2.title")}
          </h2>
          {step.status !== "made" && (
            <p className="lead">
              {format(t("connect.step2.waiting"), {
                placeholder: tokenPlaceholder,
              })}
            </p>
          )}
          <ClientTabs baseUrl={data.baseUrl} token={token} />
        </section>
        </details>
        {data.oauthEnabled && <OAuthGrants />}
        <section className="step" aria-labelledby="step-try">
          <h2 id="step-try">{t("connect.try.title")}</h2>
          <Code text={t("connect.try.prompt")} />
        </section>
        <section className="step" id="tokens" aria-labelledby="step-tokens">
          <h2 id="step-tokens">{t("connect.tokens.title")}</h2>
          <TokenList
            items={tokens}
            onActivity={(id) => {
              setActivityToken(id);
              document.getElementById("activity-token")?.focus({ preventScroll: true });
              document.getElementById("recent-activity")?.scrollIntoView({ block: "start" });
            }}
            onRevoked={(id) => {
              setTokens((list) => list.filter((x) => x.id !== id));
              setActivityRevision((value) => value + 1);
            }}
          />
        </section>
        <RecentActivity tokenID={activityToken} onTokenIDChange={setActivityToken} refreshKey={activityRevision} />
      </div>
      </main>
    </div>
  );
}

function TokenStep({
  step,
  onStep,
}: {
  step: Step1;
  onStep: (s: Step1) => void;
}) {
  const t = useT();
  const name = useRef<HTMLInputElement>(null);
  const made = step.status === "made";

  async function submit(e: SyntheticEvent) {
    e.preventDefault();
    onStep({ status: "creating" });
    try {
      onStep({
        status: "made",
        token: await createToken(name.current?.value ?? ""),
      });
    } catch {
      onStep({ status: "naming", failed: true });
    }
  }

  return (
    <>
      <form className="row" onSubmit={(e) => void submit(e)}>
        {/* One field throughout: it takes the name, then becomes the token. */}
        <input
          key="field"
          ref={name}
          name="name"
          readOnly={made}
          {...(made
            ? {
                value: step.token,
                "data-state": "revealed",
                "aria-label": t("connect.step1.token"),
              }
            : {
                placeholder: t("connect.step1.placeholder"),
                "aria-label": t("connect.step1.name"),
                maxLength: 60,
              })}
          autoComplete="off"
          spellCheck={false}
          onFocus={(e) => {
            if (made) e.currentTarget.select();
          }}
        />
        {made ? (
          <CopyButton className="primary" text={() => step.token}>
            {t("copy.label")}
          </CopyButton>
        ) : (
          <button
            className="primary"
            type="submit"
            disabled={step.status === "creating"}
          >
            {step.status === "creating"
              ? t("connect.step1.creating")
              : t("connect.step1.create")}
          </button>
        )}
      </form>
      {step.status === "naming" && step.failed && (
        <p className="error" role="alert">
          {t("connect.step1.failed")}
        </p>
      )}
      <p className="hint">
        {made && <span className="once">{t("connect.step1.once")} </span>}
        <a href="#tokens">{t("connect.step1.manage")}</a>
      </p>
    </>
  );
}

function ClientTabs({ baseUrl, token }: { baseUrl: string; token: string }) {
  const t = useT();
  return <AgentTabs>{active => <>
        {active === "claude" && (
          <>
            <p className="lead">{t("connect.claude.run")}</p>
            <Code text={claudeCommand(baseUrl, token)} />
          </>
        )}
        {active === "codex" && (
          <>
            <p className="lead">{t("connect.codex.run")}</p>
            <Code text={`${codexEnv(token)}\n${codexCommand(baseUrl)}`} />
            <p className="lead">{t("connect.codex.config")}</p>
            <Code text={codexConfig(baseUrl)} />
          </>
        )}
        {active === "other" && (
          <>
            <p className="lead">{t("connect.other.where")}</p>
            <Code text={mcpJson(baseUrl, token)} />
          </>
        )}
    </>}</AgentTabs>;
}

/** The person's tokens by name, with a revoke that asks first. Revoking takes effect on the server at once. */
function TokenList({
  items,
  onRevoked,
  onActivity,
}: {
  items: TokenItem[];
  onRevoked: (id: string) => void;
  onActivity: (id: string) => void;
}) {
  const t = useT();
  const [revoking, setRevoking] = useState<TokenItem | null>(null);

  async function revoke() {
    if (!revoking) return;
    try {
      await revokeToken(revoking.id);
    } catch (err) {
      console.error("revoke token", err);
      throw err;
    }
    onRevoked(revoking.id);
    setRevoking(null);
  }

  return (
    <>
      {items.length === 0 ? (
        <p className="lead">{t("connect.tokens.empty")}</p>
      ) : (
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>{t("connect.tokens.name")}</th>
                <th>{t("connect.tokens.createdAt")}</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {items.map((item) => (
                <tr key={item.id}>
                  <td><button type="button" className="quiet activity-token-link" title={format(t("connect.activity.forToken"), { name: item.name })} aria-label={format(t("connect.activity.forToken"), { name: item.name })} onClick={() => { onActivity(item.id); }}>{item.name}</button></td>
                  <td className="muted mono"><RelativeTime iso={item.createdAt} /></td>
                  <td className="end">
                    <button
                      type="button"
                      className="danger quiet"
                      aria-label={`${t("connect.tokens.revoke")} ${item.name}`}
                      onClick={() => {
                        setRevoking(item);
                      }}
                    >
                      {t("connect.tokens.revoke")}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <ConfirmDialog
        open={revoking !== null}
        onOpenChange={(open) => {
          if (!open) setRevoking(null);
        }}
        title={format(t("connect.tokens.confirm"), {
          name: revoking?.name ?? "",
        })}
        hint={t("connect.tokens.confirmHint")}
        confirm={t("connect.tokens.revoke")}
        failure={t("connect.tokens.failed")}
        danger
        onConfirm={revoke}
      />
    </>
  );
}
