import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Terminal } from "@xterm/xterm";
import type { FitAddon } from "@xterm/addon-fit";
import { useWebSocket } from "./useWebSocket";
import { mutate } from "../api/fetch";
vi.mock("../api/fetch", async (importOriginal) => {
  const original = await importOriginal<typeof import("../api/fetch")>();
  return { ...original, mutate: vi.fn() };
});

class MockWebSocket {
  static OPEN = 1;
  static instances: MockWebSocket[] = [];
  readyState = 0;
  binaryType = "";
  onopen: (() => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  send = vi.fn();
  close = vi.fn();

  constructor(public url: string) {
    MockWebSocket.instances.push(this);
  }

  open() {
    this.readyState = MockWebSocket.OPEN;
    this.onopen?.();
  }
}

async function setup(shareToken?: string) {
  let input: ((data: string) => void) | undefined;
  let resize: ((size: { cols: number; rows: number }) => void) | undefined;
  const inputDisposable = { dispose: vi.fn() };
  const resizeDisposable = { dispose: vi.fn() };
  const term = {
    cols: 80,
    rows: 24,
    onData: vi.fn((callback: typeof input) => {
      input = callback;
      return inputDisposable;
    }),
    onResize: vi.fn((callback: typeof resize) => {
      resize = callback;
      return resizeDisposable;
    }),
    write: vi.fn(),
    writeln: vi.fn(),
  };
  const fit = { fit: vi.fn() };
  const hook = renderHook(() =>
    useWebSocket({
      token: "session-token",
      shareToken,
      terminal: term as unknown as Terminal,
      fitAddon: fit as unknown as FitAddon,
      onDisconnect: vi.fn(),
      onError: vi.fn(),
    }),
  );
  await act(async () => hook.result.current.connect());
  const socket = MockWebSocket.instances[0];
  act(() => socket.open());
  return {
    ...hook,
    socket,
    term,
    inputDisposable,
    resizeDisposable,
    input: (data: string) => input?.(data),
    resize: (cols: number, rows: number) => resize?.({ cols, rows }),
  };
}

describe("useWebSocket terminal protocol", async () => {
  beforeEach(() => {
    MockWebSocket.instances = [];
    vi.stubGlobal("WebSocket", MockWebSocket);
    vi.mocked(mutate).mockResolvedValue({ ticket: "one-time-ticket" });
  });
  afterEach(() => vi.unstubAllGlobals());

  it("sends pasted control-like JSON and Unicode as binary UTF-8 input", async () => {
    const hook = await setup();
    for (const text of [
      '{"type":"ping"}',
      '{"type":"resize","cols":1,"rows":1}',
      "日本語\r",
    ]) {
      hook.input(text);
      const payload = hook.socket.send.mock.lastCall?.[0];
      expect(ArrayBuffer.isView(payload)).toBe(true);
      expect(new TextDecoder().decode(payload)).toBe(text);
    }
  });

  it("keeps resize messages as text control frames", async () => {
    const hook = await setup();
    hook.resize(120, 40);
    expect(hook.socket.send).toHaveBeenLastCalledWith(
      JSON.stringify({ type: "resize", cols: 120, rows: 40 }),
    );
  });

  it("does not send terminal input or resize for a shared viewer", async () => {
    const hook = await setup("viewer-id");
    expect(hook.socket.url).toContain("?ticket=one-time-ticket");
    expect(mutate).toHaveBeenLastCalledWith(
      "/api/app/shared/viewer-id/ws-ticket",
      {},
      "POST",
      expect.any(AbortSignal),
    );
    expect(hook.term.onData).not.toHaveBeenCalled();
    expect(hook.term.onResize).not.toHaveBeenCalled();
    expect(hook.socket.send).not.toHaveBeenCalled();
  });

  it("disposes terminal listeners when explicitly disconnected", async () => {
    const hook = await setup();
    act(() => hook.result.current.disconnect());
    expect(hook.socket.close).toHaveBeenCalledOnce();
    expect(hook.inputDisposable.dispose).toHaveBeenCalledOnce();
    expect(hook.resizeDisposable.dispose).toHaveBeenCalledOnce();
    hook.unmount();
    expect(hook.inputDisposable.dispose).toHaveBeenCalledOnce();
    expect(hook.resizeDisposable.dispose).toHaveBeenCalledOnce();
  });
  it("does not open a socket when a ticket arrives after unmount", async () => {
    let resolve!: (value: { ticket: string }) => void;
    vi.mocked(mutate).mockImplementationOnce(
      () =>
        new Promise((r) => {
          resolve = r;
        }),
    );
    const hook = renderHook(() =>
      useWebSocket({
        token: "session-id",
        terminal: null,
        fitAddon: null,
        onDisconnect: vi.fn(),
        onError: vi.fn(),
      }),
    );
    act(() => hook.result.current.connect());
    const signal = vi.mocked(mutate).mock.lastCall?.[3] as AbortSignal;
    hook.unmount();
    expect(signal.aborted).toBe(true);
    await act(async () => resolve({ ticket: "late-ticket" }));
    expect(MockWebSocket.instances).toHaveLength(0);
  });
});
