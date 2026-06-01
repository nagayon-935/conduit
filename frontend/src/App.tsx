import { useState, useCallback, useEffect, useRef, type MouseEvent as ReactMouseEvent } from 'react';
import { ConnectForm } from './components/ConnectForm';
import { Terminal } from './components/Terminal';
import { SessionList } from './components/SessionList';
import { LogPage } from './components/LogPage';
import { TabBar } from './components/TabBar';
import { NewConnectionOverlay } from './components/NewConnectionOverlay';
import type { AppState, AuthType, LayoutType } from './types';
import { saveSession, loadSession, clearSession } from './utils/session';
import { useConnectionHistory } from './hooks/useConnectionHistory';
import { useProfiles } from './hooks/useProfiles';
import { fetchSessions } from './api/sessions';
import { getSlotStyle, getTabStyle } from './utils/paneGeometry';
import './App.css';

type ViewState = 'main' | 'sessions' | 'logs';

interface SessionTab {
  id: string;
  sessionToken: string;
  host: string;
  port: number;
  user: string;
  expiresAt: string;
  /** Set when this tab is a read-only viewer connected via a share token. */
  shareToken?: string;
}

export default function App() {
  const [appState, setAppState] = useState<AppState>('idle');
  const [viewState, setViewState] = useState<ViewState>('main');
  const [tabs, setTabs] = useState<SessionTab[]>([]);
  const [activeTabId, setActiveTabId] = useState<string | null>(null);
  const [showOverlay, setShowOverlay] = useState(false);

  // Layout state
  const [layoutType, setLayoutType] = useState<LayoutType>('1');
  const [paneTabIds, setPaneTabIds] = useState<(string | null)[]>([null, null, null, null]);
  const [splitRatioV, setSplitRatioV] = useState(0.5); // left ↔ right
  const [splitRatioH, setSplitRatioH] = useState(0.5); // top ↔ bottom
  const isDraggingVRef = useRef(false);
  const isDraggingHRef = useRef(false);

  const { history, addEntry } = useConnectionHistory();
  const { profiles } = useProfiles();

  // ── Active session count (for TabBar badge) ──────────────────────────────
  const [sessionCount, setSessionCount] = useState<number | undefined>(undefined);
  useEffect(() => {
    async function refresh() {
      try {
        const data = await fetchSessions();
        setSessionCount(data.length);
      } catch {
        // network errors are silently ignored — badge just won't update
      }
    }
    refresh();
    const iv = setInterval(refresh, 10_000);
    return () => clearInterval(iv);
  }, []);

  // ── Layout switching ────────────────────────────────────────────────────
  const switchLayout = useCallback((newLayout: LayoutType) => {
    if (newLayout === '1') {
      setLayoutType('1');
      setPaneTabIds((prev) => [prev[0] ?? activeTabId, null, null, null]);
      return;
    }
    const numPanes = newLayout === '4' ? 4 : 2;
    setTabs((prevTabs) => {
      const orderedIds = prevTabs.map((t) => t.id);
      const newPanes: (string | null)[] = [null, null, null, null];
      for (let i = 0; i < numPanes; i++) {
        newPanes[i] = orderedIds[i] ?? null;
      }
      setLayoutType(newLayout);
      setPaneTabIds(newPanes);
      return prevTabs;
    });
  }, [activeTabId]);

  // ── Layout keyboard shortcuts (Alt+1/2/3/4) ────────────────────────────
  // Use e.code (physical key position) instead of e.key so that macOS Option
  // key works correctly — Option+Digit2 produces '™' in e.key on macOS.
  useEffect(() => {
    const LAYOUT_CODES: Record<string, LayoutType> = {
      'Digit1': '1',
      'Digit2': '2v',
      'Digit3': '2h',
      'Digit4': '4',
    };
    function handleLayoutKey(e: KeyboardEvent) {
      if (!e.altKey) return;
      const layout = LAYOUT_CODES[e.code];
      if (!layout) return;
      e.preventDefault();
      switchLayout(layout);
    }
    window.addEventListener('keydown', handleLayoutKey);
    return () => window.removeEventListener('keydown', handleLayoutKey);
  }, [switchLayout]);


  // ── Divider drag handlers ───────────────────────────────────────────────
  function handleDividerVMouseDown(e: ReactMouseEvent) {
    e.preventDefault();
    isDraggingVRef.current = true;
  }

  function handleDividerHMouseDown(e: ReactMouseEvent) {
    e.preventDefault();
    isDraggingHRef.current = true;
  }

  useEffect(() => {
    function onMouseMove(e: MouseEvent) {
      if (isDraggingVRef.current) {
        const ratio = Math.min(0.8, Math.max(0.2, e.clientX / window.innerWidth));
        setSplitRatioV(ratio);
      }
      if (isDraggingHRef.current) {
        const tabBarH = 40;
        const ratio = Math.min(
          0.8,
          Math.max(0.2, (e.clientY - tabBarH) / (window.innerHeight - tabBarH)),
        );
        setSplitRatioH(ratio);
      }
    }
    function onMouseUp() {
      isDraggingVRef.current = false;
      isDraggingHRef.current = false;
    }
    window.addEventListener('mousemove', onMouseMove);
    window.addEventListener('mouseup', onMouseUp);
    return () => {
      window.removeEventListener('mousemove', onMouseMove);
      window.removeEventListener('mouseup', onMouseUp);
    };
  }, []);

  // ── Restore session on mount (or enter viewer mode via ?share=) ─────────
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const shareToken = params.get('share');

    if (shareToken) {
      // Remove ?share from the URL without reloading (keeps the page clean).
      const cleanUrl = window.location.pathname;
      window.history.replaceState(null, '', cleanUrl);

      const id = crypto.randomUUID();
      const tab: SessionTab = {
        id,
        // sessionToken is not known from the share URL; the WS uses shareToken directly.
        sessionToken: '',
        host: 'shared session',
        port: 22,
        user: '',
        expiresAt: '',
        shareToken,
      };
      setTabs([tab]);
      setActiveTabId(id);
      return;
    }

    const stored = loadSession();
    if (stored) {
      const id = crypto.randomUUID();
      const tab: SessionTab = {
        id,
        sessionToken: stored.token,
        host: stored.host,
        port: stored.port,
        user: stored.user,
        expiresAt: stored.expiresAt,
      };
      setTabs([tab]);
      setActiveTabId(id);
    }
  }, []);

  // ── Connect ─────────────────────────────────────────────────────────────
  const handleConnect = useCallback(
    (token: string, expiresAt: string, host: string, port: number, user: string, authType: AuthType) => {
      const id = crypto.randomUUID();
      const tab: SessionTab = { id, sessionToken: token, host, port, user, expiresAt };
      setTabs((prev) => [...prev, tab]);
      setActiveTabId(id);
      saveSession({ token, expiresAt, host, port, user });
      addEntry(host, port, user, authType);
      setShowOverlay(false);
      setAppState('idle');
      setViewState('main');

      // Fill first empty pane slot when in split mode
      if (layoutType !== '1') {
        setPaneTabIds((prev) => {
          const emptyIdx = prev.findIndex((p) => p === null);
          if (emptyIdx !== -1) {
            const updated = [...prev];
            updated[emptyIdx] = id;
            return updated;
          }
          return prev;
        });
      }
    },
    [addEntry, layoutType],
  );

  // ── Reorder tabs ────────────────────────────────────────────────────────
  const handleReorderTabs = useCallback((fromId: string, toId: string) => {
    setTabs((prev) => {
      const fromIdx = prev.findIndex((t) => t.id === fromId);
      const toIdx = prev.findIndex((t) => t.id === toId);
      if (fromIdx === -1 || toIdx === -1 || fromIdx === toIdx) return prev;
      const updated = [...prev];
      const [moved] = updated.splice(fromIdx, 1);
      updated.splice(toIdx, 0, moved);
      return updated;
    });
  }, []);

  // ── Close tab ───────────────────────────────────────────────────────────
  const handleCloseTab = useCallback(
    (id: string) => {
      // Remove from pane slots; collapse to single view if ≤ 1 pane remains
      const newPanes = paneTabIds.map((p) => (p === id ? null : p));
      const occupiedCount = newPanes.filter(Boolean).length;
      setPaneTabIds(occupiedCount > 0 ? newPanes : [null, null, null, null]);
      if (occupiedCount <= 1) setLayoutType('1');

      setTabs((prev) => {
        const remaining = prev.filter((t) => t.id !== id);
        if (remaining.length === 0) {
          clearSession();
          setActiveTabId(null);
          setPaneTabIds([null, null, null, null]);
          setLayoutType('1');
        } else {
          setActiveTabId((currentActive) => {
            if (currentActive === id) {
              const idx = prev.findIndex((t) => t.id === id);
              const next = prev[idx - 1] ?? prev[idx + 1];
              return next?.id ?? null;
            }
            return currentActive;
          });
        }
        return remaining;
      });
    },
    [paneTabIds],
  );

  const handleTabSelect = useCallback((id: string) => {
    setActiveTabId(id);
  }, []);

  const handleStateChange = useCallback((state: AppState) => {
    setAppState(state);
  }, []);

  // ── Views: sessions / logs ──────────────────────────────────────────────
  if (viewState === 'sessions') {
    return <SessionList onBack={() => setViewState('main')} />;
  }
  if (viewState === 'logs') {
    return <LogPage onBack={() => setViewState('main')} />;
  }

  // ── View: no tabs — show ConnectForm ────────────────────────────────────
  if (tabs.length === 0) {
    return (
      <ConnectForm
        appState={appState}
        onConnect={handleConnect}
        onStateChange={handleStateChange}
        history={history}
        onShowSessions={() => setViewState('sessions')}
        onShowLogs={() => setViewState('logs')}
        sessionCount={sessionCount}
      />
    );
  }

  // ── View: terminal layout ───────────────────────────────────────────────
  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100vh' }}>
      <TabBar
        tabs={tabs.map(({ id, host, port, user }) => ({ id, host, port, user }))}
        activeId={activeTabId}
        onSelect={handleTabSelect}
        onClose={handleCloseTab}
        onNew={() => setShowOverlay(true)}
        layoutType={layoutType}
        paneTabIds={paneTabIds}
        onLayoutChange={switchLayout}
        profiles={profiles}
        onReorder={handleReorderTabs}
      />

      {/* ── Terminal pool: all sessions always mounted, CSS positions them ── */}
      <div style={{ flex: 1, position: 'relative', minHeight: 0, overflow: 'hidden' }}>
        {tabs.map((tab) => (
          <div
            key={tab.id}
            style={getTabStyle(tab.id, layoutType, paneTabIds, activeTabId, splitRatioV, splitRatioH)}
          >
            <Terminal
              sessionToken={tab.sessionToken}
              host={tab.host}
              port={tab.port}
              user={tab.user}
              expiresAt={tab.expiresAt}
              onDisconnect={() => handleCloseTab(tab.id)}
              shareToken={tab.shareToken}
            />
          </div>
        ))}

        {/* Empty pane placeholders for unoccupied split slots */}
        {layoutType !== '1' &&
          Array.from({ length: layoutType === '4' ? 4 : 2 }, (_, slotIdx) => {
            const tabId = paneTabIds[slotIdx];
            if (tabId != null && tabs.some((t) => t.id === tabId)) return null;
            return (
              <div key={`empty-${slotIdx}`} style={getSlotStyle(slotIdx, layoutType, splitRatioV, splitRatioH)}>
                <div className="split-empty-pane">
                  <span>No session selected</span>
                </div>
              </div>
            );
          })}

        {/* Vertical divider */}
        {(layoutType === '2v' || layoutType === '4') && (
          <div
            className="split-divider-v"
            style={{ position: 'absolute', top: 0, bottom: 0, left: `${splitRatioV * 100}%`, transform: 'translateX(-50%)', zIndex: 10 }}
            onMouseDown={handleDividerVMouseDown}
            onDoubleClick={() => setSplitRatioV(0.5)}
            title="Double-click to reset"
          />
        )}
        {/* Horizontal divider */}
        {(layoutType === '2h' || layoutType === '4') && (
          <div
            className="split-divider-h"
            style={{ position: 'absolute', left: 0, right: 0, top: `${splitRatioH * 100}%`, transform: 'translateY(-50%)', zIndex: 10 }}
            onMouseDown={handleDividerHMouseDown}
            onDoubleClick={() => setSplitRatioH(0.5)}
            title="Double-click to reset"
          />
        )}
      </div>

      {showOverlay && (
        <NewConnectionOverlay
          onConnect={handleConnect}
          onClose={() => setShowOverlay(false)}
          history={history}
          profiles={profiles}
        />
      )}
    </div>
  );
}
