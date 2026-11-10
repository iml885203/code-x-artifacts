import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { renderIn } from "../test-support";
import { OAuthGrants } from "./OAuthGrants";

const grant = { id: "g1", clientId: "client-one", clientName: "Newly authorized agent", createdAt: "2026-10-01T08:00:00Z" };
afterEach(() => { vi.unstubAllGlobals(); });

it("checks for external authorizations and offers recovery after a failed check", async () => {
  let answer: Promise<Response> = Promise.resolve(new Response("[]"));
  vi.stubGlobal("fetch", vi.fn(() => answer));
  const user = userEvent.setup();
  renderIn(<OAuthGrants />);
  await screen.findByRole("button", { name: "Refresh authorizations" });

  let finish: ((value: Response) => void) | undefined;
  answer = new Promise((resolve) => { finish = resolve; });
  await user.click(screen.getByRole("button", { name: "Refresh authorizations" }));
  expect(screen.getByRole("button", { name: "Checking…" })).toBeDisabled();
  expect(screen.getByRole("region", { name: "Authorized clients" })).toHaveAttribute("aria-busy", "true");
  finish?.(new Response(JSON.stringify([grant])));
  expect(await screen.findByText("Newly authorized agent")).toBeInTheDocument();
  expect(screen.getByText("client-one")).toBeInTheDocument();

  answer = Promise.resolve(new Response("{}", { status: 500 }));
  await user.click(screen.getByRole("button", { name: "Refresh authorizations" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("Authorized clients could not be loaded");
  answer = Promise.resolve(new Response(JSON.stringify([grant])));
  await user.click(screen.getByRole("button", { name: "Try again" }));
  expect(await screen.findByText("Newly authorized agent")).toBeInTheDocument();
  expect(screen.getByRole("region", { name: "Authorized clients" })).toHaveAttribute("aria-busy", "false");
});
