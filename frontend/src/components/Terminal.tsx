import { useEffect, useCallback, useRef, useState } from 'react';
import { useTerminal } from '../hooks/useTerminal';
import { useWebSocket } from '../hooks/useWebSocket';
import { shareSession, revokeShare } from '../api/sessions';
import { themes } from '../themes';
import { formatReconnectDeadline } from '../utils/format';
import { loadSearchHistory, pushSearchHistory } from '../utils/searchHistory';
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

  // ── Share state ──────────────────────────────────────────────────────────
  const [activeShareToken, setActiveShareToken] = useState<string | null>(null);
  const [shareCopied, setShareCopied] = useState(false);
  const copyTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  async function handleShare() {
    if (activeShareToken) {
      // Copy existing share URL to clipboard
      const url = `${window.location.origin}/?share=${activeShareToken}`;
      await navigator.clipboard.writeText(url).catch(() => {});
      setShareCopied(true);
      if (copyTimerRef.current) clearTimeout(copyTimerRef.current);
      copyTimerRef.current = setTimeout(() => setShareCopied(false), 2000);
      return;
    }
    try {
      const res = await shareSession(sessionToken);
      setActiveShareToken(res.share_token);
      await navigator.clipboard.writeText(res.url).catch(() => {});
      setShareCopied(true);
      if (copyTimerRef.current) clearTimeout(copyTimerRef.current);
      copyTimerRef.current = setTimeout(() => setShareCopied(false), 2000);
    } catch {
      // silently ignore
    }
  }

  async function handleRevokeShare() {
    if (!activeShareToken) return;
    await revokeShare(sessionToken, activeShareToken).catch(() => {});
    setActiveShareToken(null);
  }

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

  // Font size keyboard shortcut + toast
  const [fontSizeToast, setFontSizeToast] = useState<number | null>(null);
  const toastTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  function showFontSizeToast(size: number) {
    setFontSizeToast(size);
    if (toastTimerRef.current) clearTimeout(toastTimerRef.current);
    toastTimerRef.current = setTimeout(() => setFontSizeToast(null), 1500);
  }

  // Search overlay state
  const [searchOpen, setSearchOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const [searchResultMsg, setSearchResultMsg] = useState('');
  const searchInputRef = useRef<HTMLInputElement>(null);

  // Search history (sessionStorage)
  const [searchHistory, setSearchHistory] = useState<string[]>(loadSearchHistory);
  const [showHistory, setShowHistory] = useState(false);

  function addToSearchHistory(query: string) {
    setSearchHistory((prev) => pushSearchHistory(prev, query));
  }

  // Unified keyboard shortcut handler (font size + search)
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      // Font size: Ctrl+= / Ctrl+-
      if (e.ctrlKey && (e.code === 'Equal' || e.key === '+')) {
        e.preventDefault();
        changeFontSize(1);
        const current = terminal?.options.fontSize ?? 14;
        showFontSizeToast(Math.min(32, current + 1));
        return;
      }
      if (e.ctrlKey && (e.code === 'Minus' || e.key === '-')) {
        e.preventDefault();
        changeFontSize(-1);
        const current = terminal?.options.fontSize ?? 14;
        showFontSizeToast(Math.max(8, current - 1));
        return;
      }
      // Search: Ctrl+F toggle, Escape close
      if (e.ctrlKey && e.key === 'f') {
        e.preventDefault();
        setSearchOpen((prev) => {
          if (!prev) {
            setTimeout(() => searchInputRef.current?.focus(), 50);
          }
          return !prev;
        });
        return;
      }
      if (e.key === 'Escape') {
        setSearchOpen(false);
      }
    }
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [changeFontSize, terminal]);

  function handleSearchNext() {
    if (!searchQuery) return;
    addToSearchHistory(searchQuery);
    setShowHistory(false);
    const found = search(searchQuery, { findNext: true });
    setSearchResultMsg(found ? '' : 'No results');
  }

  function handleSearchPrev() {
    if (!searchQuery) return;
    addToSearchHistory(searchQuery);
    setShowHistory(false);
    const found = search(searchQuery, { findNext: false });
    setSearchResultMsg(found ? '' : 'No results');
  }

  function handleSearchKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'Enter') {
      if (e.shiftKey) {
        handleSearchPrev();
      } else {
        handleSearchNext();
      }
      return;
    }
    if (e.key === 'ArrowDown' && showHistory) {
      e.preventDefault();
      const first = document.querySelector<HTMLButtonElement>('.search-history-item');
      first?.focus();
      return;
    }
    if (e.key === 'Escape') {
      if (showHistory) {
        setShowHistory(false);
      } else {
        setSearchOpen(false);
      }
    }
  }

  function handleSearchHistorySelect(query: string) {
    setSearchQuery(query);
    setShowHistory(false);
    setSearchResultMsg('');
    setTimeout(() => searchInputRef.current?.focus(), 0);
  }

  function handleDisconnect() {
    disconnect();
    onDisconnect();
  }

  return (
    <div className="terminal-wrapper">
      {/* Read-only viewer banner */}
      {readOnly && (
        <div className="readonly-banner">
          <span className="readonly-icon" aria-hidden="true">👁</span>
          Read-only — viewing <strong>{user}@{host}</strong>
        </div>
      )}

      <div className="terminal-status-bar">
        <div className="status-left">
          <div className="status-indicator">
            <span className={`status-dot${isConnected ? '' : ' status-dot--disconnected'}`} aria-hidden="true">●</span>
            <span className={`status-label${isConnected ? '' : ' status-label--disconnected'}`}>
              {isConnected ? (readOnly ? 'Viewing' : 'Connected') : 'Disconnected'}
            </span>
          </div>

          <div className="status-divider" />

          <div className="status-info">
            <span className="status-session">
              <span className="status-user">{user}</span>
              <span className="status-at">@</span>
              <span className="status-host">{host}</span>
              {port !== 22 && <span className="status-port">:{port}</span>}
            </span>
            {!isConnected && !readOnly && (
              <>
                <span className="status-sep">•</span>
                <span className="status-expires">
                  Reconnect by: {formatReconnectDeadline(expiresAt)}
                </span>
              </>
            )}
          </div>
        </div>

        <div className="status-right">
          {/* Share button — only for owners (non-read-only) */}
          {!readOnly && (
            <>
              {activeShareToken && (
                <button
                  type="button"
                  className="share-revoke-btn"
                  onClick={handleRevokeShare}
                  title="Stop sharing — revoke share link"
                >
                  Stop sharing
                </button>
              )}
              <button
                type="button"
                className={`share-btn${activeShareToken ? ' share-btn--active' : ''}`}
                onClick={handleShare}
                title={activeShareToken ? 'Copy share link' : 'Share session (read-only)'}
              >
                {shareCopied ? 'Copied!' : activeShareToken ? 'Copy link' : 'Share'}
                {typeof viewerCount === 'number' && viewerCount > 0 && (
                  <span className="share-viewer-count" title="Active viewers">
                    {viewerCount}
                  </span>
                )}
              </button>
            </>
          )}

          {/* Theme selector */}
          <select
            className="theme-select"
            value={currentThemeKey}
            onChange={(e) => setTheme(e.target.value)}
            title="Select terminal theme"
          >
            {Object.entries(themes).map(([key, t]) => (
              <option key={key} value={key}>{t.name}</option>
            ))}
          </select>

          <button
            type="button"
            className="disconnect-btn"
            onClick={handleDisconnect}
            title={readOnly ? 'Leave viewer session' : 'Disconnect from SSH session'}
          >
            {readOnly ? 'Leave' : 'Disconnect'}
          </button>
        </div>
      </div>

      <div className="terminal-container" ref={terminalRef} />

      {/* Font size toast */}
      {fontSizeToast !== null && (
        <div className="font-size-toast">
          Font size: {fontSizeToast}px
        </div>
      )}

      {/* Search overlay */}
      {searchOpen && (
        <div className="search-overlay">
          <div className="search-input-wrap">
            <input
              ref={searchInputRef}
              className="search-input"
              type="text"
              placeholder="Search..."
              value={searchQuery}
              onChange={(e) => {
                setSearchQuery(e.target.value);
                setSearchResultMsg('');
              }}
              onFocus={() => setShowHistory(searchHistory.length > 0 && !searchQuery)}
              onBlur={() => setTimeout(() => setShowHistory(false), 150)}
              onKeyDown={handleSearchKeyDown}
            />
            {showHistory && searchHistory.length > 0 && (
              <div className="search-history-dropdown">
                {searchHistory.map((q) => (
                  <button
                    key={q}
                    type="button"
                    className="search-history-item"
                    onMouseDown={(e) => e.preventDefault()}
                    onClick={() => handleSearchHistorySelect(q)}
                  >
                    <span className="search-history-icon" aria-hidden="true">⟳</span>
                    {q}
                  </button>
                ))}
              </div>
            )}
          </div>
          <button
            type="button"
            className="search-btn"
            onClick={handleSearchPrev}
            title="Previous match (Shift+Enter)"
          >
            ↑
          </button>
          <button
            type="button"
            className="search-btn"
            onClick={handleSearchNext}
            title="Next match (Enter)"
          >
            ↓
          </button>
          {searchResultMsg && (
            <span className="search-result-msg">{searchResultMsg}</span>
          )}
          <button
            type="button"
            className="search-close-btn"
            onClick={() => setSearchOpen(false)}
            title="Close search (Escape)"
          >
            ✕
          </button>
        </div>
      )}
    </div>
  );
}
