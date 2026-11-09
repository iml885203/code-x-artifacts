import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { useState } from "react";
import { artifact, renderIn } from "../../test-support";
import { CommentsPanel } from "../../viewer/CommentsPanel";
import type { Comment } from "../../api";
import { anchorName } from "./anchor";
import { CommentsLayer } from "./CommentsLayer";

const frame = { id: "hero", name: "Hero", file: "frames/hero.html", w: 800, h: 600 };
const anchor = { type: "design", frameId: "hero", pid: "heading", name: "Title", x: 230, y: 40 };
const comment: Comment = { id: "thread", author: "alice", body: "Check this heading", createdAt: "2026-10-01T08:00:00Z", anchor, unread: false };
const other: Comment = { ...comment, id: "other", body: "Another target", anchor: { ...anchor, pid: "other", name: "Other title" } };

function Conversation({ pid }: { pid: "heading" | "body" }) {
  const [active, setActive] = useState<string | null>(null);
  const [draft, setDraft] = useState<Record<string, unknown> | null>(null);
  return <>
    <CommentsLayer comments={{ threads: [{ id: comment.id, anchor }, { id: other.id, anchor: other.anchor ?? {} }], active, onOpen: setActive, onCompose: setDraft }} frames={[{ frame, x: 120, y: 120 }]} pins={[{ id: comment.id, frameId: "hero", name: "Title", number: 1, x: 350, y: 160, fallback: false }, { id: other.id, frameId: "hero", name: "Other title", number: 2, x: 400, y: 300, fallback: false }]} selected={[{ frameId: "hero", pid }]} reports={{ [`hero/${pid}`]: { pid, tag: pid === "body" ? "body" : "h1", rect: { x: 30, y: 40, w: 200, h: 60 }, attrs: pid === "body" ? [] : [["data-name", "Title"]], inline: [], computed: {} } }} view={{ x: 0, y: 0, z: 0.5 }} />
    <CommentsPanel artifact={artifact({ title: "Canvas", author: "alice", canComment: true })} onClose={() => undefined} onSummary={() => undefined} comments={{ load: { status: "ready", list: [comment, other] }, refresh: () => Promise.resolve([comment, other]), add: () => undefined, remove: () => undefined }} draft={draft} onDraftDone={() => { setDraft(null); }} active={active} onActivate={setActive} changed={new Set()} anchorLabel={anchorName} />
  </>;
}

describe("canvas and panel share their comment threads", () => {
  it.each(["heading", "body"] as const)("opens the same thread from pin and list and focuses a draft on the selected %s", async (pid) => {
    const user = userEvent.setup();
    renderIn(<Conversation pid={pid} />);
    const pin = screen.getByRole("button", { name: "Comment 1 on Title" });
    await user.click(pin);
    expect(pin).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByText("Check this heading").closest(".comment-thread")).toHaveClass("active");
    await user.click(screen.getByRole("button", { name: "Comment on selection" }));
    expect(screen.getByText("Commenting on").parentElement).toHaveTextContent(pid === "body" ? "Hero" : "Title");
    expect(screen.getByRole("textbox", { name: "Write a comment" })).toHaveFocus();
    await user.click(screen.getByRole("button", { name: /^Cancel$/ }));
    expect(screen.queryByText("Commenting on")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Comment on selection" }));
    expect(screen.getByText("Commenting on").parentElement).toHaveTextContent(pid === "body" ? "Hero" : "Title");
    await user.click(screen.getByRole("button", { name: "on Other title" }));
    expect(pin).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByRole("button", { name: "Comment 2 on Other title" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByText("Another target").closest(".comment-thread")).toHaveClass("active");
  });
});
