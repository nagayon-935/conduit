import { useRef, useState, useCallback, useEffect } from "react";
import type { Terminal, IDisposable } from "@xterm/xterm";
import type { FitAddon } from "@xterm/addon-fit";
import { apiFetch, mutate, ApiFailure } from "../api/fetch";

interface Options {
  token: string;
  shareToken?: string;
  terminal: Terminal | null;
  fitAddon: FitAddon | null;
  onDisconnect: () => void;
  onError: (msg: string) => void;
}

export function useWebSocket(options: Options) {
  const latest = useRef(options);
  latest.current = options;
  const [isConnected, setConnected] = useState(false);
  const socket = useRef<WebSocket | null>(null);
  const generation = useRef(0);
  const listeners = useRef<IDisposable[]>([]);
  const retry = useRef<ReturnType<typeof setTimeout> | null>(null);
  const heartbeat = useRef<ReturnType<typeof setInterval> | null>(null);
  const request = useRef<AbortController | null>(null);
  const attempts = useRef(0);
  const clear = useCallback(() => {
    generation.current++;
    request.current?.abort();
    request.current = null;
    if (retry.current) clearTimeout(retry.current);
    retry.current = null;
    if (heartbeat.current) clearInterval(heartbeat.current);
    heartbeat.current = null;
    listeners.current.forEach((listener) => listener.dispose());
    listeners.current = [];
    if (socket.current) {
      socket.current.onclose = null;
      socket.current.close();
      socket.current = null;
    }
  }, []);
  const start = useCallback(
    async function start() {
      clear();
      const current = generation.current;
      const opts = latest.current;
      const controller = new AbortController();
      request.current = controller;
      try {
        const path = opts.shareToken
          ? `/api/app/shared/${encodeURIComponent(opts.shareToken)}`
          : `/api/app/sessions/${encodeURIComponent(opts.token)}`;
        // The ticket is scoped to the HttpOnly login cookie and consumed once.
        const { ticket } = await mutate<{ ticket: string }>(
          `${path}/ws-ticket`,
          {},
          "POST",
          controller.signal,
        );
        if (generation.current !== current || controller.signal.aborted) return;
        const ws = new WebSocket(
          `${location.protocol === "https:" ? "wss:" : "ws:"}//${location.host}/ws?ticket=${encodeURIComponent(ticket)}`,
        );
        socket.current = ws;
        ws.binaryType = "arraybuffer";
        const term = opts.terminal;
        if (term && !opts.shareToken) {
          listeners.current = [
            term.onData((data) => {
              if (ws.readyState === WebSocket.OPEN)
                ws.send(new TextEncoder().encode(data));
            }),
            term.onResize(({ cols, rows }) => {
              if (ws.readyState === WebSocket.OPEN)
                ws.send(JSON.stringify({ type: "resize", cols, rows }));
            }),
          ];
        }
        ws.onopen = () => {
          if (generation.current !== current) return;
          attempts.current = 0;
          setConnected(true);
          opts.fitAddon?.fit();
          if (term && !opts.shareToken)
            ws.send(
              JSON.stringify({
                type: "resize",
                cols: term.cols,
                rows: term.rows,
              }),
            );
          heartbeat.current = setInterval(() => {
            if (ws.readyState === WebSocket.OPEN)
              ws.send(JSON.stringify({ type: "ping" }));
          }, 30000);
        };
        let ended = false;
        ws.onmessage = (event) => {
          if (typeof event.data === "string") {
            try {
              const message = JSON.parse(event.data) as {
                type: string;
                message?: string;
              };
              if (message.type === "exit") {
                ended = true;
                term?.writeln("\r\n[セッションが終了しました]");
                ws.close();
                return;
              }
              if (message.type === "pong") return;
              if (message.type === "ping") {
                ws.send(JSON.stringify({ type: "pong" }));
                return;
              }
              if (message.type === "error") {
                latest.current.onError(message.message ?? "接続エラー");
                return;
              }
            } catch {
              /* ordinary terminal output */
            }
            term?.write(event.data);
          } else if (event.data instanceof ArrayBuffer)
            term?.write(new Uint8Array(event.data));
        };
        ws.onclose = (event) => {
          if (generation.current !== current) return;
          setConnected(false);
          clear();
          if (event.code === 4001)
            void apiFetch("/api/auth/me").catch(() => {});
          if (ended || event.code === 4001) {
            latest.current.onError(
              "接続または閲覧権限が終了しました。ログインとセッション一覧を確認してください。",
            );
            return;
          }
          if (attempts.current++ < 5)
            retry.current = setTimeout(
              () => void start(),
              Math.min(32000, 1000 * 2 ** attempts.current),
            );
          else
            latest.current.onError(
              "再接続できませんでした。端末の出力はこのタブに残っています。",
            );
        };
      } catch (error) {
        if (generation.current !== current || controller.signal.aborted) return;
        setConnected(false);
        latest.current.onError(
          error instanceof Error ? error.message : "接続できませんでした",
        );
        if (!(error instanceof ApiFailure) || error.status >= 500) {
          if (attempts.current++ < 5)
            retry.current = setTimeout(
              () => void start(),
              2000 * 2 ** attempts.current,
            );
        }
      }
    },
    [clear],
  );
  const connect = useCallback(() => {
    attempts.current = 0;
    void start();
  }, [start]);
  const disconnect = useCallback(() => {
    clear();
    setConnected(false);
  }, [clear]);
  useEffect(() => clear, [clear]);
  useEffect(() => {
    window.addEventListener("conduit:authenticated", connect);
    return () => window.removeEventListener("conduit:authenticated", connect);
  }, [connect]);
  return { connect, disconnect, isConnected };
}
