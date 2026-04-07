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
import './App.css';

type ViewState = 'main' | 'sessions' | 'logs';

interface SessionTab {
  id: string;
  sessionToken: string;
  host: string;
  port: number;
  user: string;
  expiresAt: string;
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

  // ── Restore session on mount ────────────────────────────────────────────
  useEffect(() => {
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

  // ── Helper: render one pane ─────────────────────────────────────────────
  function renderPane(tabId: string | null) {
    const tab = tabId ? tabs.find((t) => t.id === tabId) : undefined;
    if (!tab) {
      return (
        <div className="split-empty-pane">
          <span>No session selected</span>
        </div>
      );
    }
    return (
      <Terminal
        key={tab.id}
        sessionToken={tab.sessionToken}
        host={tab.host}
        port={tab.port}
        user={tab.user}
        expiresAt={tab.expiresAt}
        onDisconnect={() => handleCloseTab(tab.id)}
      />
    );
  }

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

      {/* ── Layout: single ── */}
      {layoutType === '1' && (
        tabs.map((tab) => (
          <div
            key={tab.id}
            style={{
              flex: 1,
              display: tab.id === activeTabId ? 'flex' : 'none',
              flexDirection: 'column',
              minHeight: 0,
            }}
          >
            <Terminal
              sessionToken={tab.sessionToken}
              host={tab.host}
              port={tab.port}
              user={tab.user}
              expiresAt={tab.expiresAt}
              onDisconnect={() => handleCloseTab(tab.id)}
            />
          </div>
        ))
      )}

      {/* ── Layout: side-by-side (2v) ── */}
      {layoutType === '2v' && (
        <div style={{ flex: 1, display: 'flex', minHeight: 0 }}>
          <div style={{ flex: splitRatioV, display: 'flex', flexDirection: 'column', minHeight: 0, overflow: 'hidden' }}>
            {renderPane(paneTabIds[0])}
          </div>
          <div className="split-divider-v" onMouseDown={handleDividerVMouseDown} onDoubleClick={() => setSplitRatioV(0.5)} title="Double-click to reset" />
          <div style={{ flex: 1 - splitRatioV, display: 'flex', flexDirection: 'column', minHeight: 0, overflow: 'hidden' }}>
            {renderPane(paneTabIds[1])}
          </div>
        </div>
      )}

      {/* ── Layout: top/bottom (2h) ── */}
      {layoutType === '2h' && (
        <div style={{ flex: 1, display: 'flex', flexDirection: 'column', minHeight: 0 }}>
          <div style={{ flex: splitRatioH, display: 'flex', flexDirection: 'column', minHeight: 0, overflow: 'hidden' }}>
            {renderPane(paneTabIds[0])}
          </div>
          <div className="split-divider-h" onMouseDown={handleDividerHMouseDown} onDoubleClick={() => setSplitRatioH(0.5)} title="Double-click to reset" />
          <div style={{ flex: 1 - splitRatioH, display: 'flex', flexDirection: 'column', minHeight: 0, overflow: 'hidden' }}>
            {renderPane(paneTabIds[1])}
          </div>
        </div>
      )}

      {/* ── Layout: 2×2 grid (4) ── */}
      {layoutType === '4' && (
        <div style={{ flex: 1, display: 'flex', flexDirection: 'column', minHeight: 0 }}>
          {/* Top row */}
          <div style={{ flex: splitRatioH, display: 'flex', minHeight: 0 }}>
            <div style={{ flex: splitRatioV, display: 'flex', flexDirection: 'column', minHeight: 0, overflow: 'hidden' }}>
              {renderPane(paneTabIds[0])}
            </div>
            <div className="split-divider-v" onMouseDown={handleDividerVMouseDown} onDoubleClick={() => setSplitRatioV(0.5)} title="Double-click to reset" />
            <div style={{ flex: 1 - splitRatioV, display: 'flex', flexDirection: 'column', minHeight: 0, overflow: 'hidden' }}>
              {renderPane(paneTabIds[1])}
            </div>
          </div>
          {/* Horizontal divider */}
          <div className="split-divider-h" onMouseDown={handleDividerHMouseDown} onDoubleClick={() => setSplitRatioH(0.5)} title="Double-click to reset" />
          {/* Bottom row */}
          <div style={{ flex: 1 - splitRatioH, display: 'flex', minHeight: 0 }}>
            <div style={{ flex: splitRatioV, display: 'flex', flexDirection: 'column', minHeight: 0, overflow: 'hidden' }}>
              {renderPane(paneTabIds[2])}
            </div>
            <div className="split-divider-v" onMouseDown={handleDividerVMouseDown} onDoubleClick={() => setSplitRatioV(0.5)} title="Double-click to reset" />
            <div style={{ flex: 1 - splitRatioV, display: 'flex', flexDirection: 'column', minHeight: 0, overflow: 'hidden' }}>
              {renderPane(paneTabIds[3])}
            </div>
          </div>
        </div>
      )}

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
