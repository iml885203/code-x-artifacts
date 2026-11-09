import { useState } from "react";
import { useT } from "../../i18n";
import type { DesignFrame } from "../manifest";
import { PRESETS } from "./rules";
import type { PageStructure } from "./structure";
import { refusalText } from "./words";

export function ArtboardProperties({ frame, structure }: { frame: DesignFrame; structure: PageStructure | null }) {
  const t = useT();
  const [problem, setProblem] = useState("");
  const write = (w: number, h: number) => {
    if (structure) setProblem(refusalText(t, structure.resizeFrame(frame.id, { w, h })) ?? "");
  };
  return <section className="artboard-properties" aria-label={frame.name}>
    <h3>{frame.name}</h3>
    <div className="fsize">
      {(["w", "h"] as const).map((side) => <label key={side}>
        <span>{t(side === "w" ? "structure.width" : "structure.height")}</span>
        <input key={`${frame.id}-${side}-${String(frame[side])}`} aria-label={t(side === "w" ? "structure.width" : "structure.height")} defaultValue={String(frame[side])} disabled={!structure} inputMode="numeric" onKeyDown={(event) => { if (event.key === "Enter") event.currentTarget.blur(); }} onBlur={(event) => {
          const value = event.currentTarget.value.trim() === "" ? NaN : Number(event.currentTarget.value);
          if (value !== frame[side]) write(side === "w" ? value : frame.w, side === "h" ? value : frame.h);
        }} />
      </label>)}
      {structure && <select aria-label={t("structure.preset")} value={PRESETS.find((preset) => preset.w === frame.w && preset.h === frame.h)?.id ?? ""} onChange={(event) => {
        const preset = PRESETS.find((value) => value.id === event.currentTarget.value);
        if (preset) write(preset.w, preset.h);
      }}><option value="" disabled>{t("structure.presetNone")}</option>{PRESETS.map((preset) => <option key={preset.id} value={preset.id}>{preset.id} · {preset.w} × {preset.h}</option>)}</select>}
    </div>
    {problem && <p role="alert" className="pproblem">{problem}</p>}
  </section>;
}
