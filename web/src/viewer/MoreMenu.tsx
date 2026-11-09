import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import { useLayoutEffect, useRef, useState, type ReactNode } from "react";
import type { Artifact } from "../api";
import { ActionItems } from "../ActionItems";
import { exportLabel, hasDangerousAction, type ActionEnv } from "../artifactActionList";
import { More, Chevron } from "../icons/menu";
import { Back } from "../icons/chrome";
import { useT } from "../i18n";
import { keepFocusWhereItWent, keepTriggerToggle } from "../menuFocus";

interface Props {
  artifact: Artifact;
  /** What the actions in the list can do here; the list itself is artifactActionList.ts. */
  env: ActionEnv;
  signedIn: boolean;
  /** The formats the Export group lists, in the order the kind declares them; empty when there is nothing to export or the viewer cannot. */
  exports: string[];
  onExport: (format: string) => void;
  secondary?: ReactNode;
  account?: ReactNode;
}

/**
 * The rest of what the viewer can do; the groups marked `phone-only` are hidden
 * by CSS where the bar has them.
 */
// Menus are non-modal on purpose: the modal ones lock scroll by injecting a
// <style> element, which the site CSP (default-src 'self') blocks and logs.
export function MoreMenu({ artifact, env, signedIn, exports, onExport, secondary, account }: Props) {
  const t = useT();
  const [accountView, setAccountView] = useState(false);
  const menu = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  useLayoutEffect(() => { menu.current?.focus(); }, [accountView]);
  const viewer = { signedIn, surface: "viewer" } as const;
  return (
    <DropdownMenu.Root modal={false} onOpenChange={(open) => { if (!open) setAccountView(false); }}>
      <DropdownMenu.Trigger ref={trigger} className="icon-button" aria-label={t("viewer.more")}>
        <More />
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content ref={menu} className={accountView ? "menu viewer-overflow account-menu" : "menu viewer-overflow"} onCloseAutoFocus={keepFocusWhereItWent} onInteractOutside={(e) => { keepTriggerToggle(e, trigger.current); }} align="end" sideOffset={8}>
          {accountView ? <>
            <DropdownMenu.Item className="menu-item" onSelect={(e) => { e.preventDefault(); setAccountView(false); }}><Back />{t("viewer.more")}</DropdownMenu.Item>
            <DropdownMenu.Separator className="menu-sep" />
            {account}
          </> : <>
          <ActionItems artifact={artifact} viewer={viewer} env={env} danger={false} />
          {exports.length > 0 && (
            <DropdownMenu.Group>
              <DropdownMenu.Separator className="menu-sep" />
              <DropdownMenu.Label className="menu-label">{t("export.title")}</DropdownMenu.Label>
              {exports.map((f) => (
                <DropdownMenu.Item
                  key={f}
                  className="menu-item"
                  onSelect={() => {
                    onExport(f);
                  }}
                >
                  {exportLabel(t, f)}
                </DropdownMenu.Item>
              ))}
            </DropdownMenu.Group>
          )}
          {secondary && (
            <DropdownMenu.Group className="viewer-secondary">
              <DropdownMenu.Separator className="menu-sep" />
              {secondary}
            </DropdownMenu.Group>
          )}
          {account && <DropdownMenu.Item className="menu-item viewer-account-link" onSelect={(e) => { e.preventDefault(); setAccountView(true); }}>
            {t("nav.account")}<Chevron />
          </DropdownMenu.Item>}
          {hasDangerousAction(artifact, viewer) && (
            <>
              <DropdownMenu.Separator className="menu-sep" />
              <ActionItems artifact={artifact} viewer={viewer} env={env} danger />
            </>
          )}
          </>}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}
