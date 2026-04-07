import { useEffect, useCallback, useRef, useState } from 'react';
import { useTerminal } from '../hooks/useTerminal';
import { useWebSocket } from '../hooks/useWebSocket';
import { themes } from '../themes';
import '@xterm/xterm/css/xterm.css';
import './Terminal.css';

interface TerminalProps {
  sessionToken: string;
  host: string;
  port: number;
  user: string;
  expiresAt: string;
  onDisconnect: () => void;
}

function formatReconnectDeadline(expiresAt: string): string {
  try {
    const date = new Date(expiresAt);
    return date.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
  } catch {
    return expiresAt;
  }
}

export function Terminal({ sessionToken, host, port, user, expiresAt, onDisconnect }: TerminalProps) {
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
    terminal,
    fitAddon,
    onDisconnect,
    onError: handleError,
  });

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
  const SEARCH_HISTORY_KEY = 'conduit:search-history';
  const MAX_SEARCH_HISTORY = 5;
  const [searchHistory, setSearchHistory] = useState<string[]>(() => {
    try {
      return JSON.parse(sessionStorage.getItem(SEARCH_HISTORY_KEY) ?? '[]') as string[];
    } catch {
      return [];
    }
  });
  const [showHistory, setShowHistory] = useState(false);

  function addToSearchHistory(query: string) {
    if (!query.trim()) return;
    setSearchHistory((prev) => {
      const filtered = prev.filter((q) => q !== query);
      const updated = [query, ...filtered].slice(0, MAX_SEARCH_HISTORY);
      try {
        sessionStorage.setItem(SEARCH_HISTORY_KEY, JSON.stringify(updated));
      } catch {
        // ignore storage errors
      }
      return updated;
    });
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
      <div className="terminal-status-bar">
        <div className="status-left">
          <div className="status-indicator">
            <span className={`status-dot${isConnected ? '' : ' status-dot--disconnected'}`} aria-hidden="true">●</span>
            <span className={`status-label${isConnected ? '' : ' status-label--disconnected'}`}>
              {isConnected ? 'Connected' : 'Disconnected'}
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
            {!isConnected && (
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
            title="Disconnect from SSH session"
          >
            Disconnect
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
