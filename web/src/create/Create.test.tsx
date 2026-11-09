import { fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Header } from "../Header";
import { artifact, bootFor, renderIn } from "../test-support";
import { RenameTitle } from "./RenameTitle";

const made = artifact({ title: "Untitled document", author: "alice", kind: "doc", canEdit: true, role: "author" });

afterEach(() => {
  vi.unstubAllGlobals();
});

const sent = (call: { init?: RequestInit | undefined } | undefined): unknown => JSON.parse(call?.init?.body as string);

/** A fake fetch that answers each call with the next response and remembers the calls. */
function fetching(...answers: [number, unknown][]) {
  const calls: { url: string; init?: RequestInit | undefined }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      calls.push({ url, init });
      const [status, body] = answers.shift() ?? [200, {}];
      return Promise.resolve(new Response(JSON.stringify(body), { status }));
    }),
  );
  return calls;
}

describe("New", () => {
  it("offers Document and Design, and Document makes a doc and goes to it in the editor", async () => {
    const calls = fetching([201, made]);
    renderIn(<Header boot={bootFor("en", { user: "alice" })} />);
    await userEvent.click(screen.getByRole("button", { name: "New" }));
    expect(screen.getByRole("menuitem", { name: "Design" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("menuitem", { name: "Document" }));
    await waitFor(() => {
      expect(location.pathname + location.search).toBe(`/a/${made.id}`);
    });
    expect(calls[0]?.url).toBe("/api/new");
    expect(sent(calls[0])).toEqual({ kind: "doc", title: "Untitled document" });
  });

  it("leaves focus where the person put it when they click elsewhere as the menu closes, and gives it back to the button otherwise", async () => {
    vi.stubGlobal("fetch", vi.fn(() => new Promise<Response>(() => undefined)));
    renderIn(
      <>
        <Header boot={bootFor("en", { user: "alice" })} />
        <div role="textbox" aria-label="Doc" tabIndex={0} />
      </>,
    );
    const user = userEvent.setup({ delay: null });
    const later = () => new Promise((done) => setTimeout(done, 0));

    // The menu has gone from the page; the person's click into the document lands before Radix's own hand-back.
    await user.click(screen.getByRole("button", { name: "New" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Design" }));
    screen.getByRole("textbox", { name: "Doc" }).focus();
    await later();
    expect(screen.getByRole("textbox", { name: "Doc" })).toHaveFocus();

    // Nothing else took the focus: it goes back to the button that opened the menu.
    await user.click(screen.getByRole("button", { name: "New" }));
    await user.click(screen.getByRole("menuitem", { name: "Design" }));
    await later();
    expect(screen.getByRole("button", { name: "New" })).toHaveFocus();
  });

  it("says what each kind is for, so a person can tell a board for ideas from a design for a page", async () => {
    fetching();
    renderIn(<Header boot={bootFor("en", { user: "alice" })} />);
    await userEvent.click(screen.getByRole("button", { name: "New" }));
    expect(screen.getByRole("menuitem", { name: "Document" })).toHaveAccessibleDescription("Write notes, reports and plans");
    expect(screen.getByRole("menuitem", { name: "Design" })).toHaveAccessibleDescription("Design how a page looks");
    expect(screen.getByRole("menuitem", { name: "Board" })).toHaveAccessibleDescription("Draw flows, architecture and ideas");
    expect(screen.getByText("Draw flows, architecture and ideas")).toBeVisible();
  });

  it("makes a board from Board and goes to it", async () => {
    const calls = fetching([201, { ...made, kind: "board", title: "Untitled board" }]);
    renderIn(<Header boot={bootFor("en", { user: "alice" })} />);
    await userEvent.click(screen.getByRole("button", { name: "New" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Board" }));
    await waitFor(() => {
      expect(location.pathname).toBe(`/a/${made.id}`);
    });
    expect(sent(calls[0])).toEqual({ kind: "board", title: "Untitled board" });
  });

  it("makes a design from Design, which opens without the editor flag", async () => {
    const calls = fetching([201, { ...made, kind: "design", title: "Untitled design" }]);
    renderIn(<Header boot={bootFor("en", { user: "alice" })} />);
    await userEvent.click(screen.getByRole("button", { name: "New" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Design" }));
    await waitFor(() => {
      expect(location.pathname + location.search).toBe(`/a/${made.id}`);
    });
    expect(sent(calls[0])).toEqual({ kind: "design", title: "Untitled design" });
  });

  it("says so and stays where it is when the server refuses", async () => {
    fetching([500, { error: "no" }]);
    window.history.pushState(null, "", "/");
    renderIn(<Header boot={bootFor("en", { user: "alice" })} />);
    await userEvent.click(screen.getByRole("button", { name: "New" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Document" }));
    expect(await screen.findByText("Couldn't create it. Try again.")).toBeInTheDocument();
    expect(location.pathname).toBe("/");
  });

  it("is in Chinese for a Chinese page and absent when not signed in", () => {
    const first = renderIn(<Header boot={bootFor("zh-Hant", { user: "alice" })} />, "zh-Hant");
    expect(screen.getByRole("button", { name: "新增" })).toBeInTheDocument();
    fireEvent.pointerDown(screen.getByRole("button", { name: "新增" }), { button: 0, ctrlKey: false });
    expect(screen.getByRole("menuitem", { name: "白板" })).toHaveAccessibleDescription("畫流程、架構和想法");
    first.unmount();
    renderIn(<Header boot={bootFor("en", { user: "" })} />);
    expect(screen.queryByRole("button", { name: "New" })).not.toBeInTheDocument();
  });
});

describe("renaming from the viewer's header", () => {
  it("turns the title into a field for someone who may edit, and Enter saves the new title for the server to keep", async () => {
    const calls = fetching([200, { ...made, title: "Plan" }]);
    const renamed = vi.fn();
    renderIn(<RenameTitle artifact={made} onRenamed={renamed} />);
    await userEvent.click(screen.getByRole("button", { name: "Untitled document" }));
    const field = screen.getByRole("textbox", { name: "Title" });
    await userEvent.clear(field);
    await userEvent.type(field, "Plan{Enter}");
    await waitFor(() => {
      expect(renamed).toHaveBeenCalledWith(expect.objectContaining({ title: "Plan" }));
    });
    expect(calls[0]?.url).toBe(`/api/artifacts/${made.id}/title`);
    expect(calls[0]?.init?.method).toBe("PUT");
    expect(sent(calls[0])).toEqual({ title: "Plan" });
  });

  it("changes nothing for Escape, a blank title or the same title", async () => {
    const calls = fetching();
    renderIn(<RenameTitle artifact={made} onRenamed={vi.fn()} />);
    await userEvent.click(screen.getByRole("button", { name: "Untitled document" }));
    await userEvent.type(screen.getByRole("textbox", { name: "Title" }), " more{Escape}");
    await userEvent.click(screen.getByRole("button", { name: "Untitled document" }));
    await userEvent.clear(screen.getByRole("textbox", { name: "Title" }));
    await userEvent.type(screen.getByRole("textbox", { name: "Title" }), "   {Enter}");
    await userEvent.click(screen.getByRole("button", { name: "Untitled document" }));
    await userEvent.type(screen.getByRole("textbox", { name: "Title" }), "{Enter}");
    expect(calls).toHaveLength(0);
  });

  it("is a plain heading for a reader", () => {
    renderIn(<RenameTitle artifact={{ ...made, canEdit: false }} onRenamed={vi.fn()} />);
    expect(screen.getByRole("heading", { name: "Untitled document" })).toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("tells the person when the server refuses and keeps the old title", async () => {
    fetching([403, { error: "no" }]);
    const renamed = vi.fn();
    renderIn(<RenameTitle artifact={made} onRenamed={renamed} />);
    await userEvent.click(screen.getByRole("button", { name: "Untitled document" }));
    await userEvent.type(screen.getByRole("textbox", { name: "Title" }), "x{Enter}");
    expect(await screen.findByText("Couldn't rename it. Try again.")).toBeInTheDocument();
    expect(renamed).not.toHaveBeenCalled();
  });
});
