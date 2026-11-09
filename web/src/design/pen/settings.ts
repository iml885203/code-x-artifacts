import { useState } from "react";
import { DEFAULT_PEN, type PenStyle } from "./stroke";
export interface PenSettings { style: PenStyle; onChange: (style:PenStyle)=>void }
/** Keep one settings object in the canvas and pass it to both the toolbar and PenDraw. */
export function usePenSettings(): PenSettings {
 const [style,onChange]=useState<PenStyle>(DEFAULT_PEN);
 return {style,onChange};
}
