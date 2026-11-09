import { useState } from "react";
import { useT, type Key } from "../../i18n";
import type { Tokens } from "../tokens";
import { Field, type ColorChange } from "./controls";
import { TokenColor } from "./TokenColor";
import { parseColor, toLength } from "./css";
import { readBorderSides, type BorderSide, type BorderPart, type Computed } from "./model";

export type BorderScope = BorderSide | "all";
export function BorderControls({computed,inline,tokens,onEdit,onColor,onInvalid}: {
  computed: Computed; inline:[string,string][]; tokens:Tokens;
  onEdit:(scope:BorderScope,part:BorderPart,value:string)=>void;
  onColor:(scope:BorderScope,change:ColorChange)=>void; onInvalid:()=>void;
}) {
  const t=useT();
  const [scope,setScope]=useState<BorderScope>("all");
  const border=readBorderSides(computed);
  const selected=border.sides.find(s => s.side===scope) ?? border.sides[0];
  if (!selected) return null;
  const mixed=(part:BorderPart) => scope === "all" && border.mixed[part];
  const sideLabels: Record<BorderSide,Key>={top:"props.sideTop",right:"props.sideRight",bottom:"props.sideBottom",left:"props.sideLeft"};
  const color=parseColor(selected.color) ?? {r:0,g:0,b:0,a:1};
  const style=mixed("style") ? "" : selected.style;
  const styles=["none","solid","dashed","dotted"];
  return <div className="prop-border" role="group" aria-label={t("props.borderSides")}>
    <select aria-label={t("props.borderSides")} value={scope} onChange={event => { setScope(event.target.value as BorderScope); }}>
      <option value="all">{t("props.allSides")}</option>{border.sides.map(side => <option value={side.side} key={side.side}>{t(sideLabels[side.side])}</option>)}
    </select>
    <div className="prop-row">
      <Field label={t("props.borderWidth")} value={mixed("width") ? "" : selected.width} placeholder={mixed("width") ? t("props.mixed") : undefined} onCommit={value => { const css=toLength(value); if (css===null || css.startsWith("-")) { onInvalid(); return false; } onEdit(scope,"width",css); return true; }} />
      <select aria-label={t("props.borderStyle")} value={style} onChange={event => { onEdit(scope,"style",event.target.value); }}>
        {mixed("style") && <option value="" disabled>{t("props.mixed")}</option>}
        {!["",...styles].includes(style) && <option value={style}>{style}</option>}
        {styles.map(value => <option value={value} key={value}>{value === "none" ? t("props.styleNone") : value === "solid" ? t("props.styleSolid") : value === "dashed" ? t("props.styleDashed") : value === "dotted" ? t("props.styleDotted") : value}</option>)}
      </select>
    </div>
    <TokenColor label={t("props.borderColor")} color={color} mixed={mixed("color")} source={mixed("color") ? undefined : inline.find(([name]) => name === `border-${selected.side}-color`)?.[1] ?? inline.find(([name]) => name === "border-color")?.[1]} tokens={tokens} onColor={change => { onColor(scope,change); }} onToken={value => { onEdit(scope,"color",value); }} />
  </div>;
}
