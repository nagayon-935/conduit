import { useRef, useState, useCallback, useEffect } from 'react';
import type { Terminal, IDisposable } from '@xterm/xterm';
import type { FitAddon } from '@xterm/addon-fit';
import type { ConnectionState, SessionInfo, WsControlMessage } from '../types';
import { fetchOwnSession, fetchSharedSession } from '../api/sessions';
import { ApiRequestError } from '../api/fetch';
import { ANSI, HEARTBEAT_INTERVAL_MS, RECONNECT_BASE_DELAY_MS, MAX_RECONNECT_ATTEMPTS } from '../constants';

interface UseWebSocketOptions {
  token: string; shareToken?: string; terminal: Terminal | null; fitAddon: FitAddon | null;
  onDisconnect: () => void; onError: (msg: string) => void;
  onSessionInfo?: (info: SessionInfo) => void;
  onEnded?: (reason: string) => void;
}
const inputEncoder = new TextEncoder();
function buildWsUrl(token: string, shareToken?: string): string {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${protocol}//${window.location.host}/ws?${shareToken ? 'share' : 'token'}=${encodeURIComponent(shareToken || token)}`;
}

export function useWebSocket(options: UseWebSocketOptions) {
  const latest = useRef(options); latest.current = options;
  const [state, setState] = useState<ConnectionState>('disconnected');
  const [reason, setReason] = useState('');
  const [attempt, setAttempt] = useState(0);
  const [info, setInfo] = useState<SessionInfo | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const intentional = useRef(true), ended = useRef(false), attempts = useRef(0);
  const timers = useRef<{ heartbeat?: ReturnType<typeof setInterval>; metadata?: ReturnType<typeof setInterval>; retry?: ReturnType<typeof setTimeout> }>({});
  const listeners = useRef<IDisposable[]>([]);
  const metadataAbort = useRef<AbortController | null>(null);

  const cleanup = useCallback(() => {
    clearInterval(timers.current.heartbeat); clearInterval(timers.current.metadata); clearTimeout(timers.current.retry);
    timers.current = {};
    metadataAbort.current?.abort(); metadataAbort.current = null;
    listeners.current.forEach((listener) => listener.dispose()); listeners.current = [];
  }, []);
  const discardSocket = useCallback(() => {
    const ws = wsRef.current; wsRef.current = null;
    if (ws) { ws.onopen = ws.onclose = ws.onmessage = ws.onerror = null; ws.close(); }
  }, []);
  const finish = useCallback((message: string) => {
    ended.current = true; intentional.current = true;
    cleanup(); discardSocket(); setState('ended'); setReason(message);
    latest.current.terminal?.writeln(`\r\n[Conduit] ${message}`);
    latest.current.onEnded?.(message); latest.current.onDisconnect();
  }, [cleanup, discardSocket]);
  const updateInfo = useCallback((session: SessionInfo) => {
    setInfo(session); latest.current.onSessionInfo?.(session);
  }, []);
  const refresh = useCallback((ws: WebSocket | null) => {
    const { token, shareToken } = latest.current;
    if (!token && !shareToken) return;
    metadataAbort.current?.abort();
    const controller = new AbortController(); metadataAbort.current = controller;
    const request = shareToken ? fetchSharedSession(shareToken, controller.signal) : fetchOwnSession(token, controller.signal);
    request.then((session) => {
      if (!controller.signal.aborted && wsRef.current === ws) updateInfo(session);
    }).catch((error: unknown) => {
      if (controller.signal.aborted || wsRef.current !== ws) return;
      if (error instanceof ApiRequestError && error.status === 410) finish(shareToken ? '共有リンクが無効になったか、共有先のセッションが終了しました。' : 'セッションが終了したか、再接続期限を過ぎています。');
    });
  }, [finish, updateInfo]);

  const start = useCallback(function startSocket(isReconnect: boolean) {
    cleanup(); discardSocket();
    const { token, shareToken, terminal, fitAddon } = latest.current;
    if (!token && !shareToken) return;
    const ws = new WebSocket(buildWsUrl(token, shareToken)); ws.binaryType = 'arraybuffer'; wsRef.current = ws;
    setState(isReconnect ? 'reconnecting' : 'connecting'); setReason('');
    if (terminal && !shareToken) {
      listeners.current = [terminal.onData((data) => { if (wsRef.current?.readyState === WebSocket.OPEN) wsRef.current.send(inputEncoder.encode(data)); }),
        terminal.onResize(({ cols, rows }) => { if (wsRef.current?.readyState === WebSocket.OPEN) wsRef.current.send(JSON.stringify({ type: 'resize', cols, rows })); })];
    }
    ws.onopen = () => {
      if (wsRef.current !== ws) return;
      attempts.current = 0; setAttempt(0); setState('connected');
      timers.current.heartbeat = setInterval(() => { if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ type: 'ping' })); }, HEARTBEAT_INTERVAL_MS);
      refresh(ws);
      timers.current.metadata = setInterval(() => refresh(ws), 5000);
      if (isReconnect) terminal?.writeln(ANSI.RECONNECTED);
      if (terminal && fitAddon) { fitAddon.fit(); if (!shareToken) ws.send(JSON.stringify({ type: 'resize', cols: terminal.cols, rows: terminal.rows })); }
    };
    ws.onmessage = (event: MessageEvent) => {
      if (wsRef.current !== ws) return;
      if (typeof event.data === 'string') {
        let message: WsControlMessage | undefined;
        try { message = JSON.parse(event.data) as WsControlMessage; } catch { /* terminal text */ }
        if (message && typeof message === 'object') {
          switch (message.type) {
            case 'session': updateInfo(message.session); return;
            case 'exit': finish(message.reason || 'SSH セッションが終了しました。'); return;
            case 'error': setReason(message.message); latest.current.onError(message.message); return;
            case 'ping': if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ type: 'pong' })); return;
            case 'pong': case 'resize': return;
          }
        }
        latest.current.terminal?.write(event.data);
      } else if (event.data instanceof ArrayBuffer) latest.current.terminal?.write(new Uint8Array(event.data));
    };
    ws.onerror = () => {}; // browser always follows this with onclose
    ws.onclose = () => {
      if (wsRef.current !== ws) return;
      wsRef.current = null; cleanup();
      if (intentional.current || ended.current) return;
      refresh(null);
      if (!navigator.onLine) { setState('disconnected'); setReason('ネットワーク接続を待っています。'); return; }
      if (attempts.current >= MAX_RECONNECT_ATTEMPTS) {
        setState('disconnected'); setReason('自動再接続できませんでした。手動で再接続できます。'); return;
      }
      const count = ++attempts.current; setAttempt(count); setState('reconnecting');
      setReason(`${count}/${MAX_RECONNECT_ATTEMPTS} 回目の再接続を試みています。`);
      timers.current.retry = setTimeout(() => { if (!intentional.current && !ended.current) startSocket(true); }, RECONNECT_BASE_DELAY_MS * 2 ** (count - 1));
    };
  }, [cleanup, discardSocket, finish, refresh, updateInfo]);
  const connect = useCallback(() => {
    intentional.current = false; ended.current = false; attempts.current = 0; start(false);
  }, [start]);
  const disconnect = useCallback(() => {
    intentional.current = true; cleanup(); discardSocket(); setState('disconnected');
  }, [cleanup, discardSocket]);
  useEffect(() => {
    const online = () => { if (!intentional.current && !ended.current && !wsRef.current) connect(); };
    window.addEventListener('online', online);
    return () => { window.removeEventListener('online', online); intentional.current = true; cleanup(); discardSocket(); };
  }, [cleanup, discardSocket, connect]);
  return { connect, disconnect, isConnected: state === 'connected', state, reason, attempt, info };
}
