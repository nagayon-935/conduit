import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { Workspace } from "./Workspace";
import { apiFetch } from "../api/fetch";
import type { SessionTab } from "../types";
vi.mock("../api/fetch", () => ({ apiFetch: vi.fn(), mutate: vi.fn() }));
vi.mock("../components/TerminalPool", () => ({
  TerminalPool: ({
    tabs,
    paneTabIds,
  }: {
    tabs: SessionTab[];
    paneTabIds: (string | null)[];
  }) => (
    <div data-testid="restored">
      <span data-testid="tab-order">{tabs.map((t) => t.id).join(",")}</span>
      <span data-testid="panes">{paneTabIds.join(",")}</span>
    </div>
  ),
}));
const session = (id: string) => ({
  id,
  owner_user_id: "alice",
  target_account_id: "account",
  host: "host",
  port: 22,
  user: "ubuntu",
  state: "disconnected",
  created_at: "2026-01-01",
  expires_at: "2099-01-01",
  ws_count: 0,
  viewer_count: 0,
  recording_enabled: false,
});
afterEach(() => vi.unstubAllGlobals());
beforeEach(() => {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      disconnect() {}
    },
  );
  localStorage.clear();
  vi.clearAllMocks();
  vi.mocked(apiFetch).mockImplementation(async (path) => {
    if (path === "/api/app/sessions") return [session("a"), session("b")];
    if (path === "/api/app/targets") return [];
    if (path === "/api/app/logs") return { items: [] };
    if (path === "/api/app/shared/link")
      return {
        session: session("shared-session"),
        share: { expires_at: 4000000000 },
        creator_name: "Alice",
      };
    return { theme: "tokyo-night", font_size: 14, favorites: [] };
  });
});
it("restores every authorized saved tab, order and panes using server ownership and expiry", async () => {
  localStorage.setItem(
    "conduit:workspace:alice",
    JSON.stringify({
      ids: [
        { id: "b" },
        { id: "a" },
        { id: "shared:link", share: "link" },
        { id: "ended" },
      ],
      active: "a",
      type: "2v",
      paneIds: ["b", "a", null, null],
      ratioV: 0.35,
      ratioH: 0.6,
    }),
  );
  render(<Workspace uid="alice" page="workspace" navigate={vi.fn()} />);
  await waitFor(() =>
    expect(screen.getByTestId("tab-order")).toHaveTextContent(
      "b,a,shared:link",
    ),
  );
  expect(screen.getByTestId("panes")).toHaveTextContent("b,a,,");
  expect(localStorage.getItem("conduit:workspace:alice")).not.toContain(
    "ended",
  );
});
it("does not restore another login user’s workspace", async () => {
  localStorage.setItem(
    "conduit:workspace:alice",
    JSON.stringify({
      ids: [{ id: "a" }],
      active: "a",
      type: "1",
      paneIds: ["a", null, null, null],
    }),
  );
  render(<Workspace uid="bob" page="workspace" navigate={vi.fn()} />);
  await waitFor(() =>
    expect(localStorage.getItem("conduit:workspace:bob")).not.toBeNull(),
  );
  expect(screen.getByTestId("tab-order")).toBeEmptyDOMElement();
});
