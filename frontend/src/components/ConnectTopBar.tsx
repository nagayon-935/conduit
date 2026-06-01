import type { UseNavMenuResult } from '../hooks/useNavMenu';

interface ConnectTopBarProps {
  nav: UseNavMenuResult;
  onShowSessions?: () => void;
  onShowLogs?: () => void;
  sessionCount?: number;
}

/** Fixed top bar: hamburger nav dropdown + app identity. */
export function ConnectTopBar({ nav, onShowSessions, onShowLogs, sessionCount }: ConnectTopBarProps) {
  return (
    <header className="cf-topbar">
      <div className="cf-topbar-left" ref={nav.menuRef}>
        <button
          className={`cf-topbar-menu-btn${nav.open ? ' active' : ''}`}
          aria-label="Menu"
          aria-expanded={nav.open}
          onClick={nav.toggle}
        >
          ≡
        </button>
        {nav.open && (onShowSessions || onShowLogs) && (
          <div className="cf-nav-dropdown" role="menu">
            {onShowSessions && (
              <button
                className="cf-nav-item"
                role="menuitem"
                onClick={() => { nav.close(); onShowSessions(); }}
              >
                <span>Sessions</span>
                {sessionCount !== undefined && (
                  <span className="cf-nav-badge">{sessionCount}</span>
                )}
              </button>
            )}
            {onShowLogs && (
              <button
                className="cf-nav-item"
                role="menuitem"
                onClick={() => { nav.close(); onShowLogs(); }}
              >
                Logs
              </button>
            )}
          </div>
        )}
      </div>
      <div className="cf-topbar-brand">
        <img src="/favicon.svg" alt="Conduit Logo" className="cf-topbar-logo" />
        <span className="cf-topbar-title">Conduit</span>
        <span className="cf-topbar-subtitle">Secure Web SSH Terminal</span>
      </div>
    </header>
  );
}
