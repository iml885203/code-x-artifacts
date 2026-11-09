// Finding elements by the id the room gave them, and setting an element's content from markup.
export const PID = "data-pid";

export const find = (pid) => {
  for (const el of document.querySelectorAll("[" + PID + "]")) if (el.getAttribute(PID) === pid) return el;
  return null;
};

// Not touched while it already shows that markup.
export const setHtml = (el, html) => {
  const t = document.createElement("template");
  t.innerHTML = html;
  if (t.innerHTML !== el.innerHTML) el.replaceChildren(t.content);
};
