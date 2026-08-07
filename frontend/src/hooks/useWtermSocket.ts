import { useRef, useState, useCallback, useEffect } from 'react';
import type { WsControlMessage } from '../types';
import {
  ANSI,
  HEARTBEAT_INTERVAL_MS,
  RECONNECT_BASE_DELAY_MS,
  MAX_RECONNECT_ATTEMPTS,
} from '../constants';

/**
 * PoC counterpart to useWebSocket.ts, adapted for @wterm/react's imperative
 * write()/onData/onResize API instead of xterm.js's Terminal instance.
 * Wire protocol is unchanged — same JSON control frames and binary framing
 * as the backend already speaks, so this is a renderer-side swap only.
 */
interface UseWtermSocketOptions {
  token: string;
  shareToken?: string;
  write: (data: string | Uint8Array) => void;
  onDisconnect: () => void;
  onError: (msg: string) => void;
}

interface UseWtermSocketReturn {
  connect: () => void;
  disconnect: () => void;
  isConnected: boolean;
  /** Pass to <Terminal onData={...}> */
  handleData: (data: string) => void;
  /** Pass to <Terminal onResize={...}> */
  handleResize: (cols: number, rows: number) => void;
}

function buildWsUrl(token: string, shareToken?: string): string {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  if (shareToken) {
    return `${protocol}//${window.location.host}/ws?share=${encodeURIComponent(shareToken)}`;
  }
  return `${protocol}//${window.location.host}/ws?token=${encodeURIComponent(token)}`;
}

