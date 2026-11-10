import { authorName } from "./author";
import { Tooltip } from "../Tooltip";
import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import * as ToggleGroup from "@radix-ui/react-toggle-group";
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { useArtifacts } from "../artifacts";
import type { Boot } from "../boot";
import { Easel } from "../Easel";
import { NewMenu } from "../create/NewMenu";
import { Check } from "../icons/chrome";
import { kindsTable } from "../kinds/kinds";
import { Chevron, Grid, List as ListIcon } from "../icons/nav";
import { useT, type Key } from "../i18n";
import { useRouter } from "../router";
import { useDocumentTitle } from "../title";
import { countByFilter, filters, isFilter, isRecentPath, recentlyOpened, visibleArtifacts, type Filter } from "./filter";
import { Card } from "./Card";
import { groupArtifacts, groupChoices, isGroupBy, readGroupBy, readView, views, writeGroupBy, writeView, type DateKey, type Group, type GroupBy, type View } from "./group";
import { Row } from "./List";
import { Nav } from "./Nav";
import "./gallery.css";
import { arrive, canAnimate, fadeOut, placesOf, type Places } from "./wall";

const filterLabel = {
  all: "gallery.filter.all",
  mine: "gallery.filter.mine",
  shared: "gallery.filter.shared",
  pinned: "gallery.filter.pinned",
} as const satisfies Record<Filter, Key>;

const groupLabel = {
  none: "gallery.group.none",
  date: "gallery.group.date",
  owner: "gallery.group.owner",
  kind: "gallery.group.kind",
} as const satisfies Record<GroupBy, Key>;

const viewLabel = { grid: "gallery.view.grid", list: "gallery.view.list" } as const satisfies Record<View, Key>;

const viewIcon = { grid: Grid, list: ListIcon } as const;

const dateLabel = {
  today: "gallery.date.today",
  yesterday: "gallery.date.yesterday",
  week: "gallery.date.week",
  earlier: "gallery.date.earlier",
} as const satisfies Record<DateKey, Key>;

/** The header over a group: the owner's name, or the date or kind named in the catalogue. Not grouped has none. */
function headerOf(g: Group, t: (key: Key) => string): string | null {
  switch (g.by) {
    case "none":
      return null;
    case "owner":
      return g.items[0] === undefined ? g.owner : authorName(g.items[0]);
    case "date":
      return t(dateLabel[g.key]);
    case "kind":
      return t(kindsTable[g.key].label);
  }
}

/** How many plates stand in for the cards while the list loads. */
const PLATES = 6;

