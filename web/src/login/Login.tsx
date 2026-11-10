import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import { Check } from "../icons/chrome";
import { Chevron } from "../icons/nav";
import { keepFocusWhereItWent } from "../menuFocus";
import type { Boot } from "../boot";
import { Brand } from "../Header";
import { Warning } from "../icons/chrome";
import { format, useT, type Key } from "../i18n";
import { useDocumentTitle } from "../title";
import { LoginShowcase } from "./LoginShowcase";
import "./login.css";

/** Sign in with the configured mode; provider navigation begins only after an explicit action. */
export function Login({ boot }: { boot: Boot }) {
  const t = useT();
  const login = boot.login;
  if (!login) throw new Error("the sign-in page has no login data");
  useDocumentTitle(t("login.title"));
  // The server cannot say which of the two was wrong, so the error sits under the pair and both are marked; the account name stays typed.
  const failed = login.error !== "";
  const invalid = failed ? { "aria-invalid": true, "aria-describedby": "login-error" } : {};

  return (
    <div className="signin">
      <div className="signin-stage">
        <section className="signin-hero" aria-labelledby="workspace-intro">
          <LoginShowcase animate={!failed} />
          <h2 id="workspace-intro">{t("login.headline")}</h2>
          <p>{t("login.description")}</p>
          <div className="signin-formats" aria-label={t("login.formats")}><span>Web</span><span>Docs</span><span>Design</span><span>Board</span><span>Slides</span></div>
        </section>
        <div className="signin-account">
          <Brand link={false} />
          {login.oidc ? (
            <section className="signin-panel" aria-labelledby="signin-title" aria-describedby="signin-help">
              <h1 id="signin-title">{t("login.title")}</h1>
              <p id="signin-help" className="signin-help">{t("login.oidc.help")}</p>
              {failed && <LoginError error={login.error} />}
              <a className="btn primary signin-provider" href={login.oidc.startUrl} aria-describedby={failed ? "login-error" : undefined}>
                {format(t(failed ? "login.oidc.retry" : "login.oidc.continue"), { provider: login.oidc.providerName })}
              </a>
            </section>
          ) : (
            <form className="signin-panel" method="post" action="/login" aria-labelledby="signin-title" aria-describedby="signin-help">
              <input type="hidden" name="next" value={login.next} />
              <h1 id="signin-title">{t("login.title")}</h1>
              <div className="signin-field">
                <label htmlFor="username">{t("login.username")}</label>
                <input id="username" name="username" autoComplete="username" defaultValue={login.username} autoFocus={!failed} required {...invalid} />
              </div>
              <div className="signin-field">
                <label htmlFor="password">{t("login.password")}</label>
                <input id="password" name="password" type="password" autoComplete="current-password" autoFocus={failed} required {...invalid} />
                {failed && <LoginError error={login.error} />}
              </div>
              <button className="primary" type="submit">{t("login.submit")}</button>
              <p id="signin-help" className="signin-help">{t("login.help")}</p>
            </form>
          )}
        </div>
        <form id="signin-language" className="signin-tools" method="post" action="/lang">
          <input type="hidden" name="next" value={boot.here} />
          <DropdownMenu.Root modal={false}>
            <DropdownMenu.Trigger className="signin-language" aria-label={`${t("lang.label")}: ${t(`shell.language.${boot.lang}` as Key)}`}>
              {t(`shell.language.${boot.lang}` as Key)}<Chevron />
            </DropdownMenu.Trigger>
            <DropdownMenu.Portal>
              <DropdownMenu.Content className="menu" side="bottom" align="end" sideOffset={8} collisionPadding={12} onCloseAutoFocus={keepFocusWhereItWent} aria-label={t("lang.label")}>
                <DropdownMenu.RadioGroup value={boot.lang}>
                  {boot.langs.map((tag) => (
                    <DropdownMenu.RadioItem key={tag} className="menu-item" value={tag} asChild>
                      <button type="submit" form="signin-language" name="lang" value={tag}>
                        <DropdownMenu.ItemIndicator className="menu-check"><Check /></DropdownMenu.ItemIndicator>
                        {t(`shell.language.${tag}` as Key)}
                      </button>
                    </DropdownMenu.RadioItem>
                  ))}
                </DropdownMenu.RadioGroup>
              </DropdownMenu.Content>
            </DropdownMenu.Portal>
          </DropdownMenu.Root>
        </form>
      </div>
    </div>
  );
}


function LoginError({ error }: { error: string }) {
  const t = useT();
  return <p id="login-error" className="field-error" role="alert"><Warning />{t(error as Key)}</p>;
}
