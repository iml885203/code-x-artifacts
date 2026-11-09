// Typing @ opens the list of people; typing narrows it; Enter, Tab or a click picks; Escape leaves the @
// as text. The list is the editor's own DOM beside the caret: nothing about it is in the document.
import { Plugin, type EditorState } from "prosemirror-state";
import type { EditorView } from "prosemirror-view";
import type { Person } from "../../../api";
import type { Hooks } from "../../feature";
import { createPopup } from "./popup";
import { pick, triggerAt, type Trigger } from "./trigger";

const same = (a: Trigger, b: Trigger | null): boolean => b !== null && a.from === b.from && a.query === b.query;

export function suggest(hooks: Hooks): Plugin {
  const popup = createPopup();
  let people: readonly Person[] = [];
  let index = 0;
  let complete = true;
  // What the list was asked for, so an answer to an older question is dropped.
  let asked: Trigger | null = null;
  let request = 0;
  // The @ the person pressed Escape on: it stays text until they start another.
  let left: number | null = null;

  const close = () => {
    popup.hide();
    asked = null;
    request++;
  };

  const draw = (view: EditorView, at: Trigger) => {
    const words = hooks.mention.words();
    const caret = view.coordsAtPos(at.to);
    popup.show(caret, people, index, words, (person) => {
      choose(view, person);
    }, complete);
    view.dom.setAttribute("aria-activedescendant", popup.chosen());
  };

  const choose = (view: EditorView, person: Person) => {
    const at = triggerAt(view.state);
    if (at === null) return;
    const tr = pick(view.state, at, person);
    if (tr === null) return;
    close();
    view.dispatch(tr);
    hooks.mention.mentioned(person);
    view.focus();
  };

  const refresh = (view: EditorView, state: EditorState) => {
    const at = triggerAt(state);
    if (at === null || at.from === left) {
      if (at === null) left = null;
      if (popup.open || asked !== null) close();
      view.dom.removeAttribute("aria-activedescendant");
      return;
    }
    if (same(at, asked)) {
      if (popup.open) draw(view, at);
      return;
    }
    asked = at;
    const mine = ++request;
    hooks.mention
      .people(at.query)
      .then((result) => {
        const found = result.people;
        complete = result.complete;
        if (mine !== request) return;
        const now = triggerAt(view.state);
        if (now === null || !same(now, asked)) return;
        people = found;
        index = 0;
        // Words with a space that nobody matches are a sentence, not a name.
        if (found.length === 0 && complete && now.query.includes(" ")) {
          popup.hide();
          return;
        }
        draw(view, now);
      })
      .catch(() => {
        if (mine === request) close();
      });
  };

  return new Plugin({
    view: (view) => ({
      update: (v) => {
        refresh(v, v.state);
      },
      destroy: () => {
        close();
        view.dom.removeAttribute("aria-activedescendant");
      },
    }),
    props: {
      handleKeyDown: (view, event) => {
        // A key that confirms a composition (an IME's candidate) is the IME's, not the list's.
        // eslint-disable-next-line @typescript-eslint/no-deprecated -- IMEs can send 229 after compositionend; mention.test.ts protects that confirmation key.
        if (!popup.open || event.isComposing || view.composing || event.keyCode === 229) return false;
        const at = triggerAt(view.state);
        if (at === null) return false;
        switch (event.key) {
          case "ArrowDown":
          case "ArrowUp":
            if (people.length === 0) return false;
            index = (index + (event.key === "ArrowDown" ? 1 : people.length - 1)) % people.length;
            draw(view, at);
            return true;
          case "Enter":
          case "Tab": {
            const chosen = people[index];
            if (chosen === undefined) return false;
            choose(view, chosen);
            return true;
          }
          case "Escape":
            left = at.from;
            close();
            view.dom.removeAttribute("aria-activedescendant");
            return true;
          default:
            return false;
        }
      },
    },
  });
}
