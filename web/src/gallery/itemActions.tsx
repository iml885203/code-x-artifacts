import { Tooltip } from "../Tooltip";
import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import { useEffect, useRef, useState } from "react";
import { ApiError, type Artifact } from "../api";
import { ActionItems } from "../ActionItems";
import { useArtifacts } from "../artifacts";
import { useArtifactActions } from "../artifactActions";
import { actionForKey, hasDangerousAction, type ActionEnv, type ActionViewer } from "../artifactActionList";
import { renameArtifact } from "../create/api";
import { maxTitleLength } from "../create/RenameTitle";
import { More } from "../icons/menu";
import { Pin, PinFilled } from "../icons/pin";
import { format, useT } from "../i18n";
import { keepFocusWhereItWent } from "../menuFocus";
import { colorClassOf, initialOf } from "../presence/people";
import { useToast } from "../Toast";
import { audienceIcon, audienceLabel, isAudience } from "../viewer/audience";
import { ConfirmDialog } from "../viewer/ConfirmDialog";
import "../presence/presence.css";
import "./gallery.css";

/** The audience icon, with its name as tooltip and accessible name; a visibility this bundle does not know shows as its word. */
export function Audience({ visibility }: { visibility: string }) {
  const t = useT();
  if (!isAudience(visibility)) return <span className="audience">{visibility}</span>;
  const Icon = audienceIcon[visibility];
  const label = t(audienceLabel[visibility]);
  return (
    <Tooltip content={label}><span className={visibility === "private" ? "audience" : "audience open"} role="img" aria-label={label} tabIndex={0}>
      <Icon />
    </span></Tooltip>
  );
}

/** The author's avatar: the same circle and colour the viewers' stack gives that person. */
export function AuthorFace({ name }: { name: string }) {
  return (
    <span className={`pr-face card-face ${colorClassOf({ id: name, name, agent: false, self: false })}`} aria-hidden="true">
      {initialOf(name)}
    </span>
  );
}

/**
 * What a card and a list row share: the pin, the "⋯" menu with its letters, renaming in place and the delete
 * question. The surface draws the pieces where its layout wants them.
 */
export function useItemActions(a: Artifact, beforePin: () => void) {
  const t = useT();
  const toast = useToast();
  const store = useArtifacts();
  const actions = useArtifactActions();
  const [deleting, setDeleting] = useState(false);
  const [renaming, setRenaming] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  // The menu hands focus back to its button when it closes; a rename that the menu started keeps it.
  const keepFocus = useRef(false);

  function togglePin() {
    beforePin();
    actions.togglePin(a, !a.pinned);
  }

  // The library is for people who are signed in; an item has no panels.
  const viewer: ActionViewer = { signedIn: true, surface: "card" };
  const env: ActionEnv = {
    copyLink: (x) => void actions.copyLink(x),
    copy: (x) => void actions.copy(x),
    togglePin,
    openPanel: () => {
      throw new Error("a card has no panels");
    },
    askDelete: () => {
      setDeleting(true);
    },
    rename: () => {
      keepFocus.current = true;
      setRenaming(true);
    },
  };

  async function saveTitle(title: string) {
    setRenaming(false);
    if (title === "" || title === a.title || /[\r\n]/.test(title)) return;
    try {
      store.replace(await renameArtifact(a.id, title));
    } catch (err) {
      console.error("rename the artifact", err);
      toast({ message: err instanceof ApiError && err.reason !== "" ? err.reason : t("create.renameFailed"), error: true });
    }
  }

  const pin = (
    <Tooltip content={t(a.pinned ? "pin.remove" : "pin.add")}><button
      type="button"
      className={a.pinned ? "icon-button item-pin pinned" : "icon-button item-pin"}
      aria-pressed={a.pinned}
      aria-label={format(t(a.pinned ? "card.unpin" : "card.pin"), { title: a.title })}
      onClick={togglePin}
    >
      {a.pinned ? <PinFilled /> : <Pin />}
    </button></Tooltip>
  );

  const menu = (
    <DropdownMenu.Root modal={false} open={menuOpen} onOpenChange={setMenuOpen}>
      <Tooltip content={t("card.menu")} disabled={menuOpen}><DropdownMenu.Trigger className="icon-button item-more" aria-label={format(t("card.actions"), { title: a.title })}>
        <More />
      </DropdownMenu.Trigger></Tooltip>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          className="menu card-menu"
          align="end"
          sideOffset={4}
          onCloseAutoFocus={(e) => {
            if (!keepFocus.current) {
              keepFocusWhereItWent(e);
              return;
            }
            keepFocus.current = false;
            e.preventDefault();
          }}
          onKeyDown={(e) => {
            if (e.metaKey || e.ctrlKey || e.altKey) return;
            const action = actionForKey(a, viewer, e.key);
            if (!action) return;
            // The letter runs the action and closes the menu, as picking its row does.
            e.preventDefault();
            setMenuOpen(false);
            action.run?.(a, env);
          }}
        >
          <ActionItems artifact={a} viewer={viewer} env={env} danger={false} />
          {hasDangerousAction(a, viewer) && (
            <>
              <DropdownMenu.Separator className="menu-sep" />
              <ActionItems artifact={a} viewer={viewer} env={env} danger />
            </>
          )}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );

  // Mounted only while it asks: a dialog per item would be as many hidden headings as items.
  const dialog =
    a.canChange && deleting ? (
      <ConfirmDialog
        open
        onOpenChange={setDeleting}
        title={t("viewer.confirmDelete")}
        hint={t("viewer.confirmDeleteHint")}
        confirm={t("viewer.deleteConfirm")}
        failure={t("viewer.deleteFailed")}
        danger
        onConfirm={() => actions.remove(a)}
      />
    ) : null;

  return { renaming, saveTitle, pin, menu, dialog };
}

/** The card's title as a field: Enter or leaving it keeps the new name, Escape keeps the old one. */
export function RenameField({ title, onDone }: { title: string; onDone: (title: string) => void }) {
  const t = useT();
  const [text, setText] = useState(title);
  const done = useRef(false);
  const field = useRef<HTMLInputElement>(null);
  useEffect(() => {
    field.current?.select();
  }, []);
  const finish = (keep: boolean) => {
    if (done.current) return;
    done.current = true;
    onDone(keep ? text.trim() : title);
  };
  return (
    <input
      className="title-field"
      value={text}
      maxLength={maxTitleLength}
      aria-label={t("create.titleField")}
      ref={field}
      autoFocus
      onChange={(e) => {
        setText(e.target.value);
      }}
      onKeyDown={(e) => {
        if (e.key === "Enter") finish(true);
        if (e.key === "Escape") finish(false);
      }}
      onBlur={() => {
        finish(true);
      }}
    />
  );
}
