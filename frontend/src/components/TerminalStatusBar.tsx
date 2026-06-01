import { themes } from '../themes';
import { formatReconnectDeadline } from '../utils/format';

interface TerminalStatusBarProps {
  host: string;
  port: number;
  user: string;
  expiresAt: string;
  isConnected: boolean;
  readOnly: boolean;
  currentThemeKey: string;
  onThemeChange: (key: string) => void;
  onDisconnect: () => void;
  // Share controls (owners only)
  activeShareToken: string | null;
  shareCopied: boolean;
  viewerCount?: number;
  onShare: () => void;
  onRevokeShare: () => void;
}

/** The terminal's top status bar: connection state, session info, and controls. */
export function TerminalStatusBar({
  host, port, user, expiresAt, isConnected, readOnly,
  currentThemeKey, onThemeChange, onDisconnect,
  activeShareToken, shareCopied, viewerCount, onShare, onRevokeShare,
}: TerminalStatusBarProps) {
  return (
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
        {!readOnly && (
          <>
            {activeShareToken && (
              <button
                type="button"
                className="share-revoke-btn"
                onClick={onRevokeShare}
                title="Stop sharing — revoke share link"
              >
                Stop sharing
              </button>
            )}
            <button
              type="button"
              className={`share-btn${activeShareToken ? ' share-btn--active' : ''}`}
              onClick={onShare}
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

        <select
          className="theme-select"
          value={currentThemeKey}
          onChange={(e) => onThemeChange(e.target.value)}
          title="Select terminal theme"
        >
          {Object.entries(themes).map(([key, t]) => (
            <option key={key} value={key}>{t.name}</option>
          ))}
        </select>

        <button
          type="button"
          className="disconnect-btn"
          onClick={onDisconnect}
          title={readOnly ? 'Leave viewer session' : 'Disconnect from SSH session'}
        >
          {readOnly ? 'Leave' : 'Disconnect'}
        </button>
      </div>
    </div>
  );
}
