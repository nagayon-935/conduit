import { useState, useCallback, useEffect } from 'react';
import type { SessionTab } from '../types';
import { saveSession, loadSession, clearSession } from '../utils/session';

export interface UseTabsResult {
  tabs: SessionTab[];
  activeTabId: string | null;
  selectTab: (id: string) => void;
  addTab: (tab: SessionTab) => void;
  removeTab: (id: string) => void;
  reorderTabs: (fromId: string, toId: string) => void;
}

/**
 * Owns the tab list, the active tab, and the localStorage mirror of the
 * current session. On mount it restores either a shared-viewer tab (from a
 * `?share=` URL) or the last persisted session.
 */
export function useTabs(): UseTabsResult {
  const [tabs, setTabs] = useState<SessionTab[]>([]);
  const [activeTabId, setActiveTabId] = useState<string | null>(null);

  // ── Restore on mount (viewer mode via ?share=, else stored session) ──────
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const shareToken = params.get('share');

    if (shareToken) {
      // Remove ?share from the URL without reloading (keeps the page clean).
      window.history.replaceState(null, '', window.location.pathname);
      const id = crypto.randomUUID();
      setTabs([{
        id,
        // sessionToken is unknown from the share URL; the WS uses shareToken directly.
        sessionToken: '',
        host: 'shared session',
        port: 22,
        user: '',
        expiresAt: '',
        shareToken,
      }]);
      setActiveTabId(id);
      return;
    }

    const stored = loadSession();
    if (stored) {
      const id = crypto.randomUUID();
      setTabs([{
        id,
        sessionToken: stored.token,
        host: stored.host,
        port: stored.port,
        user: stored.user,
        expiresAt: stored.expiresAt,
      }]);
      setActiveTabId(id);
    }
  }, []);

  const selectTab = useCallback((id: string) => setActiveTabId(id), []);

  const addTab = useCallback((tab: SessionTab) => {
    setTabs((prev) => [...prev, tab]);
    setActiveTabId(tab.id);
    if (tab.sessionToken) {
      saveSession({ token: tab.sessionToken, expiresAt: tab.expiresAt, host: tab.host, port: tab.port, user: tab.user });
    }
  }, []);

  const removeTab = useCallback((id: string) => {
    setTabs((prev) => {
      const remaining = prev.filter((t) => t.id !== id);
      if (remaining.length === 0) {
        clearSession();
        setActiveTabId(null);
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
  }, []);

  const reorderTabs = useCallback((fromId: string, toId: string) => {
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

  return { tabs, activeTabId, selectTab, addTab, removeTab, reorderTabs };
}
