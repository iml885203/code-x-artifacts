import { Tooltip } from "../Tooltip";
import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import type { Boot } from "../boot";
import { Chevron } from "../icons/nav";
import { Check } from "../icons/chrome";
import { useLang, useT, type Key } from "../i18n";
import { keepFocusWhereItWent } from "../menuFocus";
import { useToast } from "../Toast";
import "./shell.css";
import { crossfade } from "./crossfade";
import { useChangeLanguage } from "./language";
import { themeChoices, useThemeChoice, type ThemeChoice } from "./theme";

const themeLabels: Record<ThemeChoice, Key> = { system: "shell.theme.system", light: "shell.theme.light", dark: "shell.theme.dark" };

/**
 * The avatar menu: who you are, the language, the theme, sign out. Language
 * and theme live here and nowhere else, and each takes effect at once, without
 * a reload; the choice is remembered (language by the server, theme by this
 * browser).
 */
export function AccountItems({ user, langs, displayName }: Pick<Boot, "user" | "langs" | "displayName">) {
  const t = useT();
  const lang = useLang();
  const toast = useToast();
  const changeLanguage = useChangeLanguage();
  const [theme, setTheme] = useThemeChoice();
  const signedIn = user !== "";
  const name = displayName === undefined || displayName === "" ? user : displayName;

  const pickLanguage = (tag: string) => {
    if (tag === lang) return;
    changeLanguage(tag).then(
      (ok) => {
        if (!ok) toast({ message: t("shell.languageFailed"), error: true });
      },
      () => { toast({ message: t("shell.languageFailed"), error: true }); },
    );
  };

  return (
    <>
      {signedIn && <DropdownMenu.Label className="menu-label account-identity">{name}{displayName && displayName !== user && <span className="account-handle">{user}</span>}</DropdownMenu.Label>}
      {signedIn && <DropdownMenu.Separator className="menu-sep" />}
      <DropdownMenu.Label className="menu-label">{t("shell.language")}</DropdownMenu.Label>
      <DropdownMenu.RadioGroup value={lang} onValueChange={pickLanguage}>
        {langs.map((tag) => (
          <DropdownMenu.RadioItem key={tag} className="menu-item" value={tag}>
            <DropdownMenu.ItemIndicator className="menu-check">
              <Check />
            </DropdownMenu.ItemIndicator>
            {t(`shell.language.${tag}` as Key)}
          </DropdownMenu.RadioItem>
        ))}
      </DropdownMenu.RadioGroup>
      <DropdownMenu.Separator className="menu-sep" />
      <DropdownMenu.Label className="menu-label">{t("shell.theme")}</DropdownMenu.Label>
      <DropdownMenu.RadioGroup value={theme} onValueChange={(v) => { crossfade(() => { setTheme(v as ThemeChoice); }); }}>
        {themeChoices.map((choice) => (
          <DropdownMenu.RadioItem key={choice} className="menu-item" value={choice}>
            <DropdownMenu.ItemIndicator className="menu-check">
              <Check />
            </DropdownMenu.ItemIndicator>
            {t(themeLabels[choice])}
          </DropdownMenu.RadioItem>
        ))}
      </DropdownMenu.RadioGroup>
      {signedIn && (
        <>
          <DropdownMenu.Separator className="menu-sep" />
          <DropdownMenu.Item asChild>
            <a className="menu-item" href="/connect">
              {t("nav.connect")}
            </a>
          </DropdownMenu.Item>
          <form method="post" action="/logout">
            <DropdownMenu.Item asChild>
              <button className="menu-item" type="submit">
                {t("nav.signOut")}
              </button>
            </DropdownMenu.Item>
          </form>
        </>
      )}
    </>
  );
}

/** The same account controls can live in the viewer overflow on a phone. */
export function AccountMenu(props: Pick<Boot, "user" | "initials" | "langs" | "displayName"> & { rail?: boolean }) {
  const t = useT();
  const name = props.displayName === undefined || props.displayName === "" ? props.user : props.displayName;
  return (
    <DropdownMenu.Root modal={false}>
      <Tooltip content={name}><DropdownMenu.Trigger className={props.rail ? "account-trigger" : "initials account-avatar"} aria-label={props.rail ? `${t("nav.account")}: ${name}` : t("nav.account")}>
        {props.rail ? <><span className="initials account-avatar" aria-hidden="true">{props.initials}</span><span className="lib-account-name">{name}</span><Chevron /></> : props.initials}
      </DropdownMenu.Trigger></Tooltip>
      <DropdownMenu.Portal>
        <DropdownMenu.Content className="menu account-menu" onCloseAutoFocus={keepFocusWhereItWent} align={props.rail ? "start" : "end"} alignOffset={props.rail ? 12 : 0} side={props.rail ? "top" : "bottom"} sideOffset={8} collisionPadding={12}>
          <AccountItems user={props.user} langs={props.langs} displayName={props.displayName ?? ""} />
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}