export function Gallery({ boot }: { boot: Boot }) {
  const { user } = boot;
  const t = useT();
  const store = useArtifacts();
  const { load, refresh } = store;
  const [filter, setFilter] = useState<Filter>("all");
  // The filter the wall shows trails the one just pressed by the time the
  // cards that drop out take to fade.
  const [applied, setApplied] = useState<Filter>("all");
  const [query, setQuery] = useState("");
  // How the wall is arranged is each person's own, kept in this browser.
  const [groupBy, setGroupBy] = useState<GroupBy>(() => readGroupBy(user));
  const [view, setView] = useState<View>(() => readView(user));
  const search = useRef<HTMLInputElement>(null);
  const grid = useRef<HTMLDivElement>(null);
  const standing = useRef<Places | null>(null);
  const pending = useRef(0);
  // The "Recently opened" page is the same wall over what the person opened, newest open first, without tabs or grouping.
  const recent = isRecentPath(useRouter().path);
  useDocumentTitle(recent ? t("nav.recent") : "");

  useEffect(() => {
    refresh();
  }, [refresh]);

  // "/" jumps to the search, unless a field already has the keyboard.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const target = e.target;
      const typing = target instanceof HTMLElement && (target.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName));
      if (e.key !== "/" || typing || e.metaKey || e.ctrlKey || e.altKey) return;
      e.preventDefault();
      search.current?.focus();
    };
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("keydown", onKey);
    };
  }, []);

  const list = load.status === "ready" ? load.list : [];
  const shown = recent ? recentlyOpened(list, query) : visibleArtifacts(list, applied, query, user);
  const counts = countByFilter(list, user);

  // The wall arrives with a stagger the first time its list appears, and
  // otherwise only moves when a change the person made rearranged it.
  const hadList = useRef(load.status === "ready");
  const groups: Group[] = recent ? (shown.length === 0 ? [] : [{ by: "none", key: "all", items: shown }]) : groupArtifacts(shown, groupBy, new Date(), user);
  // Changing the grouping or the view moves cards without changing which are shown, so they count in the key.
  const shownIds = `${groups.flatMap((g) => g.items.map((a) => a.id)).join(",")}|${groupBy}|${view}`;
  useLayoutEffect(() => {
    const g = grid.current;
    if (!g) return;
    if (standing.current) {
      arrive(g, standing.current);
      standing.current = null;
    } else if (!hadList.current) {
      arrive(g, null);
    }
    hadList.current = true;
  }, [shownIds]);

  function remember() {
    standing.current = grid.current ? placesOf(grid.current) : null;
  }

  function chooseGroup(next: GroupBy) {
    remember();
    setGroupBy(next);
    writeGroupBy(user, next);
  }

  function chooseView(next: View) {
    remember();
    setView(next);
    writeView(user, next);
  }

  function chooseFilter(next: Filter) {
    setFilter(next);
    const mine = ++pending.current;
    const g = grid.current;
    const commit = () => {
      if (mine !== pending.current) return;
      remember();
      setApplied(next);
    };
    if (!g || !canAnimate()) {
      commit();
      return;
    }
    const staying = new Set(visibleArtifacts(list, next, query, user).map((a) => a.id));
    void fadeOut(g, staying).then(commit);
  }

  return (
    <div className="library">
      <Nav boot={boot} artifacts={list.length} />
      <main className="lib-main" aria-busy={load.status === "loading"}>
        <h1 className="visually-hidden">{t(recent ? "nav.recent" : "gallery.title")}</h1>
        <div className="lib-controls">
        <div className="lib-top">
          {list.length > 0 && (
            <input
              ref={search}
              className="lib-search"
              type="search"
              value={query}
              onChange={(e) => {
                remember();
                setQuery(e.target.value);
              }}
              placeholder={t("gallery.searchPlaceholder")}
              aria-label={t("gallery.search")}
            />
          )}
          <span className="spacer" />
          {list.length > 0 && (
            <>
              {!recent && <DropdownMenu.Root modal={false}>
                <DropdownMenu.Trigger className="lib-group-button">
                  {t("gallery.group")}
                  {groupBy !== "none" && <span className="lib-group-current">{t(groupLabel[groupBy])}</span>}
                  <Chevron />
                </DropdownMenu.Trigger>
                <DropdownMenu.Portal>
                  <DropdownMenu.Content className="menu" align="end" sideOffset={8}>
                    <DropdownMenu.RadioGroup
                      value={groupBy}
                      onValueChange={(v) => {
                        if (isGroupBy(v)) chooseGroup(v);
                      }}
                    >
                      {groupChoices.map((g) => (
                        <DropdownMenu.RadioItem key={g} className="menu-item" value={g}>
                          <DropdownMenu.ItemIndicator className="menu-check">
                            <Check />
                          </DropdownMenu.ItemIndicator>
                          {t(groupLabel[g])}
                        </DropdownMenu.RadioItem>
                      ))}
                    </DropdownMenu.RadioGroup>
                  </DropdownMenu.Content>
                </DropdownMenu.Portal>
              </DropdownMenu.Root>}
              <div className="lib-views" role="group" aria-label={t("gallery.view")}>
                {views.map((v) => {
                  const Icon = viewIcon[v];
                  return (
                    <Tooltip key={v} content={t(viewLabel[v])}><button
                      type="button"
                      className="icon-button lib-view"
                      aria-pressed={view === v}
                      aria-label={t(viewLabel[v])}
                      onClick={() => {
                        if (view !== v) chooseView(v);
                      }}
                    >
                      <Icon />
                    </button></Tooltip>
                  );
                })}
              </div>
            </>
          )}
          {user !== "" && <NewMenu />}
        </div>
        {list.length > 0 && !recent && (
          <ToggleGroup.Root
            className="lib-tabs"
            type="single"
            value={filter}
            // Radix reports "" when the pressed item is pressed again; a filter is always chosen.
            onValueChange={(v) => {
              if (isFilter(v)) chooseFilter(v);
            }}
            aria-label={t("gallery.filter")}
          >
            {filters.map((f) => (
              <ToggleGroup.Item key={f} className="lib-tab" value={f}>
                {t(filterLabel[f])}
                <span className="count">{counts[f]}</span>
              </ToggleGroup.Item>
            ))}
          </ToggleGroup.Root>
        )}
        </div>
        {load.status === "failed" && (
          <p className="error" role="alert">
            {t("gallery.loadFailed")}
          </p>
        )}
        {load.status === "loading" && (
          <div className="lib-grid" aria-hidden="true">
            {Array.from({ length: PLATES }, (_, i) => (
              <div key={i} className="card plate">
                <span className="thumb" />
              </div>
            ))}
          </div>
        )}
        {load.status === "ready" && list.length === 0 && (
          <div className="empty">
            <Easel />
            <p>{t("gallery.empty")}</p>
            <a className="btn" href="/connect">
              {t("nav.connect")}
            </a>
          </div>
        )}
        {shown.length > 0 && (
          <div ref={grid} className="lib-wall">
            {groups.map((g) => {
              const header = headerOf(g, t);
              const Item = view === "grid" ? Card : Row;
              return (
                <section key={g.key} className="lib-group" {...(header === null ? {} : { "aria-label": header })}>
                  {header !== null && (
                    <h2 className="lib-group-head" {...(g.by === "kind" ? { "data-kind": g.key, style: { "--kh": `var(--kind-${kindsTable[g.key].token}-fg)` } as React.CSSProperties } : {})}>
                      {header}
                      <span className="count">{g.items.length}</span>
                    </h2>
                  )}
                  <ul className={view === "grid" ? "lib-grid" : "lib-list"}>
                    {g.items.map((a) => (
                      <Item key={a.id} artifact={a} beforePin={remember} />
                    ))}
                  </ul>
                </section>
              );
            })}
          </div>
        )}
        {list.length > 0 && shown.length === 0 && <p className="empty">{t(recent && query.trim() === "" ? "gallery.recentEmpty" : "gallery.noMatch")}</p>}
      </main>
    </div>
  );
}
