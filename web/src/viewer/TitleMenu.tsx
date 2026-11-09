import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import { useRef, useState } from "react";
import type { Artifact } from "../api";
import { ActionItems } from "../ActionItems";
import { hasDangerousAction, type ActionEnv } from "../artifactActionList";
import { RenameTitle } from "../create/RenameTitle";
import { Chevron } from "../icons/menu";
import { useT } from "../i18n";
import { keepTriggerToggle } from "../menuFocus";

interface Props {
  artifact: Artifact;
  signedIn: boolean;
  env: ActionEnv;
  /** The artifact as the server has it after a rename. */
  onRenamed: (saved: Artifact) => void;
}

/**
 * The title, which opens what belongs to this one page: its versions, its link and deleting it, and renaming
 * for those who may edit it. Someone who may do none of that still has the link.
 */
export function TitleMenu({ artifact, signedIn, env, onRenamed }: Props) {
  const t = useT();
  const [renaming, setRenaming] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const viewer = { signedIn, surface: "title" } as const;
  if (renaming) {
    return (
      <RenameTitle
        artifact={artifact}
        onRenamed={onRenamed}
        begun
        onFinished={() => {
          setRenaming(false);
        }}
      />
    );
  }
  return (
    <h1 className="title-menu">
      <DropdownMenu.Root modal={false}>
        <DropdownMenu.Trigger ref={trigger} className="title-button">
          <span className="title-text">{artifact.title}</span>
          <Chevron />
        </DropdownMenu.Trigger>
        <DropdownMenu.Portal>
          <DropdownMenu.Content className="menu" onInteractOutside={(e) => { keepTriggerToggle(e, trigger.current); }} align="start" sideOffset={8}>
            {artifact.canEdit && (
              <DropdownMenu.Item
                className="menu-item"
                onSelect={() => {
                  setRenaming(true);
                }}
              >
                {t("create.rename")}
              </DropdownMenu.Item>
            )}
            <ActionItems artifact={artifact} viewer={viewer} env={env} danger={false} />
            {hasDangerousAction(artifact, viewer) && (
              <>
                <DropdownMenu.Separator className="menu-sep" />
                <ActionItems artifact={artifact} viewer={viewer} env={env} danger />
              </>
            )}
          </DropdownMenu.Content>
        </DropdownMenu.Portal>
      </DropdownMenu.Root>
    </h1>
  );
}
