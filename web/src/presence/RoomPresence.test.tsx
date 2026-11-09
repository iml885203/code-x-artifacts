import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Awareness } from "y-protocols/awareness";
import * as Y from "yjs";
import type { Connection } from "../editor/provider";
import { I18nProvider } from "../i18n";
import { catalogues } from "../test-support";
import { RoomPresence } from "./RoomPresence";

describe("a room's connection light", () => {
  it.each(["en", "zh-Hant"] as const)("announces each actual connection state with visible %s text", (lang) => {
    const doc = new Y.Doc();
    const awareness = new Awareness(doc);
    const labels = lang === "en"
      ? { connecting: "Connecting…", live: "Live", reconnecting: "Reconnecting…", offline: "Offline" }
      : { connecting: "連線中…", live: "即時連線", reconnecting: "重新連線中…", offline: "離線" };
    const states: Connection[] = ["connecting", "live", "reconnecting", "offline"];
    const view = (state: Connection) => <I18nProvider strings={catalogues[lang]} lang={lang}>
      <RoomPresence awareness={awareness} self={doc.clientID} state={state} />
    </I18nProvider>;
    const rendered = render(view("connecting"));
    try {
      for (const state of states) {
        rendered.rerender(view(state));
        const status = screen.getByRole("status", { name: labels[state] });
        expect(status).toBeVisible();
        expect(status).toHaveTextContent(labels[state]);
        expect(status).toHaveAttribute("aria-atomic", "true");
        expect(screen.getAllByRole("status")).toHaveLength(1);
      }
    } finally {
      rendered.unmount();
      awareness.destroy();
      doc.destroy();
    }
  });
});
