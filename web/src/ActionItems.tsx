import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import type { Artifact } from "./api";
import { visibleActions, type ActionEnv, type ActionViewer, type ArtifactAction } from "./artifactActionList";
import { useT, type Translate } from "./i18n";
import { Arrow } from "./icons/chrome";
import { CopyIcon } from "./icons/copy";
import { Icon } from "./icons/Icon";
import { LinkIcon } from "./icons/link";
import { Pin } from "./icons/pin";
import "./actionItems.css";

const actionPaths: Record<string, string> = {
  rename: "m3 10 7-7 3 3-7 7-4 1zM9 4l3 3",
  delete: "M2.5 4.5h11M6 2.5h4M4 4.5l.5 9h7l.5-9M6.5 7v4M9.5 7v4",
  versions: "M3 5V2.5M3 5h2.5M3 5a5.5 5.5 0 1 1-1 5M8 4.5V8l2.5 1.5",
  files: "M2 4h4l1.5 1.5H14v7H2z",
  editors: "M6 7a2 2 0 1 0 0-4 2 2 0 0 0 0 4M2 13v-1a4 4 0 0 1 8 0v1M11 3a2 2 0 0 1 0 4M12 9a3 3 0 0 1 2 3v1",
  comments: "M2 3h12v8H7l-4 3v-3H2z",
};

/** Decorative symbols follow the same action ids and ordering as the shared menu registry. */
function ActionIcon({ id }: { id: string }) {
  if (id === "pin") return <Pin />;
  if (id === "copy") return <CopyIcon />;
  if (id === "copyLink") return <LinkIcon />;
  if (id === "openRaw") return <Arrow />;
  const path = actionPaths[id];
  return path === undefined ? null : <Icon><path d={path} /></Icon>;
}

interface Props {
  artifact: Artifact;
  viewer: ActionViewer;
  env: ActionEnv;
  /** The dangerous actions or all the others: a menu sets the first apart. */
  danger: boolean;
}

function Item({ action, artifact, env, t, letters }: { action: ArtifactAction; artifact: Artifact; env: ActionEnv; t: Translate; letters: boolean }) {
  const label = action.label(artifact, t);
  const className = action.danger === true ? "menu-item menu-action danger" : "menu-item menu-action";
  const letter = letters && action.key !== undefined ? <kbd className="menu-key" aria-hidden="true">{action.key}</kbd> : null;
  const item =
    action.href !== undefined ? (
      <DropdownMenu.Item asChild>
        <a className={className} href={action.href(artifact)} target="_blank" rel="noopener">
          <ActionIcon id={action.id} />
          {label}
          {letter}
        </a>
      </DropdownMenu.Item>
    ) : (
      <DropdownMenu.Item
        className={className}
        onSelect={() => {
          action.run?.(artifact, env);
        }}
      >
        <ActionIcon id={action.id} />
        {label}
        {letter}
      </DropdownMenu.Item>
    );
  return action.phoneOnly === true ? <DropdownMenu.Group className="phone-only">{item}</DropdownMenu.Group> : item;
}

/** The menu entries of the actions on offer: one list, whichever menu shows it. A card's menu shows each action's letter. */
export function ActionItems({ artifact, viewer, env, danger }: Props) {
  const t = useT();
  const actions = visibleActions(artifact, viewer).filter((a) => (a.danger === true) === danger);
  return actions.map((action) => <Item key={action.id} action={action} artifact={artifact} env={env} t={t} letters={viewer.surface === "card"} />);
}
