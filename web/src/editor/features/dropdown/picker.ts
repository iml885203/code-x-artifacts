import type { EditorView } from "prosemirror-view";
import { validConfig, type DropdownConfig } from "./config";
import "./dropdown.css";

export interface DropdownWords {
  label: string;
  options: string;
  value: string;
  add: string;
  remove: string;
  save: string;
  cancel: string;
  invalid: string;
  changed: string;
  missing: string;
}
export type Refusal = "invalid" | "changed" | "missing";
const opened = new WeakMap<EditorView, () => void>();
export function closePicker(view: EditorView): void { opened.get(view)?.(); }

/** A draft lives outside the document; only an accepted submit writes anything. */
export function showPicker(view: EditorView, initial: DropdownConfig, words: DropdownWords, save: (config: DropdownConfig) => Refusal | null): void {
  closePicker(view);
  const host = view.dom.ownerDocument;
  const form = host.createElement("form");
  form.className = "ed-dropdown-picker";
  form.setAttribute("role", "dialog");
  form.setAttribute("aria-label", words.label);
  const title = host.createElement("strong");
  title.textContent = words.label;
  const selectedLabel = host.createElement("label");
  selectedLabel.textContent = words.value;
  const select = host.createElement("select");
  selectedLabel.append(select);
  const fields = host.createElement("fieldset");
  const legend = host.createElement("legend");
  legend.textContent = words.options;
  const rows = host.createElement("div");
  rows.className = "ed-dropdown-options";
  const actions = host.createElement("div");
  actions.className = "ed-dropdown-actions";
  const error = host.createElement("p");
  error.className = "ed-dropdown-error";
  error.setAttribute("role", "alert");
  error.hidden = true;
  const options = [...initial.options];
  let chosen = options.indexOf(initial.value);

  const button = (label: string, action: () => void, primary = false) => {
    const made = host.createElement("button");
    made.type = "button";
    made.textContent = label;
    if (!primary) made.className = "quiet";
    made.addEventListener("click", action);
    return made;
  };
  const hideError = () => { error.hidden = true; };
  const selectOptions = () => {
    select.replaceChildren(...options.map((text, index) => {
      const option = host.createElement("option");
      option.value = String(index);
      option.textContent = text;
      return option;
    }));
    select.value = String(chosen);
  };
  select.addEventListener("change", () => { chosen = Number(select.value); hideError(); });
  const drawRows = () => {
    rows.replaceChildren(...options.map((text, index) => {
      const row = host.createElement("div");
      row.className = "ed-dropdown-option";
      const label = host.createElement("label");
      const name = `${words.options} ${String(index + 1)}`;
      label.className = "ed-dropdown-option-label";
      const caption = host.createElement("span");
      caption.textContent = name;
      const input = host.createElement("input");
      input.type = "text";
      input.value = text;
      input.addEventListener("input", () => { options[index] = input.value; selectOptions(); hideError(); });
      label.append(caption, input);
      const remove = button(words.remove, () => {
        options.splice(index, 1);
        if (chosen === index) chosen = 0;
        else if (chosen > index) chosen--;
        drawRows();
        selectOptions();
        hideError();
        const inputs = rows.querySelectorAll("input");
        inputs[Math.min(index, inputs.length - 1)]?.focus();
      });
      remove.setAttribute("aria-label", `${words.remove}: ${name}`);
      remove.disabled = options.length === 1;
      row.append(label, remove);
      return row;
    }));
    add.disabled = options.length >= 50;
  };
  const add = button(words.add, () => {
    if (options.length >= 50) return;
    options.push("");
    drawRows();
    selectOptions();
    rows.querySelectorAll("input")[options.length - 1]?.focus();
  });
  fields.append(legend, rows, add);
  const close = () => {
    host.removeEventListener("pointerdown", outside);
    host.removeEventListener("keydown", escape);
    form.remove();
    opened.delete(view);
  };
  const cancel = () => { close(); if (!view.isDestroyed) view.focus(); };
  const outside = (event: Event) => { if (event.target instanceof Node && !form.contains(event.target)) close(); };
  const escape = (event: KeyboardEvent) => {
    if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); cancel(); }
  };
  const submit = button(words.save, () => undefined, true);
  submit.type = "submit";
  actions.append(button(words.cancel, cancel), submit);
  form.append(title, selectedLabel, fields, actions, error);
  drawRows();
  selectOptions();
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    if (view.isDestroyed || !view.editable) { close(); return; }
    const config = { options: [...options], value: options[chosen] ?? "" };
    const refusal = validConfig(config) ? save(config) : "invalid";
    if (refusal === null) { cancel(); return; }
    error.textContent = words[refusal];
    error.hidden = false;
  });
  host.body.append(form);
  opened.set(view, close);
  host.addEventListener("pointerdown", outside);
  host.addEventListener("keydown", escape);
  // Measure and focus the attached picker after the discovery event finishes.
  queueMicrotask(() => {
    if (!form.isConnected) return;
    const bounds = view.dom.getBoundingClientRect();
    const viewport = host.defaultView;
    form.style.left = `${String(Math.max(8, Math.min(bounds.left, (viewport?.innerWidth ?? 800) - form.offsetWidth - 8)))}px`;
    form.style.top = `${String(Math.max(8, Math.min(bounds.top, (viewport?.innerHeight ?? 600) - form.offsetHeight - 8)))}px`;
    select.focus();
  });
}
