import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Terminal } from '@xterm/xterm';
import type { FitAddon } from '@xterm/addon-fit';
import { useWebSocket } from './useWebSocket';
import { fetchOwnSession, fetchSharedSession } from '../api/sessions';
import { ApiRequestError } from '../api/fetch';
vi.mock('../api/sessions', () => ({ fetchOwnSession: vi.fn(), fetchSharedSession: vi.fn() }));

class MockWebSocket {
  static OPEN = 1;
  static instances: MockWebSocket[] = [];
  readyState = 0;
  binaryType = '';
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

function setup(shareToken?: string) {
  let input: ((data: string) => void) | undefined;
  let resize: ((size: { cols: number; rows: number }) => void) | undefined;
  const inputDisposable = { dispose: vi.fn() };
  const resizeDisposable = { dispose: vi.fn() };
  const term = {
    cols: 80, rows: 24,
    onData: vi.fn((callback: typeof input) => { input = callback; return inputDisposable; }),
    onResize: vi.fn((callback: typeof resize) => { resize = callback; return resizeDisposable; }),
    write: vi.fn(), writeln: vi.fn(),
  };
  const fit = { fit: vi.fn() };
  const hook = renderHook(() => useWebSocket({
    token: 'session-token', shareToken,
    terminal: term as unknown as Terminal,
    fitAddon: fit as unknown as FitAddon,
    onDisconnect: vi.fn(), onError: vi.fn(),
  }));
  act(() => hook.result.current.connect());
  const socket = MockWebSocket.instances[0];
  act(() => socket.open());
  return { ...hook, socket, term, inputDisposable, resizeDisposable,
    input: (data: string) => input?.(data),
    resize: (cols: number, rows: number) => resize?.({ cols, rows }),
  };
}

describe('useWebSocket terminal protocol', () => {
  beforeEach(() => {
    MockWebSocket.instances = [];
    vi.mocked(fetchOwnSession).mockReset().mockImplementation(() => new Promise(() => {}));
    vi.mocked(fetchSharedSession).mockReset().mockImplementation(() => new Promise(() => {}));
    vi.stubGlobal('WebSocket', MockWebSocket);
  });
  afterEach(() => { vi.unstubAllGlobals(); vi.useRealTimers(); });

  it('sends pasted control-like JSON and Unicode as binary UTF-8 input', () => {
    const hook = setup();
    for (const text of ['{"type":"ping"}', '{"type":"resize","cols":1,"rows":1}', '日本語\r']) {
      hook.input(text);
      const payload = hook.socket.send.mock.lastCall?.[0];
      expect(ArrayBuffer.isView(payload)).toBe(true);
      expect(new TextDecoder().decode(payload)).toBe(text);
    }
  });

  it('keeps resize messages as text control frames', () => {
    const hook = setup();
    hook.resize(120, 40);
    expect(hook.socket.send).toHaveBeenLastCalledWith(JSON.stringify({ type: 'resize', cols: 120, rows: 40 }));
  });

  it('does not send terminal input or resize for a shared viewer', () => {
    const hook = setup('viewer-token');
    expect(hook.socket.url).toContain('?share=viewer-token');
    expect(hook.term.onData).not.toHaveBeenCalled();
    expect(hook.term.onResize).not.toHaveBeenCalled();
    expect(hook.socket.send).not.toHaveBeenCalled();
  });

  it('disposes terminal listeners when explicitly disconnected', () => {
    const hook = setup();
    act(() => hook.result.current.disconnect());
    expect(hook.socket.close).toHaveBeenCalledOnce();
    expect(hook.inputDisposable.dispose).toHaveBeenCalledOnce();
    expect(hook.resizeDisposable.dispose).toHaveBeenCalledOnce();
    hook.unmount();
    expect(hook.inputDisposable.dispose).toHaveBeenCalledOnce();
    expect(hook.resizeDisposable.dispose).toHaveBeenCalledOnce();
  });
  it('retains terminal output and stops retrying after a permanent exit', () => {
    vi.useFakeTimers(); const hook = setup();
    act(() => hook.socket.onmessage?.({ data: JSON.stringify({ type: 'exit', reason: 'SSH ended' }) } as MessageEvent));
    expect(hook.result.current.state).toBe('ended'); expect(hook.result.current.reason).toBe('SSH ended');
    expect(hook.term.writeln).toHaveBeenCalledWith('\r\n[Conduit] SSH ended');
    act(() => vi.advanceTimersByTime(120000)); expect(MockWebSocket.instances).toHaveLength(1);
  });
  it('stops retrying a shared viewer whose link has been revoked', async () => {
    vi.mocked(fetchSharedSession).mockRejectedValue(new ApiRequestError('gone', 410));
    const hook = setup('expired-link');
    await act(async () => {});
    expect(hook.result.current.state).toBe('ended'); expect(hook.result.current.reason).toContain('共有リンク');
  });
  it('keeps the terminal available for manual reconnect after retry attempts are exhausted', () => {
    vi.useFakeTimers(); const hook = setup();
    for (let index = 0; index < 10; index++) {
      const socket = MockWebSocket.instances[MockWebSocket.instances.length - 1];
      act(() => socket.onclose?.()); act(() => vi.advanceTimersByTime(60000));
    }
    expect(hook.result.current.state).toBe('disconnected'); expect(hook.term.writeln).not.toHaveBeenCalled();
    const count = MockWebSocket.instances.length; act(() => hook.result.current.connect());
    expect(MockWebSocket.instances.length).toBe(count + 1);
  });

});
