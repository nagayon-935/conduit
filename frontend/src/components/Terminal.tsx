import { useEffect, useCallback } from 'react';
import { useTerminal } from '../hooks/useTerminal';
import { useWebSocket } from '../hooks/useWebSocket';
import { useShareSession } from '../hooks/useShareSession';
import { useFontSizeShortcuts } from '../hooks/useFontSizeShortcuts';
import { useSearchController } from '../hooks/useSearchController';
import { TerminalStatusBar } from './TerminalStatusBar';
import { SearchOverlay } from './SearchOverlay';
import { FONT_SIZE_DEFAULT } from '../constants';
import '@xterm/xterm/css/xterm.css';
import './Terminal.css';

interface TerminalProps {
  sessionToken: string;
  host: string;
  port: number;
  user: string;
  expiresAt: string;
  onDisconnect: () => void;
  /** When set, this terminal is a read-only viewer connected via a share token. */
  shareToken?: string;
  /** Current viewer count from the server (shown next to Share button). */
  viewerCount?: number;
}

export function Terminal({ sessionToken, host, port, user, expiresAt, onDisconnect, shareToken, viewerCount }: TerminalProps) {
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

  const handleError = useCallback((msg: string) => {
    console.error('[Conduit] WebSocket error:', msg);
  }, []);

  const { connect, disconnect, isConnected } = useWebSocket({
    token: sessionToken,
    shareToken,
    terminal,
    fitAddon,
    onDisconnect,
    onError: handleError,
  });

  const { activeShareToken, shareCopied, share, revoke } = useShareSession(sessionToken);

  const getFontSize = useCallback(() => terminal?.options.fontSize ?? FONT_SIZE_DEFAULT, [terminal]);
  const fontSizeToast = useFontSizeShortcuts(changeFontSize, getFontSize);

  const searchController = useSearchController(search);

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
      {readOnly && (
        <div className="readonly-banner">
          <span className="readonly-icon" aria-hidden="true">👁</span>
          Read-only — viewing <strong>{user}@{host}</strong>
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
        activeShareToken={activeShareToken}
        shareCopied={shareCopied}
        viewerCount={viewerCount}
        onShare={share}
        onRevokeShare={revoke}
      />

      <div className="terminal-container" ref={terminalRef} />

      {fontSizeToast !== null && (
        <div className="font-size-toast">
          Font size: {fontSizeToast}px
        </div>
      )}

      {searchController.open && <SearchOverlay controller={searchController} />}
    </div>
  );
}
