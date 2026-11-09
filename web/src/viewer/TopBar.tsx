import { Tooltip } from "../Tooltip";
import { useEffect, useState } from "react";
import type { Artifact, Visibility } from "../api";
import type { ActionEnv } from "../artifactActionList";
import type { Boot } from "../boot";
import { Brand } from "../Header";
import { SaveStateLabel, type SaveState } from "../editor/SaveState";
import { Back } from "../icons/chrome";
import { format, useT } from "../i18n";
import { KindMark } from "../kinds/KindMark";
import { AvatarStack } from "../presence/AvatarStack";
import { useViewingPeople } from "../presence/Viewing";
import { RelativeTime } from "../RelativeTime";
import { AccountItems, AccountMenu } from "../shell/AccountMenu";
import { inApp, useRouter } from "../router";
import { audienceIcon, audienceLabel } from "./audience";
import { CommentsButton } from "./CommentsButton";
import { MoreMenu } from "./MoreMenu";
import { ShareDialog } from "./ShareDialog";
import { TitleMenu } from "./TitleMenu";
import { ViewerSecondary } from "./ViewerSecondary";
import "./viewer-bar.css";

interface Props {
  artifact: Artifact;
  boot: Boot;
  visibility: Visibility;
  commentsOpen: boolean;
  onComments: () => void;
  /** The artifact as the server has it after a rename or access change. */
  onChanged: (saved: Artifact) => void;
  onVisibility: (v: Visibility) => Promise<boolean>;
  /** What the actions in the menus can do here. */
  env: ActionEnv;
  exports: string[];
  onExport: (format: string) => void;
  /** What a doc open in the editor says about being saved, beside the version; null for the other kinds. */
  saveState: SaveState | null;
}

/** Whether the window is as narrow as a phone's (false where the browser cannot say). */
function useNarrow(): boolean {
  const query = "(max-width: 640px)";
  const [narrow, setNarrow] = useState(() => typeof window.matchMedia === "function" && window.matchMedia(query).matches);
  useEffect(() => {
    if (typeof window.matchMedia !== "function") return;
    const list = window.matchMedia(query);
    const update = () => {
      setNarrow(list.matches);
    };
    list.addEventListener("change", update);
    return () => {
      list.removeEventListener("change", update);
    };
  }, []);
  return narrow;
}

/**
 * The bar across the top of the viewer, the same for every kind and for everyone who opens it: what it is, its
 * title and who made it, then who is here, the comments, Share and the account. What a person may not do is not
 * there to press; the server still refuses it.
 */
export function TopBar({ artifact, boot, visibility, commentsOpen, onComments, onChanged, onVisibility, env, exports, onExport, saveState }: Props) {
  const t = useT();
  const router = useRouter();
  const signedIn = boot.user !== "";
  const [sharing, setSharing] = useState(false);
  // Phone identity and preferences fold into overflow; presence still sends its heartbeat.
  const narrow = useNarrow();
  const people = useViewingPeople(artifact.id, signedIn);
  const Audience = audienceIcon[visibility];
  return (
    <>
      <header className="vbar">
        {signedIn ? (
          <a className="back icon-link" href="/" aria-label={t("nav.backToArtifacts")} onClick={inApp(router, "/")}>
            <Back />
          </a>
        ) : (
          <Brand link={false} />
        )}
        <KindMark kind={artifact.kind} size="bar" />
        <div className="vtitle">
          <TitleMenu artifact={artifact} signedIn={signedIn} env={env} onRenamed={onChanged} />
          <p className="facts">
            <Tooltip key={visibility} content={t(audienceLabel[visibility])}><span className="audience fade-in" role="img" aria-label={t(audienceLabel[visibility])} tabIndex={0}>
              <Audience />
            </span></Tooltip>
            <span>{artifact.author}</span>
            <RelativeTime iso={artifact.updatedAt} sentence="time.edited" />
            <span>v{artifact.version}</span>
            {saveState !== null && <SaveStateLabel state={saveState} />}
            {artifact.copiedFrom && (
              <span>
                {artifact.copiedFrom.id === "" ? (
                  format(t("viewer.copiedFrom"), { title: artifact.copiedFrom.title })
                ) : (
                  <a href={`/a/${artifact.copiedFrom.id}`} onClick={inApp(router, `/a/${artifact.copiedFrom.id}`)}>
                    {format(t("viewer.copiedFrom"), { title: artifact.copiedFrom.title })}
                  </a>
                )}
              </span>
            )}
          </p>
        </div>
        {!narrow && <AvatarStack people={people} />}
        {artifact.canComment && (
          <div className="desktop-only">
            <CommentsButton summary={artifact.comments} open={commentsOpen} onClick={onComments} />
          </div>
        )}
        <button
          type="button"
          className="primary share-button"
          onClick={() => {
            setSharing(true);
          }}
        >
          {t("share.button")}
        </button>
        <MoreMenu
          artifact={artifact}
          env={env}
          signedIn={signedIn}
          exports={exports}
          onExport={onExport}
          secondary={narrow ? <ViewerSecondary people={people} /> : undefined}
          account={narrow ? <AccountItems user={boot.user} displayName={boot.displayName ?? ""} langs={boot.langs} /> : undefined}
        />
        {!narrow && <AccountMenu user={boot.user} displayName={boot.displayName ?? ""} initials={boot.initials} langs={boot.langs} />}
      </header>
      <ShareDialog artifact={artifact} visibility={visibility} publicLinks={boot.publicLinks} open={sharing} onOpenChange={setSharing} onVisibility={onVisibility} onChanged={onChanged} />
    </>
  );
}
