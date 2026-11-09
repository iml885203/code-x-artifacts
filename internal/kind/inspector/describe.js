// What an element is, for a `report`: its place, attributes, inline style and the computed values the panel shows.
import { PID } from "./dom.js";

const COMPUTED = [
  "display", "position", "top", "left", "width", "height", "z-index", "overflow",
  "margin-top", "margin-right", "margin-bottom", "margin-left",
  "padding-top", "padding-right", "padding-bottom", "padding-left",
  "color", "background-color", "opacity", "box-shadow",
  "border-top-width", "border-top-style", "border-top-color", "border-right-width", "border-right-style", "border-right-color", "border-bottom-width", "border-bottom-style", "border-bottom-color", "border-left-width", "border-left-style", "border-left-color",
  "border-top-left-radius", "border-top-right-radius", "border-bottom-right-radius", "border-bottom-left-radius",
  "font-family", "font-size", "font-weight", "font-style", "line-height", "letter-spacing", "text-align",
  "flex-wrap", "flex-direction", "justify-content", "align-items", "align-self", "flex-grow", "gap",
];

const rectOf = (el) => {
  const r = el.getBoundingClientRect();
  return { x: r.left + scrollX, y: r.top + scrollY, w: r.width, h: r.height };
};

export const describe = (el) => {
  const style = getComputedStyle(el);
  const computed = {};
  for (const name of COMPUTED) computed[name] = style.getPropertyValue(name);
  const attrs = [];
  for (const a of el.attributes) attrs.push([a.name, a.value]);
  const inline = [];
  for (let i = 0; i < el.style.length; i++) inline.push([el.style[i], el.style.getPropertyValue(el.style[i])]);
  return { pid: el.getAttribute(PID), tag: el.localName, rect: rectOf(el), attrs, inline, computed };
};
