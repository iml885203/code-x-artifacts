import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { Artifact } from "../api";
import { artifact, bootFor, renderIn } from "../test-support";
import { Viewer } from "./Viewer";

afterEach(() => { vi.unstubAllGlobals(); });

function setup(over: Partial<Artifact> = {}, reject = false) {
  let saved = artifact({ title: "Team notes", author: "alice", canChange: true, canEdit: true, visibility: "organisation", ...over });
  const fetch = vi.fn((url: string, init?: RequestInit) => {
    if (init?.method === "PATCH") {
      if (reject) return Promise.resolve(new Response(JSON.stringify({ error: "Please try again" }), { status: 409 }));
      const b = JSON.parse(init.body as string) as { add?: string[]; remove?: string[]; organisationEdit?: boolean; visibility?: string };
      saved = { ...saved, visibility: b.visibility ?? saved.visibility, organisationEdit: b.visibility === "private" ? false : b.organisationEdit ?? saved.organisationEdit, editors: [...saved.editors.filter(n => !b.remove?.includes(n)), ...(b.add ?? [])] };
    }
    return Promise.resolve(new Response(JSON.stringify(url === `/api/artifacts/${saved.id}` || init?.method === "PATCH" ? saved : {})));
  });
  vi.stubGlobal("fetch", fetch);
  const view = () => <Viewer boot={bootFor("en", { page: "viewer", publicLinks: true })} artifact={saved} />;
  const rendered = renderIn(view());
  return { saved: () => saved, fetch, view, rendered };
}
async function share(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: "Share" }));
  return within(screen.getByRole("dialog"));
}

it("manages named editors and organisation editing together, with saved roles after reopening and reload", async () => {
  const user = userEvent.setup();
  const server = setup();
  let d = await share(user);
  await user.type(d.getByRole("textbox", { name: "Account name" }), "bob");
  await user.click(d.getByRole("button", { name: "Add" }));
  expect((await d.findByText("bob")).closest("li")).toHaveTextContent("Can edit");
  await user.click(d.getByRole("checkbox", { name: "Everyone in the organisation can edit" }));
  await waitFor(() => { expect(d.getByText("Everyone in the organisation").closest("li")).toHaveTextContent("Can edit"); });
  expect(d.queryByText(/Only you can edit/)).not.toBeInTheDocument();
  expect(d.getByText("Changes are saved immediately.")).toBeVisible();
  expect(d.queryByRole("button", { name: "Cancel" })).not.toBeInTheDocument();
  await user.click(d.getByRole("button", { name: "Done" }));
  d = await share(user);
  expect(d.getByRole("checkbox")).toBeChecked();
  await user.click(d.getByRole("button", { name: "Remove bob" }));
  await waitFor(() => { expect(d.queryByText("bob")).not.toBeInTheDocument(); });
  server.rendered.unmount();
  renderIn(server.view());
  d = await share(user);
  expect(d.getByRole("checkbox")).toBeChecked();
  expect(d.getByText("Everyone in the organisation").closest("li")).toHaveTextContent("Can edit");
  expect(server.saved().editors).toEqual([]);
  await user.click(d.getByRole("checkbox"));
  await waitFor(() => { expect(d.getByText("Everyone in the organisation").closest("li")).toHaveTextContent("Can view"); });
  expect(d.getByRole("checkbox")).not.toBeChecked();
});

it("keeps anonymous public visitors read-only and organisation roles truthful across audience changes", async () => {
  const user = userEvent.setup();
  const server = setup({ organisationEdit: true, editors: ["bob"] });
  const d = await share(user);
  await user.click(d.getByRole("radio", { name: /^Public link/ }));
  await waitFor(() => { expect(d.getByRole("radio", { name: /^Public link/ })).toBeChecked(); });
  expect(d.getByText("Anyone with the link").closest("li")).toHaveTextContent("Can view");
  expect(d.getByText("Everyone in the organisation").closest("li")).toHaveTextContent("Can edit");
  expect(d.getByRole("checkbox")).toBeChecked();
  await user.click(d.getByRole("radio", { name: /^Only me/ }));
  await waitFor(() => { expect(d.getByRole("radio", { name: /^Only me/ })).toBeChecked(); });
  expect(d.queryByText("Everyone in the organisation")).not.toBeInTheDocument();
  expect(d.queryByRole("checkbox")).not.toBeInTheDocument();
  expect(d.getByRole("textbox", { name: "Account name" })).toBeEnabled();
  expect(d.getByText("bob").closest("li")).toHaveTextContent("Can edit");
  expect(server.saved().organisationEdit).toBe(false);
  await user.click(d.getByRole("radio", { name: /^Organisation/ }));
  await waitFor(() => { expect(d.getByRole("checkbox")).toBeEnabled(); });
  expect(d.getByRole("checkbox")).not.toBeChecked();
  expect(server.saved().organisationEdit).toBe(false);
  expect(server.fetch.mock.calls.filter(([, init]) => init?.method === "PATCH").map(([, init]) => JSON.parse(init?.body as string) as unknown)).toEqual([{ visibility: "public" }, { visibility: "private" }, { visibility: "organisation" }]);
});