export function useWtermSocket(options: UseWtermSocketOptions): UseWtermSocketReturn {
  const { token, shareToken, write, onDisconnect, onError } = options;
  const readOnly = !!shareToken;

  const [isConnected, setIsConnected] = useState(false);

  const wsRef = useRef<WebSocket | null>(null);
  const isIntentionalCloseRef = useRef(false);
  const reconnectAttemptsRef = useRef(0);
  const heartbeatIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // wterm reports size via onResize rather than us reading term.cols/rows on demand,
  // so the latest known size is cached here to (re-)send on (re)connect.
  const lastSizeRef = useRef<{ cols: number; rows: number } | null>(null);

  const writeRef = useRef(write);
  const onDisconnectRef = useRef(onDisconnect);
  const onErrorRef = useRef(onError);

  useEffect(() => { writeRef.current = write; }, [write]);
  useEffect(() => { onDisconnectRef.current = onDisconnect; }, [onDisconnect]);
  useEffect(() => { onErrorRef.current = onError; }, [onError]);

  const clearHeartbeat = useCallback(() => {
    if (heartbeatIntervalRef.current !== null) {
      clearInterval(heartbeatIntervalRef.current);
      heartbeatIntervalRef.current = null;
    }
  }, []);

  const clearReconnectTimeout = useCallback(() => {
    if (reconnectTimeoutRef.current !== null) {
      clearTimeout(reconnectTimeoutRef.current);
      reconnectTimeoutRef.current = null;
    }
  }, []);

  const startHeartbeat = useCallback((ws: WebSocket) => {
    clearHeartbeat();
    heartbeatIntervalRef.current = setInterval(() => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'ping' } satisfies WsControlMessage));
      }
    }, HEARTBEAT_INTERVAL_MS);
  }, [clearHeartbeat]);

  const connectInternal = useCallback((isReconnect: boolean) => {
    if (wsRef.current) {
      wsRef.current.onclose = null;
      wsRef.current.onerror = null;
      wsRef.current.onmessage = null;
      wsRef.current.onopen = null;
      wsRef.current.close();
      wsRef.current = null;
    }

    const url = buildWsUrl(token, shareToken);
    const ws = new WebSocket(url);
    ws.binaryType = 'arraybuffer';
    wsRef.current = ws;

    ws.onopen = () => {
      reconnectAttemptsRef.current = 0;
      setIsConnected(true);
      startHeartbeat(ws);

      if (isReconnect) {
        writeRef.current(ANSI.RECONNECTED);
      }

      const size = lastSizeRef.current;
      if (size) {
        const resizeMsg: WsControlMessage = { type: 'resize', cols: size.cols, rows: size.rows };
        ws.send(JSON.stringify(resizeMsg));
      }
    };

    ws.onmessage = (event: MessageEvent) => {
      if (typeof event.data === 'string') {
        try {
          const msg: WsControlMessage = JSON.parse(event.data) as WsControlMessage;
          if (msg && typeof msg === 'object' && 'type' in msg) {
            switch (msg.type) {
              case 'ping':
                if (ws.readyState === WebSocket.OPEN) {
                  ws.send(JSON.stringify({ type: 'pong' } satisfies WsControlMessage));
                }
                return;
              case 'pong':
                return;
              case 'error':
                onErrorRef.current(msg.message);
                return;
              case 'exit':
                // SSH session ended on the server side — close without reconnecting
                isIntentionalCloseRef.current = true;
                writeRef.current(ANSI.SESSION_ENDED);
                ws.close();
                return;
              case 'resize':
                // Server-initiated resize — ignore or handle as needed
                return;
            }
          }
        } catch {
          // Not JSON — fall through to write as text
        }
        writeRef.current(event.data);
      } else if (event.data instanceof ArrayBuffer) {
        writeRef.current(new Uint8Array(event.data));
      }
    };

    ws.onerror = () => {
      // onerror is always followed by onclose; handle reconnect there
    };

    ws.onclose = () => {
      clearHeartbeat();
      setIsConnected(false);
      wsRef.current = null;

      if (isIntentionalCloseRef.current) {
        onDisconnectRef.current();
        return;
      }

      if (reconnectAttemptsRef.current < MAX_RECONNECT_ATTEMPTS) {
        const attempt = reconnectAttemptsRef.current;
        reconnectAttemptsRef.current += 1;
        const delay = RECONNECT_BASE_DELAY_MS * Math.pow(2, attempt);

        writeRef.current(ANSI.RECONNECTING);

        reconnectTimeoutRef.current = setTimeout(() => {
          connectInternal(true);
        }, delay);
      } else {
        writeRef.current(ANSI.CONNECTION_LOST);
        onErrorRef.current('Connection lost after maximum reconnect attempts.');
        onDisconnectRef.current();
      }
    };
  }, [token, shareToken, startHeartbeat, clearHeartbeat]);

  const connect = useCallback(() => {
    isIntentionalCloseRef.current = false;
    reconnectAttemptsRef.current = 0;
    clearReconnectTimeout();
    connectInternal(false);
  }, [connectInternal, clearReconnectTimeout]);

  const disconnect = useCallback(() => {
    isIntentionalCloseRef.current = true;
    clearHeartbeat();
    clearReconnectTimeout();
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }
    setIsConnected(false);
  }, [clearHeartbeat, clearReconnectTimeout]);

  // Read-only viewers must not send stdin or resize — the server also enforces this.
  const handleData = useCallback((data: string) => {
    if (readOnly) return;
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(data);
    }
  }, [readOnly]);

  const handleResize = useCallback((cols: number, rows: number) => {
    lastSizeRef.current = { cols, rows };
    if (readOnly) return;
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      const resizeMsg: WsControlMessage = { type: 'resize', cols, rows };
      wsRef.current.send(JSON.stringify(resizeMsg));
    }
  }, [readOnly]);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      isIntentionalCloseRef.current = true;
      clearHeartbeat();
      clearReconnectTimeout();
      if (wsRef.current) {
        wsRef.current.onclose = null;
        wsRef.current.close();
        wsRef.current = null;
      }
    };
  }, [clearHeartbeat, clearReconnectTimeout]);

  return { connect, disconnect, isConnected, handleData, handleResize };
}
