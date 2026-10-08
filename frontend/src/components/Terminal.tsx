import { useEffect, useCallback, useState } from "react";
import { useTerminal } from "../hooks/useTerminal";
import { useWebSocket } from "../hooks/useWebSocket";
import { Sharing } from "../portal/Sharing";
import { ErrorMessage } from "../portal/common";
import { useFontSizeShortcuts } from "../hooks/useFontSizeShortcuts";
import { useSearchController } from "../hooks/useSearchController";
import { TerminalStatusBar } from "./TerminalStatusBar";
import { SearchOverlay } from "./SearchOverlay";
import { FONT_SIZE_DEFAULT } from "../constants";
import "@xterm/xterm/css/xterm.css";
import "./Terminal.css";

interface TerminalProps {
  sessionToken: string;
  host: string;
  port: number;
  user: string;
  expiresAt: string;
  onDisconnect: () => void;
  /** When set, this terminal is a read-only viewer connected via a share token. */
  shareToken?: string;
  shareCreator?: string;
  shareExpiresAt?: number;
  /** Current viewer count from the server (shown next to Share button). */
  viewerCount?: number;
  active?: boolean;
  recording?: boolean;
  environment?: string;
}

export function Terminal({
  sessionToken,
  host,
  port,
  user,
  expiresAt,
  onDisconnect,
  shareToken,
  shareCreator,
  shareExpiresAt,
  viewerCount,
  active = true,
  recording,
  environment,
}: TerminalProps) {
  const readOnly = !!shareToken;
  const {
    terminalRef,
    terminal,
    fitAddon,
    initTerminal,
    disposeTerminal,
    changeFontSize,
    setTheme,
    currentThemeKey,
    search,
  } = useTerminal();

  const [error, setError] = useState("");
  const [showSharing, setShowSharing] = useState(false);
  const handleError = useCallback((msg: string) => setError(msg), []);

  const { connect, disconnect, isConnected } = useWebSocket({
    token: sessionToken,
    shareToken,
    terminal,
    fitAddon,
    onDisconnect,
    onError: handleError,
  });

  const getFontSize = useCallback(
    () => terminal?.options.fontSize ?? FONT_SIZE_DEFAULT,
    [terminal],
  );
  const fontSizeToast = useFontSizeShortcuts(
    changeFontSize,
    getFontSize,
    active,
  );

  const searchController = useSearchController(search, active, () =>
    terminal?.focus(),
  );

  // Init terminal on mount, then connect WebSocket once terminal is ready
  useEffect(() => {
    initTerminal();
    return () => {
      disposeTerminal();
    };
  }, [initTerminal, disposeTerminal]);

  // Connect WebSocket once the terminal instance is available
  useEffect(() => {
    if (terminal) {
      connect();
    }
    // We only want to (re-)connect when the terminal instance changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [terminal]);

  function handleDisconnect() {
    disconnect();
    onDisconnect();
  }

  return (
    <div className="terminal-wrapper">
      <ErrorMessage message={error} />
      {!isConnected && (
        <button
          onClick={() => {
            setError("");
            connect();
          }}
        >
          再接続
        </button>
      )}
      {(environment || recording) && (
        <div className="readonly-banner">
          {environment}
          {recording && " · 録画中"}
        </div>
      )}
      {readOnly && (
        <div className="readonly-banner">
          <span className="readonly-icon" aria-hidden="true">
            👁
          </span>
          閲覧のみ · {shareCreator} さんからの共有 ·{" "}
          {shareExpiresAt &&
            `期限 ${new Date(shareExpiresAt * 1000).toLocaleString()}`}{" "}
          ·{" "}
          <strong>
            {user}@{host}
          </strong>
        </div>
      )}

      <TerminalStatusBar
        host={host}
        port={port}
        user={user}
        expiresAt={expiresAt}
        isConnected={isConnected}
        readOnly={readOnly}
        currentThemeKey={currentThemeKey}
        onThemeChange={setTheme}
        onDisconnect={handleDisconnect}
        activeShareToken={null}
        shareCopied={false}
        viewerCount={viewerCount}
        onShare={() => setShowSharing(true)}
        onRevokeShare={() => setShowSharing(true)}
      />

      <div className="terminal-container" ref={terminalRef} />

      {fontSizeToast !== null && (
        <div className="font-size-toast">Font size: {fontSizeToast}px</div>
      )}

      {searchController.open && active && (
        <SearchOverlay controller={searchController} />
      )}
      {showSharing && (
        <Sharing
          sessionId={sessionToken}
          onClose={() => setShowSharing(false)}
        />
      )}
    </div>
  );
}
