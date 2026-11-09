// The list of people that opens under the @: plain DOM made here (text is only ever set as text), a
// listbox the editor keeps its focus beside.
import type { Person } from "../../../api";
import "./mention.css";

export interface PopupWords {
  label: string;
  none: string;
  incomplete: string;
}

export interface Popup {
  readonly open: boolean;
  /** Shows the people with the row at `index` chosen, under the point `at`. */
  show(at: { left: number; top: number; bottom: number }, people: readonly Person[], index: number, words: PopupWords, onPick: (person: Person) => void, complete?: boolean): void;
  /** The id of the chosen row, for the editor to point at. */
  chosen(): string;
  hide(): void;
}

export function createPopup(host: Document = document): Popup {
  const list = host.createElement("ul");
  list.className = "ed-mention-list";
  list.setAttribute("role", "listbox");
  // The editor keeps the caret: pressing a row must not take the focus.
  list.addEventListener("mousedown", (event) => {
    event.preventDefault();
  });
  let attached = false;
  let current = "";

  return {
    get open() {
      return attached;
    },
    chosen: () => current,
    show(at, people, index, words, onPick, complete = true) {
      list.setAttribute("aria-label", words.label);
      list.replaceChildren();
      current = "";
      if (people.length === 0) {
        const none = host.createElement("li");
        none.className = "ed-mention-none";
        none.textContent = complete ? words.none : words.incomplete;
        list.append(none);
      }
      people.forEach((person, i) => {
        const row = host.createElement("li");
        row.setAttribute("role", "option");
        row.id = `ed-mention-${String(i)}`;
        row.setAttribute("aria-selected", i === index ? "true" : "false");
        row.className = i === index ? "ed-mention-row ed-mention-on" : "ed-mention-row";
        if (i === index) current = row.id;
        const name = host.createElement("span");
        name.className = "ed-mention-name";
        name.textContent = person.name;
        const account = host.createElement("span");
        account.className = "ed-mention-account";
        account.textContent = person.user;
        row.append(name, account);
        row.addEventListener("click", () => {
          onPick(person);
        });
        list.append(row);
      });
      if (!complete && people.length > 0) {
        const notice = host.createElement("li");
        notice.className = "ed-mention-none";
        notice.setAttribute("role", "status");
        notice.textContent = words.incomplete;
        list.append(notice);
      }
      if (!attached) {
        host.body.append(list);
        attached = true;
      }
      // Under the caret, or over it when the page ends first.
      const height = list.offsetHeight;
      const below = at.bottom + 4;
      list.style.left = `${String(Math.max(8, Math.min(at.left, window.innerWidth - list.offsetWidth - 8)))}px`;
      list.style.top = `${String(below + height > window.innerHeight && at.top - 4 - height > 0 ? at.top - 4 - height : below)}px`;
      list.querySelector(".ed-mention-on")?.scrollIntoView({ block: "nearest" });
    },
    hide() {
      if (attached) list.remove();
      attached = false;
      current = "";
    },
  };
}
