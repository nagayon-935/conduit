import { useCallback, useState } from 'react';
import { Terminal as WTermTerminal, useTerminal as useWTerm } from '@wterm/react';
import '@wterm/react/css';
import { useWtermSocket } from '../hooks/useWtermSocket';
import { useShareSession } from '../hooks/useShareSession';
import { TerminalStatusBar } from './TerminalStatusBar';
import { defaultThemeKey } from '../themes';
import './Terminal.css';

interface TerminalWtermProps {
  sessionToken: string;
  host: string;
  port: number;
  user: string;
  expiresAt: string;
  onDisconnect: () => void;
  shareToken?: string;
  viewerCount?: number;
}

// wterm ships only 3 built-in themes (solarized-dark, monokai, light).
// Map Conduit's existing xterm ITheme keys onto the closest match — only
// 'solarized-dark' has an exact match; the rest fall back to 'monokai'.
// Porting the full 16-color palettes to wterm's CSS custom properties is
// out of scope for this PoC.
const WTERM_THEME_MAP: Record<string, string> = {
  'tokyo-night': 'monokai',
  dracula: 'monokai',
  'solarized-dark': 'solarized-dark',
  'one-dark': 'monokai',
};

/**
 * PoC: renders one tab via @wterm/react instead of xterm.js to evaluate
 * DOM-based rendering, PTY resize fidelity, and theme portability before
 * deciding whether a full migration is worthwhile. Wire protocol is
 * unchanged (see useWtermSocket.ts) — only the renderer differs from
 * Terminal.tsx.
 */
export function TerminalWterm({
  sessionToken,
  host,
  port,
  user,
  expiresAt,
  onDisconnect,
  shareToken,
  viewerCount,
}: TerminalWtermProps) {
  const readOnly = !!shareToken;
  const { ref, write } = useWTerm();
  const [themeKey, setThemeKey] = useState(defaultThemeKey);

  const handleError = useCallback((msg: string) => {
    console.error('[Conduit/wterm-poc] WebSocket error:', msg);
  }, []);

  const { connect, isConnected, handleData, handleResize } = useWtermSocket({
    token: sessionToken,
    shareToken,
    write,
    onDisconnect,
    onError: handleError,
  });

  const { activeShareToken, shareCopied, share, revoke } = useShareSession(sessionToken);

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
        currentThemeKey={themeKey}
        onThemeChange={setThemeKey}
        onDisconnect={onDisconnect}
        activeShareToken={activeShareToken}
        shareCopied={shareCopied}
        viewerCount={viewerCount}
        onShare={share}
        onRevokeShare={revoke}
      />

      <div className="terminal-container" style={{ display: 'flex' }}>
        <WTermTerminal
          // @wterm/react's useTerminal() is typed for React 19's nullable ref
          // shape; this project is on React 18, where forwardRef expects a
          // non-null RefObject. A concrete migration-cost data point — full
          // adoption would need either a React 19 upgrade or an upstream fix.
          ref={ref as React.RefObject<import('@wterm/react').TerminalHandle>}
          autoResize
          cursorBlink
          theme={WTERM_THEME_MAP[themeKey] ?? 'monokai'}
          onData={handleData}
          onResize={handleResize}
          onReady={() => connect()}
          style={{ flex: 1, minWidth: 0 }}
        />
      </div>

      <div
        style={{
          position: 'absolute', bottom: 4, right: 8, zIndex: 20,
          fontSize: 11, opacity: 0.6, pointerEvents: 'none',
        }}
        title="Rendering via @wterm/react — PoC tab"
      >
        wterm PoC
      </div>
    </div>
  );
}
