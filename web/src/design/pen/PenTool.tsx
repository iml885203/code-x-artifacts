import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import { Tooltip } from "../../Tooltip";
import { useRef, useState } from "react";
import { useT } from "../../i18n";
import { keepFocusWhereItWent, keepTriggerToggle } from "../../menuFocus";
import { Chevron, PenIcon } from "../toolbar/icons";
import type { Tool } from "../toolbar/tools";
import type { PenSettings } from "./settings";
import { DEFAULT_PEN, validPen } from "./stroke";
import "./pen.css";

export function PenTool({tool,onTool,settings}:{tool:Tool;onTool:(tool:Tool)=>void;settings?:PenSettings}) {
 const t=useT(),trigger=useRef<HTMLButtonElement>(null);
 const [draft,setDraft]=useState({color:settings?.style.color??DEFAULT_PEN.color,width:String(settings?.style.width??DEFAULT_PEN.width)});
 const [invalid,setInvalid]=useState(false);
 const change=(next:typeof draft)=>{
  setDraft(next);const style={color:next.color,width:Number(next.width)};
  const ok=validPen(style);setInvalid(!ok);if(ok)settings?.onChange(style);
 };
 return <DropdownMenu.Root modal={false} onOpenChange={open=>{if(open)onTool("pen");}}>
  <Tooltip content={`${t("pen.tool")} (P)`}><DropdownMenu.Trigger ref={trigger} className="tool tool-wide" aria-label={t("pen.tool")} aria-pressed={tool==="pen"}><PenIcon/><Chevron/></DropdownMenu.Trigger></Tooltip>
  <DropdownMenu.Portal><DropdownMenu.Content className="menu bar-menu" aria-label={t("pen.settings")} align="start" sideOffset={8} collisionPadding={16} onCloseAutoFocus={keepFocusWhereItWent} onInteractOutside={e=>{keepTriggerToggle(e,trigger.current);}}>
   <div className="pen-fields" onKeyDown={e=>{if(e.key!=="Escape"&&e.key!=="Tab")e.stopPropagation();}}>
    <label>{t("pen.color")}<input value={draft.color} disabled={!settings} aria-invalid={invalid} onChange={e=>{change({...draft,color:e.target.value});}} spellCheck={false}/></label>
    <label>{t("pen.width")}<input type="number" min="0.5" max="64" step="0.5" value={draft.width} disabled={!settings} aria-invalid={invalid} onChange={e=>{change({...draft,width:e.target.value});}}/></label>
    {invalid&&<p className="pen-error" role="alert">{t("pen.invalid")}</p>}
   </div>
  </DropdownMenu.Content></DropdownMenu.Portal>
 </DropdownMenu.Root>;
}
