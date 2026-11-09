import { useT } from "../../i18n";
import type { Key } from "../../i18n";
import { BackIcon, BackwardIcon, DuplicateIcon, ForwardIcon, FrontIcon, GroupIcon, UngroupIcon } from "./icons";
import type { Operations } from "./useDesignOps";

const mod = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.platform) ? "⌘" : "Ctrl+";

/** The buttons for what Cmd+D, Cmd+G, Shift+Cmd+G and Cmd+[ ] do to the selected elements. */
export function ArrangeBar({ ops }: { ops: Operations }) {
  const t = useT();
  const buttons: { label: Key; keys: string; icon: () => React.JSX.Element; run: () => void }[] = [
    { label: "ops.duplicate", keys: "D", icon: DuplicateIcon, run: ops.duplicate },
    { label: "ops.group", keys: "G", icon: GroupIcon, run: ops.group },
    { label: "ops.ungroup", keys: "⇧G", icon: UngroupIcon, run: ops.ungroup },
    { label: "ops.forward", keys: "]", icon: ForwardIcon, run: () => { ops.stack("forward"); } },
    { label: "ops.backward", keys: "[", icon: BackwardIcon, run: () => { ops.stack("backward"); } },
    { label: "ops.front", keys: "⇧]", icon: FrontIcon, run: () => { ops.stack("front"); } },
    { label: "ops.back", keys: "⇧[", icon: BackIcon, run: () => { ops.stack("back"); } },
  ];
  return (
    <div className="canvas-arrange" role="toolbar" aria-label={t("ops.arrange")}>
      {buttons.map(({ label, keys, icon: Glyph, run }, i) => (
        <span key={label} className="arrange-item">
          {(i === 3) && <span className="bar-sep" />}
          <button type="button" aria-label={t(label)} title={`${t(label)} (${mod}${keys})`} onClick={run}>
            <Glyph />
          </button>
        </span>
      ))}
    </div>
  );
}
