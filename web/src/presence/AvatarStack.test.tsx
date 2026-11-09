import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { renderIn } from "../test-support";
import { AvatarStack } from "./AvatarStack";
import type { Person } from "./people";

const person = (name: string, over: Partial<Person> = {}): Person => ({ id: name, name, agent: false, self: false, ...over });
const crowd = ["Ann", "Bea", "Cy", "Dee", "Eli", "Fay"].map((n) => person(n));

describe("the avatar stack", () => {
  it("shows each person by an avatar that is named for the screen reader", () => {
    renderIn(<AvatarStack people={[person("Ann", { self: true }), person("Bea")]} />);
    const stack = screen.getByRole("group", { name: "Viewing now" });
    expect(within(stack).getAllByRole("button").map((b) => b.textContent)).toEqual(["AAnn (You)", "BBea"]);
  });

  it("names a person on hover and on focus, and stops when the pointer or focus leaves", async () => {
    renderIn(<AvatarStack people={[person("Ann Lee"), person("Bea")]} />);
    const ann = screen.getByRole("button", { name: "Ann Lee" });
    await userEvent.hover(ann);
    expect((await screen.findByRole("tooltip")).textContent).toBe("Ann Lee");
    await userEvent.unhover(ann);
    fireEvent.pointerMove(document.body, { pointerType: "mouse", clientX: 500, clientY: 500 });
    expect(screen.queryByRole("tooltip")).toBeNull();
    await userEvent.tab();
    expect((await screen.findByRole("tooltip")).textContent).toBe("Ann Lee");
    await userEvent.tab();
    expect((await screen.findByRole("tooltip")).textContent).toBe("Bea");
  });

  it("names an agent by its account and client with the agent marker, and you with yours", async () => {
    renderIn(<AvatarStack people={[person("bob", { self: true }), person("bob 的 Claude Code", { agent: true })]} />);
    await userEvent.hover(screen.getByRole("button", { name: "bob (You)" }));
    expect((await screen.findByRole("tooltip")).textContent).toBe("bob You");
    await userEvent.hover(screen.getByRole("button", { name: "bob 的 Claude Code (AI agent)" }));
    await waitFor(() => { expect(screen.getByRole("tooltip").textContent).toBe("bob 的 Claude Code AI agent"); });
  });

  it("opens a list of everyone from any avatar, with who is an agent and who is you", async () => {
    renderIn(<AvatarStack people={[person("Ann", { self: true }), person("Ann 的 Claude Code", { agent: true }), person("Bea")]} />);
    expect(screen.queryByRole("dialog")).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Bea" }));
    const list = within(screen.getByRole("dialog", { name: "Everyone viewing now" }));
    expect(list.getAllByRole("listitem").map((li) => li.textContent)).toEqual(["AAnnYou", "AIAnn 的 Claude CodeAI agent", "BBea"]);
    expect(screen.queryByRole("tooltip")).toBeNull();
  });

  it("folds everyone past the limit into +N, which opens the same list", async () => {
    renderIn(<AvatarStack people={crowd} />);
    expect(within(screen.getByRole("list")).getAllByRole("listitem")).toHaveLength(4);
    const more = screen.getByRole("button", { name: "2 more viewing" });
    expect(more.textContent).toBe("+2");
    await userEvent.click(more);
    const list = within(screen.getByRole("dialog", { name: "Everyone viewing now" }));
    expect(list.getAllByRole("listitem").map((li) => li.textContent)).toEqual(["AAnn", "BBea", "CCy", "DDee", "EEli", "FFay"]);
  });

  it("opens from the keyboard and closes on Esc, on an outside click and when focus leaves", async () => {
    renderIn(
      <>
        <button type="button">outside</button>
        <AvatarStack people={crowd} />
      </>,
    );
    const first = screen.getByRole("button", { name: "Ann" });
    await userEvent.tab();
    await userEvent.tab();
    expect(document.activeElement).toBe(first);
    await userEvent.keyboard("{Enter}");
    expect(first.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByRole("dialog")).toBeTruthy();
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(document.activeElement).toBe(first);

    await userEvent.keyboard(" ");
    expect(screen.getByRole("dialog")).toBeTruthy();
    await userEvent.click(screen.getByRole("button", { name: "outside" }));
    expect(screen.queryByRole("dialog")).toBeNull();

    await userEvent.click(first);
    await userEvent.tab({ shift: true });
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("says who is here in Chinese too", async () => {
    renderIn(<AvatarStack people={[person("王小明", { self: true }), person("王小明 的 Claude Code", { agent: true })]} />, "zh-Hant");
    expect(screen.getByRole("group", { name: "正在看" })).toBeTruthy();
    await userEvent.hover(screen.getByRole("button", { name: "王小明 (你)" }));
    expect((await screen.findByRole("tooltip")).textContent).toBe("王小明 你");
    await userEvent.click(screen.getByRole("button", { name: "王小明 (你)" }));
    expect(screen.getByRole("dialog", { name: "正在看的所有人" })).toBeTruthy();
  });

  it("marks a guest in the avatar's name, its hint and the list, and keeps two guests of one nickname apart", async () => {
    const guest = (id: string, name: string, over: Partial<Person> = {}) => person(name, { id, guest: true, ...over });
    renderIn(<AvatarStack people={[guest("guest:1", "Swift Otter", { self: true }), person("Bea"), guest("guest:2", "Calm Owl #ab12"), guest("guest:3", "Calm Owl #cd34")]} />);
    const otter = screen.getByRole("button", { name: "Swift Otter (Guest, You)" });
    expect(screen.getByRole("button", { name: "Calm Owl #ab12 (Guest)" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Calm Owl #cd34 (Guest)" })).toBeInTheDocument();
    await userEvent.hover(otter);
    expect((await screen.findByRole("tooltip")).textContent).toBe("Swift Otter Guest You");
    await userEvent.click(otter);
    const list = within(screen.getByRole("dialog", { name: "Everyone viewing now" }));
    expect(list.getAllByRole("listitem").map((li) => li.textContent)).toEqual(["SSwift OtterGuestYou", "BBea", "CCalm Owl #ab12Guest", "CCalm Owl #cd34Guest"]);
  });

  it("draws nothing for an empty page", () => {
    renderIn(<AvatarStack people={[]} />);
    expect(screen.queryByRole("group")).toBeNull();
  });
});
