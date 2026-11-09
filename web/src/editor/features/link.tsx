// Links: the Link button and Cmd+K open the popover (LinkPopover.tsx).
import { LinkIcon } from "../../icons/link";
import { button, type Feature } from "../feature";
import { linkAt } from "../link";

export const link: Feature = {
  id: "link",
  tools: [button({ id: "link", group: "blocks", bubble: 4, label: "editor.link", face: <LinkIcon />, pressed: (state) => linkAt(state) !== null, run: (ctx) => { ctx.openLink(); } })],
  keymap: (hooks) => ({
    "Mod-k": () => {
      hooks.onLink();
      return true;
    },
  }),
};
