import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App";
import { apiFetch, mutate } from "./api/fetch";
import type { Identity } from "./portalTypes";
const lifecycle = vi.hoisted(() => ({ mounts: vi.fn(), unmounts: vi.fn() }));
vi.mock("./api/fetch", () => ({ apiFetch: vi.fn(), mutate: vi.fn() }));
vi.mock("./portal/Workspace", async () => {
  const { useEffect } = await import("react");
  return {
    Workspace: ({ uid }: { uid: string }) => {
      useEffect(() => {
        lifecycle.mounts(uid);
        return () => lifecycle.unmounts(uid);
      }, [uid]);
      return <div data-testid="retained-workspace">{uid}</div>;
    },
  };
});
vi.mock("./portal/Admin", () => ({
  adminPages: { users: "ユーザー", targets: "接続先" },
  Admin: () => <div>管理データ</div>,
}));
vi.mock("./portal/Logs", () => ({ Logs: () => <h1>自分の接続履歴</h1> }));
const alice: Identity = {
  user: {
    id: "alice",
    login: "alice",
    display_name: "Alice",
    role: "user",
    enabled: true,
    must_change_password: false,
  },
  csrf_token: "csrf",
  expires_at: Math.floor(Date.now() / 1000) + 3600,
};
beforeEach(() => {
  history.replaceState(null, "", "/app");
  vi.clearAllMocks();
  vi.mocked(apiFetch).mockImplementation(async (url) => {
    if (url === "/api/auth/me") return alice;
    return { theme: "tokyo-night", font_size: 14, favorites: [] };
  });
});
describe("authenticated portal", () => {
  it("guards direct administrator URLs for an ordinary user", async () => {
    history.replaceState(null, "", "/admin/users");
    render(<App />);
    expect(
      await screen.findByRole("heading", { name: "管理者権限が必要です" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "管理者画面" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByText("管理データ")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("navigation", { name: "管理者メニュー" }),
    ).not.toBeInTheDocument();
  });
  it("retains terminal workspace across history and administrator navigation", async () => {
    const admin = { ...alice, user: { ...alice.user, role: "admin" as const } };
    vi.mocked(apiFetch).mockImplementation(async (url) =>
      url === "/api/auth/me"
        ? admin
        : { theme: "tokyo-night", font_size: 14, favorites: [] },
    );
    render(<App />);
    await screen.findByTestId("retained-workspace");
    fireEvent.click(screen.getByRole("button", { name: "接続履歴" }));
    await screen.findByRole("heading", { name: "自分の接続履歴" });
    fireEvent.click(screen.getByRole("button", { name: "管理者画面" }));
    await screen.findByText("管理データ");
    expect(lifecycle.mounts).toHaveBeenCalledOnce();
    expect(lifecycle.unmounts).not.toHaveBeenCalled();
  });
  it("preserves output while reauthenticating and clears it on a different user login", async () => {
    render(<App />);
    await screen.findByTestId("retained-workspace");
    act(() => window.dispatchEvent(new Event("conduit:reauthenticate")));
    expect(
      await screen.findByRole("heading", { name: "ログインし直してください" }),
    ).toBeInTheDocument();
    expect(lifecycle.unmounts).not.toHaveBeenCalled();
    vi.mocked(mutate).mockResolvedValue({
      ...alice,
      user: { ...alice.user, id: "bob", login: "bob", display_name: "Bob" },
    });
    fireEvent.change(screen.getByLabelText("ログイン名"), {
      target: { value: "bob" },
    });
    fireEvent.change(screen.getByLabelText("パスワード"), {
      target: { value: "temporary-password" },
    });
    fireEvent.click(screen.getByRole("button", { name: "ログイン" }));
    await waitFor(() =>
      expect(screen.getByTestId("retained-workspace")).toHaveTextContent("bob"),
    );
    expect(lifecycle.unmounts).toHaveBeenCalledWith("alice");
  });
  it("requires the initial password change before rendering the workspace", async () => {
    vi.mocked(apiFetch).mockResolvedValue({
      ...alice,
      user: { ...alice.user, must_change_password: true },
    });
    render(<App />);
    await screen.findByRole("heading", {
      name: "最初にパスワードを変更してください",
    });
    expect(lifecycle.mounts).not.toHaveBeenCalled();
  });
});
