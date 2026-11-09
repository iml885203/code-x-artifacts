import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { artifact, renderIn } from "../test-support";
import { CommentsPanel } from "./CommentsPanel";
import { useComments } from "./useComments";

const doc = artifact({ title: "Plan", author: "alice", kind: "doc", canComment: true, comments: { count: 2, unread: 0 } });
const place = { tab: "t1", block: "b_2", quote: "brown fox", start: "AQ==", end: "Ag==" };
const thread = [
  { id: "c1", author: "bob", body: "Which fox?", anchor: place, createdAt: "2026-10-03T10:00:00Z" },
  { id: "c2", author: "carol", body: "A general remark.", createdAt: "2026-10-03T11:00:00Z" },
];

/** The nth of a list that must have one. */
function at<T>(list: T[], n: number): T {
  const found = list[n];
  if (found === undefined) throw new Error(`there is no item ${String(n)}`);
  return found;
}

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });

function stubApi(sent: unknown[] = []) {
  vi.stubGlobal(
    "fetch",
    vi.fn((_url: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        const body = JSON.parse(init.body as string) as { body: string; anchor?: unknown };
        sent.push(body);
        return Promise.resolve(json({ id: "c3", author: "alice", body: body.body, anchor: body.anchor, createdAt: "2026-10-04T08:00:00Z" }, 201));
      }
      return Promise.resolve(json(thread));
    }),
  );
}
afterEach(() => {
  vi.unstubAllGlobals();
});

interface Over {
  draft?: Record<string, unknown> | null;
  changed?: string[];
  done?: () => void;
  activate?: (id: string) => void;
}

function Panel({ draft = null, changed = [], done = () => undefined, activate = () => undefined }: Over) {
  const comments = useComments(doc.id, false, 2);
  return <CommentsPanel artifact={doc} onClose={() => undefined} onSummary={() => undefined} comments={comments} draft={draft} onDraftDone={done} active={null} onActivate={activate} changed={new Set(changed)} />;
}

describe("comments that point at words", () => {
  it("quote the words above their thread, and say when those words were changed", async () => {
    stubApi();
    renderIn(<Panel changed={["c1"]} />);
    const panel = await screen.findByRole("complementary", { name: "Comments" });
    const threads = await within(panel).findAllByRole("listitem");
    expect(within(at(threads, 0)).getByRole("button", { name: "brown fox" })).toBeInTheDocument();
    expect(within(at(threads, 0)).getByText("Original text changed")).toBeInTheDocument();
    expect(within(at(threads, 1)).queryByText("Original text changed")).not.toBeInTheDocument();
    expect(within(at(threads, 1)).queryByRole("button", { name: "brown fox" })).not.toBeInTheDocument();
  });

  it("open the thread in the document when the quote is chosen", async () => {
    stubApi();
    const activate = vi.fn();
    renderIn(<Panel activate={activate} />);
    await userEvent.setup().click(await screen.findByRole("button", { name: "brown fox" }));
    expect(activate).toHaveBeenCalledWith("c1");
  });

  it("are sent with the place the selected words are, which is then done with", async () => {
    const sent: unknown[] = [];
    stubApi(sent);
    const done = vi.fn();
    const user = userEvent.setup();
    renderIn(<Panel draft={place} done={done} />);
    const panel = await screen.findByRole("complementary", { name: "Comments" });
    await within(panel).findAllByRole("article");
    expect(within(panel).getByText("brown fox", { selector: "q" })).toBeInTheDocument();
    await user.type(within(panel).getByRole("textbox", { name: "Write a comment" }), "Is this the one?");
    await user.click(within(panel).getByRole("button", { name: "Comment" }));
    expect(sent).toEqual([{ body: "Is this the one?", anchor: place }]);
    expect(done).toHaveBeenCalled();
  });

  it("are given up on without sending anything", async () => {
    const sent: unknown[] = [];
    stubApi(sent);
    const done = vi.fn();
    const user = userEvent.setup();
    renderIn(<Panel draft={place} done={done} />);
    const panel = await screen.findByRole("complementary", { name: "Comments" });
    await user.click(await within(panel).findByRole("button", { name: "Cancel" }));
    expect(done).toHaveBeenCalled();
    expect(sent).toEqual([]);
  });

  it("are answered in their thread, and the answer has no anchor of its own", async () => {
    const sent: unknown[] = [];
    stubApi(sent);
    const user = userEvent.setup();
    renderIn(<Panel draft={null} />);
    const panel = await screen.findByRole("complementary", { name: "Comments" });
    await user.click(at(await within(panel).findAllByRole("button", { name: "Reply" }), 0));
    await user.type(within(panel).getByRole("textbox", { name: "Write a comment" }), "The brown one");
    await user.click(within(panel).getByRole("button", { name: "Comment" }));
    expect(sent).toEqual([{ body: "The brown one", parentId: "c1" }]);
  });
});

it("offers deletion only for own content, preserves content on cancel/failure, and labels a deleted root while retaining another person's reply", async () => {
  const own = { ...thread[0], canDelete: true };
  const reply = { id: "c3", parentId: "c1", author: "alice", body: "Keep this reply", createdAt: "2026-10-04T08:00:00Z" };
  let succeeds = false;
  const deleted = vi.fn();
  vi.stubGlobal("fetch", vi.fn((_url: string, init?: RequestInit) => {
    if (init?.method === "DELETE") { deleted(); return Promise.resolve(succeeds ? new Response(null, { status: 204 }) : json({}, 403)); }
    return Promise.resolve(json([own, reply]));
  }));
  const user = userEvent.setup();
  renderIn(<Panel />);
  await user.click(await screen.findByRole("button", { name: "Delete comment" }));
  let dialog = screen.getByRole("alertdialog", { name: "Delete your comment?" });
  await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
  expect(deleted).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: "Delete comment" }));
  dialog = screen.getByRole("alertdialog", { name: "Delete your comment?" });
  await user.click(within(dialog).getByRole("button", { name: "Delete comment" }));
  expect(await within(dialog).findByRole("alert")).toHaveTextContent("Couldn't delete");
  expect(screen.getByText("Which fox?")).toBeInTheDocument();
  succeeds = true;
  await user.click(within(dialog).getByRole("button", { name: "Delete comment" }));
  expect(await screen.findByText("Comment deleted by its author")).toBeInTheDocument();
  expect(screen.queryByText("Which fox?")).not.toBeInTheDocument();
  expect(screen.getByText("Keep this reply")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Delete comment" })).not.toBeInTheDocument();
});