it("keeps saved access on failures, retains the account for retry and offers no changes to editors or readers", async () => {
  const user = userEvent.setup();
  const s = setup({ editors: ["bob"] }, true);
  const d = await share(user);
  await user.type(d.getByRole("textbox", { name: "Account name" }), "carol");
  await user.click(d.getByRole("button", { name: "Add" }));
  expect(await d.findByRole("alert")).toHaveTextContent("Couldn't change the editors. Try again.");
  expect(d.getByRole("textbox", { name: "Account name" })).toHaveValue("carol");
  expect(d.queryByText("carol")).not.toBeInTheDocument();
  await user.click(d.getByRole("button", { name: "Remove bob" }));
  expect(d.getByText("bob")).toBeVisible();
  await user.click(d.getByRole("checkbox"));
  expect(d.getByRole("checkbox")).not.toBeChecked();
  s.rendered.unmount();
  for (const role of ["editor", "reader"] as const) {
    const v = setup({ canChange: false, canEdit: role === "editor", role });
    const readonly = await share(user);
    expect(readonly.queryByRole("radio")).not.toBeInTheDocument();
    expect(readonly.queryByRole("checkbox")).not.toBeInTheDocument();
    expect(readonly.queryByRole("textbox", { name: "Account name" })).not.toBeInTheDocument();
    expect(readonly.queryByRole("button", { name: /Remove/ })).not.toBeInTheDocument();
    v.rendered.unmount();
  }
});

it("shows saved roles during a pending change, serializes access writes and keeps the result after dismissal", async () => {
  const user = userEvent.setup();
  const s = setup();
  const original = s.fetch.getMockImplementation();
  if (!original) throw new Error("Missing test server");
  let finish: (() => Promise<void>) | undefined;
  s.fetch.mockImplementation((url, init) => {
    if (init?.method !== "PATCH") return original(url, init);
    return new Promise<Response>(resolve => { finish = () => original(url, init).then(resolve); });
  });
  const d = await share(user);
  await user.click(d.getByRole("checkbox"));
  expect(d.getByRole("checkbox")).not.toBeChecked();
  expect(d.getByRole("checkbox")).toBeDisabled();
  expect(d.getByRole("radio", { name: /^Only me/ })).toBeDisabled();
  expect(d.getByRole("status")).toHaveTextContent("Saving access");
  fireEvent(screen.getByRole("dialog"), new Event("cancel", { cancelable: true }));
  await act(async () => { if (finish) await finish(); });
  const reopened = await share(user);
  expect(reopened.getByRole("checkbox")).toBeChecked();
  expect(s.saved().organisationEdit).toBe(true);
});

it("keeps private sharing focused on named editors and retains enabled organisation editing if narrowing fails", async () => {
  const user = userEvent.setup();
  const privateView = setup({ visibility: "private" });
  let d = await share(user);
  expect(d.queryByRole("checkbox")).not.toBeInTheDocument();
  expect(d.queryByText("Share it with the organisation first.")).not.toBeInTheDocument();
  await user.type(d.getByRole("textbox", { name: "Account name" }), "bob");
  await user.click(d.getByRole("button", { name: "Add" }));
  expect((await d.findByText("bob")).closest("li")).toHaveTextContent("Can edit");
  expect(privateView.saved().visibility).toBe("private");
  privateView.rendered.unmount();
  const failed = setup({ organisationEdit: true }, true);
  d = await share(user);
  await user.click(d.getByRole("radio", { name: /^Only me/ }));
  expect(await d.findByRole("alert")).toBeVisible();
  expect(d.getByRole("radio", { name: /^Organisation/ })).toBeChecked();
  expect(d.getByRole("checkbox")).toBeChecked();
  expect(failed.saved().organisationEdit).toBe(true);
});

it("lets a public owner disable and re-enable organisation editing without changing audience or named editors", async () => {
  const user = userEvent.setup();
  const server = setup({ visibility: "public", organisationEdit: true, editors: ["bob"] });
  let d = await share(user);
  for (const enabled of [false, true]) {
    await user.click(d.getByRole("checkbox", { name: "Everyone in the organisation can edit" }));
    await waitFor(() => { expect(server.saved().organisationEdit).toBe(enabled); });
    expect(d.getByRole("radio", { name: /^Public link/ })).toBeChecked();
    expect(d.getByText("Everyone in the organisation").closest("li")).toHaveTextContent(enabled ? "Can edit" : "Can view");
    expect(d.getByText("Anyone with the link").closest("li")).toHaveTextContent("Can view");
    expect(d.getByText("bob").closest("li")).toHaveTextContent("Can edit");
  }
  server.rendered.unmount();
  renderIn(server.view());
  d = await share(user);
  expect(d.getByRole("checkbox")).toBeChecked();
  expect(d.getByRole("radio", { name: /^Public link/ })).toBeChecked();
  expect(server.saved().editors).toEqual(["bob"]);
  expect(server.fetch.mock.calls.filter(([, init]) => init?.method === "PATCH").map(([, init]) => JSON.parse(init?.body as string) as unknown)).toEqual([{ organisationEdit: false }, { organisationEdit: true }]);
});
