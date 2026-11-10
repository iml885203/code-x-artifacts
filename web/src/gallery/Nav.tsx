import "./gallery.css";
import { useState, type ReactNode } from "react";
import type { Boot } from "../boot";
import { Brand } from "../Header";
import { AccountMenu } from "../shell/AccountMenu";
import { Clock, Grid, Menu, Plug } from "../icons/nav";
import { useT, type Key } from "../i18n";
import { inApp, useRouter } from "../router";
import { isRecentPath, recentPath } from "./filter";

interface Item {
  label: Key;
  href: string;
  /** The number beside it; 0 shows none. */
  count: number;
  current?: boolean;
  /** One of the library's own pages, which the app shows without loading a page. */
  inApp?: boolean;
  icon: ReactNode;
}

/** The library's left rail: where you can go, and you, at the bottom. */
export function Nav({ boot, artifacts = 0 }: { boot: Boot; artifacts?: number }) {
  const t = useT();
  const router = useRouter();
  // On a phone the links fold behind a button; on a wide screen the button is not drawn and the links always show.
  const [open, setOpen] = useState(false);
  const recent = isRecentPath(router.path);
  const pathname = router.path.split(/[?#]/)[0];
  const inLibraryApp = boot.page === "gallery" || boot.page === "viewer";
  const items: Item[] = [
    { label: "nav.artifacts", href: "/", count: artifacts, current: pathname === "/", inApp: true, icon: <Grid /> },
    { label: "nav.recent", href: recentPath, count: 0, current: recent, inApp: true, icon: <Clock /> },
    { label: "nav.connect", href: "/connect", count: 0, current: pathname === "/connect", icon: <Plug /> },
  ];
  return (
    <nav className="lib-nav" aria-label={t("nav.main")}>
      <Brand />
      <button type="button" className="icon-button lib-nav-toggle" aria-expanded={open} aria-controls="lib-nav-links" aria-label={t("nav.menu")} onClick={() => { setOpen(!open); }}>
        <Menu />
      </button>
      <div id="lib-nav-links" className={open ? "lib-nav-list open" : "lib-nav-list"}>
        {items.map((item) => (
          <a key={item.label} className="lib-nav-item" href={item.href} {...(item.current ? { "aria-current": "page" } : {})} onClick={item.inApp && inLibraryApp ? inApp(router, item.href) : undefined}>
            {item.icon}
            <span className="lib-nav-label">{t(item.label)}</span>
            {item.count > 0 && <span className="count">{item.count}</span>}
          </a>
        ))}
      </div>
      <div className="lib-nav-me">
        <AccountMenu user={boot.user} displayName={boot.displayName ?? ""} initials={boot.initials} langs={boot.langs} rail />
      </div>
    </nav>
  );
}
