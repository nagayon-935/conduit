import { useState, useCallback } from 'react';
import { ConnectForm } from './components/ConnectForm';
import { SessionList } from './components/SessionList';
import { LogPage } from './components/LogPage';
import { TabBar } from './components/TabBar';
import { TerminalPool } from './components/TerminalPool';
import { NewConnectionOverlay } from './components/NewConnectionOverlay';
import type { AppState, AuthType, SessionTab } from './types';
import { useConnectionHistory } from './hooks/useConnectionHistory';
import { useProfiles } from './hooks/useProfiles';
import { useSessionCount } from './hooks/useSessionCount';
import { useTabs } from './hooks/useTabs';
import { useSplitLayout } from './hooks/useSplitLayout';
import './App.css';

type ViewState = 'main' | 'sessions' | 'logs';

export default function App() {
  const [appState, setAppState] = useState<AppState>('idle');
  const [viewState, setViewState] = useState<ViewState>('main');
  const [showOverlay, setShowOverlay] = useState(false);

  const { history, addEntry } = useConnectionHistory();
  const { profiles } = useProfiles();
  const sessionCount = useSessionCount();

  const { tabs, activeTabId, selectTab, addTab, removeTab, reorderTabs } = useTabs();
  const layout = useSplitLayout(tabs.map((t) => t.id), activeTabId);
  const { fillEmptyPane, releasePane } = layout;

  // ── Connect ───────────────────────────────────────────────────────────────
  const handleConnect = useCallback(
    (token: string, expiresAt: string, host: string, port: number, user: string, authType: AuthType) => {
      const tab: SessionTab = { id: crypto.randomUUID(), sessionToken: token, host, port, user, expiresAt };
      addTab(tab);
      addEntry(host, port, user, authType);
      fillEmptyPane(tab.id);
      setShowOverlay(false);
      setAppState('idle');
      setViewState('main');
    },
    [addTab, addEntry, fillEmptyPane],
  );

  // ── Close tab ───────────────────────────────────────────────────────────────
  const handleCloseTab = useCallback(
    (id: string) => {
      releasePane(id);
      removeTab(id);
    },
    [releasePane, removeTab],
  );

  // ── Views: sessions / logs ──────────────────────────────────────────────────
  if (viewState === 'sessions') {
    return <SessionList onBack={() => setViewState('main')} />;
  }
  if (viewState === 'logs') {
    return <LogPage onBack={() => setViewState('main')} />;
  }

  // ── View: no tabs — show ConnectForm ─────────────────────────────────────────
  if (tabs.length === 0) {
    return (
      <ConnectForm
        appState={appState}
        onConnect={handleConnect}
        onStateChange={setAppState}
        history={history}
        onShowSessions={() => setViewState('sessions')}
        onShowLogs={() => setViewState('logs')}
        sessionCount={sessionCount}
      />
    );
  }

  // ── View: terminal layout ────────────────────────────────────────────────────
  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100vh' }}>
      <TabBar
        tabs={tabs.map(({ id, host, port, user }) => ({ id, host, port, user }))}
        activeId={activeTabId}
        onSelect={selectTab}
        onClose={handleCloseTab}
        onNew={() => setShowOverlay(true)}
        layoutType={layout.layoutType}
        paneTabIds={layout.paneTabIds}
        onLayoutChange={layout.switchLayout}
        profiles={profiles}
        onReorder={reorderTabs}
      />

      <TerminalPool
        tabs={tabs}
        layoutType={layout.layoutType}
        paneTabIds={layout.paneTabIds}
        activeTabId={activeTabId}
        splitRatioV={layout.splitRatioV}
        splitRatioH={layout.splitRatioH}
        onCloseTab={handleCloseTab}
        onDividerVMouseDown={layout.onDividerVMouseDown}
        onDividerHMouseDown={layout.onDividerHMouseDown}
        onResetRatioV={layout.resetRatioV}
        onResetRatioH={layout.resetRatioH}
      />

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
