import { Tooltip } from "../../Tooltip";
import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import { useEffect, useRef, useState } from "react";
import { Check } from "../../icons/chrome";
import { format, useT } from "../../i18n";
import { keepFocusWhereItWent, keepTriggerToggle } from "../../menuFocus";
import type { DesignPage } from "../manifest";
import type { Outcome } from "../pages/rules";
import type { PageStructure } from "../pages/structure";
import { refusalText } from "../pages/words";
import { pageLink } from "../pages/pageLinks";
import { Chevron, MoreIcon } from "./icons";

export interface PageMenuProps {
  pages: DesignPage[];
  active: string;
  onSelect: (id: string) => void;
  /** Null for someone who may not change the design: they only switch pages. */
  structure: PageStructure | null;
}

/** What the open page is being asked, apart from being shown: a new name, or whether to delete it. */
type Asking = "none" | "name" | "delete";

export function PageMenu({ pages, active, onSelect, structure }: PageMenuProps) {
  const t = useT();
  const [asking, setAsked] = useState<Asking>("none");
  const askingNow = useRef<Asking>("none");
  const setAsking = (next: Asking) => { askingNow.current = next; setAsked(next); };
  const [actionPage, setActionPage] = useState<string | null>(null);
  const [problem, setProblem] = useState("");
  const cancelled = useRef(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const previousAsk = useRef<Asking>("none");
  useEffect(() => {
    if (asking === "none" && previousAsk.current !== "none") trigger.current?.focus();
    previousAsk.current = asking;
  }, [asking]);
  const managed = pages.find((p) => p.id === actionPage);
  if (asking !== "none" && !managed) setAsked("none");
  useEffect(() => {
    if (!managed) askingNow.current = "none";
  }, [managed]);
  const current = pages.find((p) => p.id === active) ?? pages[0];
  if (!current) return null;

  const target = managed;
  const done = (outcome: Outcome) => {
    setProblem(refusalText(t, outcome) ?? "");
    return outcome;
  };
  const add = () => {
    if (!structure) return;
    const out = done(structure.addPage({ page: (n) => format(t("structure.newPage"), { n: String(n) }), frame: (n) => format(t("structure.newFrame"), { n: String(n) }) }));
    if (out.kind === "changed" && out.page) onSelect(out.page);
  };
  const remove = () => {
    if (!structure || !target) return;
    const at = pages.findIndex((p) => p.id === target.id);
    const next = pages[at + 1] ?? pages[at - 1];
    setAsking("none");
    const out = done(structure.deletePage(target.id));
    if (out.kind === "changed" && next && current.id === target.id) onSelect(next.id);
  };
  const frames = target?.rows.flat().length ?? 0;
  const label = format(t("toolbar.page"), { name: current.name });
  return (
    <>
      <div className="bar-page">
        {asking === "name" && structure && target ? (
          <input
            className="ptab-name"
            aria-label={format(t("structure.renamePage"), { name: target.name })}
            defaultValue={target.name}
            maxLength={200}
            ref={(el) => el?.focus()}
            onKeyDown={(e) => {
              if (e.key === "Enter") e.currentTarget.blur();
              else if (e.key === "Escape") {
                cancelled.current = true;
                e.currentTarget.blur();
              }
            }}
            onBlur={(e) => {
              if (!cancelled.current) done(structure.renamePage(target.id, e.currentTarget.value));
              cancelled.current = false;
              setAsking("none");
            }}
          />
        ) : (
          <DropdownMenu.Root modal={false}>
            <Tooltip content={label}><DropdownMenu.Trigger ref={trigger} className="tool tool-wide" aria-label={label}>
              <span className="tool-name">{current.name}</span>
              <Chevron />
            </DropdownMenu.Trigger></Tooltip>
            <DropdownMenu.Portal>
              <DropdownMenu.Content onInteractOutside={(e) => { keepTriggerToggle(e, trigger.current); }} className="menu bar-menu" onCloseAutoFocus={(e) => {
                if (askingNow.current !== "none") { e.preventDefault(); setAsked(askingNow.current); }
                else keepFocusWhereItWent(e);
              }} align="start" sideOffset={8}>
                <DropdownMenu.RadioGroup value={current.id} onValueChange={onSelect}>
                  {pages.map((p) => (
                    <div key={p.id} className="page-menu-row">
                    <DropdownMenu.RadioItem className="menu-item" value={p.id}>
                      <DropdownMenu.ItemIndicator className="menu-check"><Check /></DropdownMenu.ItemIndicator><span className="page-menu-name">{p.name}</span>
                    </DropdownMenu.RadioItem>
                    <DropdownMenu.Sub>
                      <DropdownMenu.SubTrigger className="menu-item page-more" aria-label={format(t("toolbar.pageMore"), { name: p.name })}><MoreIcon /></DropdownMenu.SubTrigger>
                      <DropdownMenu.Portal><DropdownMenu.SubContent className="menu bar-menu" sideOffset={4}>
                        <DropdownMenu.Item className="menu-item" onSelect={() => {
                          void (async () => { try { await navigator.clipboard.writeText(pageLink(window.location.href, p.id)); } catch { setProblem(t("copy.failed")); } })();
                        }}>{t("toolbar.copyPageLink")}</DropdownMenu.Item>
                        {structure && <>
                          <DropdownMenu.Item className="menu-item" onSelect={() => { setActionPage(p.id); askingNow.current = "name"; }}>{t("toolbar.renamePage")}</DropdownMenu.Item>
                          <DropdownMenu.Item className="menu-item danger" disabled={pages.length < 2} aria-describedby={pages.length < 2 ? "page-last-reason" : undefined} onSelect={() => { setActionPage(p.id); askingNow.current = "delete"; }}>{t("toolbar.deletePage")}</DropdownMenu.Item>
                          {pages.length < 2 && <p id="page-last-reason" className="menu-label">{t("structure.ruleLastPage")}</p>}
                        </>}
                      </DropdownMenu.SubContent></DropdownMenu.Portal>
                    </DropdownMenu.Sub>
                    </div>
                  ))}
                </DropdownMenu.RadioGroup>
                {structure && (
                  <>
                    <DropdownMenu.Separator className="menu-sep" />
                    <DropdownMenu.Item className="menu-item" onSelect={add}>
                      {t("structure.addPage")}
                    </DropdownMenu.Item>

                  </>
                )}
              </DropdownMenu.Content>
            </DropdownMenu.Portal>
          </DropdownMenu.Root>
        )}
        {asking === "delete" && target && (
          <div
            className="pconfirm"
            role="alertdialog"
            aria-label={format(t("structure.deletePage"), { name: target.name })}
            onKeyDown={(e) => {
              if (e.key === "Escape") setAsking("none");
            }}
          >
            <p>{frames === 1 ? format(t("structure.confirmOne"), { name: target.name }) : format(t("structure.confirmMany"), { name: target.name, count: String(frames) })}</p>
            <div className="pconfirm-actions">
              <button type="button" onClick={() => { setAsking("none"); }} ref={(el) => el?.focus()}>
                {t("structure.cancel")}
              </button>
              <button type="button" className="danger" onClick={remove}>
                {t("structure.confirm")}
              </button>
            </div>
          </div>
        )}
        {problem && (
          <p className="pproblem" role="alert">
            {problem}
          </p>
        )}
      </div>
      <span className="bar-sep" />
    </>
  );
}
