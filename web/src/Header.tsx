import type { ReactNode } from "react";
import type { Boot } from "./boot";
import { NewMenu } from "./create/NewMenu";
import { Mark } from "./Mark";
import { AccountMenu } from "./shell/AccountMenu";

export function Brand({ link = true }: { link?: boolean }) {
  const inner = (
    <>
      <Mark />
      <span className="wordmark">CodeXArtifacts</span>
    </>
  );
  return link ? (
    <a className="brand" href="/" aria-label="CodeXArtifacts">
      {inner}
    </a>
  ) : (
    <span className="brand">{inner}</span>
  );
}

/** The bar of every signed-in page but the viewer. `children` is page-specific (the overview's search). */
export function Header({ boot, children }: { boot: Boot; children?: ReactNode }) {
  return (
    <header className="bar">
      <Brand />
      {children}
      <span className="spacer" />
      {boot.user !== "" && <NewMenu />}
      <AccountMenu user={boot.user} displayName={boot.displayName ?? ""} initials={boot.initials} langs={boot.langs} />
    </header>
  );
}
