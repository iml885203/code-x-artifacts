import { useEffect, useRef, useState, type ReactNode } from "react";
import type { Command } from "prosemirror-state";
import type { Feature, ToolContext } from "../../feature";
import type { Key, Translate } from "../../../i18n";
import { cancelDate, chooseDate, datePicker, datePickerKey, insertDate, validDate, type DatePicker } from "./commands";
import { Calendar } from "./calendar";
import "./date.css";

export function DateTools({ picker, ctx, t }: { picker: DatePicker; ctx: ToolContext; t: Translate }) {
  const [value, setValue] = useState(picker.initial);
  const input = useRef<HTMLInputElement>(null);
  useEffect(() => { input.current?.focus({ preventScroll: true }); }, [picker.session]);
  return (
    <form className="ed-toolbar ed-date-tools" aria-label={t("editor.date")} onSubmit={(event) => {
      event.preventDefault();
      ctx.run(chooseDate(value));
    }} onKeyDown={(event) => {
      if (event.key === "Escape") { event.preventDefault(); ctx.run(cancelDate); }
    }}>
      <label>{t("editor.dateValue")}<input ref={input} type="date" min="0001-01-01" max="9999-12-31" required value={value} onChange={(event) => { setValue(event.target.value); }} /></label>
      <button type="submit" className="quiet" disabled={!validDate(value)}>{t("download.save")}</button>
      <button type="button" className="quiet" onClick={() => { ctx.run(cancelDate); }}>{t("viewer.cancel")}</button>
    </form>
  );
}

export const date: Feature & { insertions: { id: string; label: Key; command: Command; keywords?: string[]; face?: ReactNode }[] } = {
  id: "date",
  insertions: [{ id: "date", label: "editor.date", command: insertDate, keywords: ["calendar"], face: <Calendar /> }],
  plugins: () => [datePicker],
  rows: [{ id: "date-tools", render: (state, ctx, t) => {
    const picker = datePickerKey.getState(state);
    return picker ? <DateTools key={picker.session} picker={picker} ctx={ctx} t={t} /> : null;
  } }],
};
