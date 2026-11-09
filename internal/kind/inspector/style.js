import { authoredStyle, declarations } from "./declarations.js";
// The styles the page may set on an element, and hiding one: both can be undone to what the frame had.

// What the page may set: web/src/design/properties/owned.json (a test holds them equal) and display.
export const OWNED = [
  "left", "top", "width", "height", "display",
  "flex-wrap",
  "border-top-width", "border-top-style", "border-top-color", "border-right-width", "border-right-style", "border-right-color", "border-bottom-width", "border-bottom-style", "border-bottom-color", "border-left-width", "border-left-style", "border-left-color",
  "position", "flex-direction", "align-items", "justify-content", "gap", "flex-grow",
  "padding", "padding-top", "padding-right", "padding-bottom", "padding-left",
  "margin", "margin-top", "margin-right", "margin-bottom", "margin-left",
  "border-radius", "border-top-left-radius", "border-top-right-radius", "border-bottom-right-radius", "border-bottom-left-radius",
  "border-width", "border-style", "border-color", "background-color", "opacity", "box-shadow",
  "font-family", "font-size", "font-weight", "font-style", "text-align", "line-height", "letter-spacing", "color",
  "background-image", "background-size", "background-position", "background-repeat", "object-fit",
];

const had = new Map();
export const hidden = new Map();
const put = (el, prop, was) => (was[0] ? el.style.setProperty(prop, was[0], was[1]) : el.style.removeProperty(prop));
const remember = (el, prop) => [el.style.getPropertyValue(prop), el.style.getPropertyPriority(prop)];

export const setStyle = (el, prop, value) => {
  let own = had.get(el);
  if (!own) had.set(el, (own = { source: el.getAttribute("style"), overrides: new Map() }));
  const original = declarations(own.source ?? "").map((part) => /^\s*([-\w]+)\s*:([\s\S]*?);?\s*$/.exec(part)).filter((part) => part?.[1]?.toLowerCase() === prop).at(-1)?.[2];
  const normalized = (text) => text?.trim().replace(/\s*!important\s*$/i, "!important");
  if (value === null || normalized(value) === normalized(original)) own.overrides.delete(prop);
  else own.overrides.set(prop, value);
  const style = authoredStyle(own.source, own.overrides);
  if (style === null) el.removeAttribute("style");
  else el.setAttribute("style", style);
  if (hidden.has(el)) {
    hidden.set(el, remember(el, "display"));
    el.style.setProperty("display", "none", "important");
  }
};

const inlineHad = new Map();

export const setInline = (el, source) => {
  if (!inlineHad.has(el)) inlineHad.set(el, el.getAttribute("style"));
  const next = source === null ? inlineHad.get(el) : source;
  if (next === null) el.removeAttribute("style");
  else el.setAttribute("style", next);
  had.delete(el);
  if (source === null) inlineHad.delete(el);
  if (hidden.has(el)) {
    hidden.set(el, remember(el, "display"));
    el.style.setProperty("display", "none", "important");
  }
};

export const show = (el, shown) => {
  if (!shown) {
    if (!hidden.has(el)) {
      hidden.set(el, remember(el, "display"));
      el.style.setProperty("display", "none", "important");
    }
  } else if (hidden.has(el)) {
    put(el, "display", hidden.get(el));
    hidden.delete(el);
  }
};
