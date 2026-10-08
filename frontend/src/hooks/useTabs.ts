import { useState, useCallback, useEffect } from 'react';
import type { SessionTab } from '../types';
import { readJSON, writeJSON, removeKey } from '../utils/storage';
import { STORAGE_KEYS } from '../constants';
import { fetchOwnSession } from '../api/sessions';
import { ApiRequestError } from '../api/fetch';

interface Workspace { version: 1; tabs: SessionTab[]; activeTabId: string | null }
const emptyWorkspace = (): Workspace => ({ version: 1, tabs: [], activeTabId: null });

function validTab(value: unknown): value is SessionTab {
  if (!value || typeof value !== 'object') return false;
  const tab = value as SessionTab;
  return typeof tab.id === 'string' && typeof tab.sessionToken === 'string'
    && typeof tab.host === 'string' && typeof tab.user === 'string'
    && Number.isInteger(tab.port) && typeof tab.expiresAt === 'string'
    && (!!tab.sessionToken || typeof tab.shareToken === 'string');
}

// Persist connection descriptions, never form passwords, key content or passphrases.
function storedTab(tab: SessionTab): SessionTab {
  const { id, sessionToken, host, port, user, expiresAt, shareToken, name, authType,
    jumpHost, jumpPort, jumpUser, jumpAuthType, privateKeyName, jumpPrivateKeyName,
    tag, color, paused, ended, endReason, gracePeriodSeconds } = tab;
  return { id, sessionToken, host, port, user, expiresAt, shareToken, name, authType,
    jumpHost, jumpPort, jumpUser, jumpAuthType, privateKeyName, jumpPrivateKeyName,
    tag, color, paused, ended, endReason, gracePeriodSeconds };
}

function loadStoredWorkspace(): Workspace {
  const raw = readJSON<Workspace | null>(STORAGE_KEYS.WORKSPACE, null);
  if (raw?.version === 1 && Array.isArray(raw.tabs)) {
    const tabs = raw.tabs.filter(validTab).slice(0, 32).map(storedTab);
    const activeTabId = tabs.find((t) => t.id === raw.activeTabId && !t.paused)?.id
      ?? tabs.find((t) => !t.paused)?.id ?? null;
    return { version: 1, tabs, activeTabId };
  }
  const legacy = readJSON<{ token?: string; host?: string; port?: number; user?: string; expiresAt?: string } | null>(STORAGE_KEYS.SESSION, null);
  if (legacy?.token && legacy.host && legacy.user && legacy.port) {
    const id = crypto.randomUUID();
    return { version: 1, activeTabId: id, tabs: [{ id, sessionToken: legacy.token,
      host: legacy.host, port: legacy.port, user: legacy.user, expiresAt: legacy.expiresAt ?? '' }] };
  }
  return emptyWorkspace();
}

export function loadWorkspace(): Workspace {
  const workspace = loadStoredWorkspace();
  const shareToken = new URLSearchParams(window.location.search).get('share');
  if (!shareToken) return workspace;
  const existing = workspace.tabs.find((tab) => tab.shareToken === shareToken);
  if (existing) return { ...workspace, activeTabId: existing.id, tabs: workspace.tabs.map((tab) => tab.id === existing.id ? { ...tab, paused: false } : tab) };
  const id = crypto.randomUUID();
  return { ...workspace, activeTabId: id, tabs: [...workspace.tabs, { id, sessionToken: '', shareToken,
    host: '共有セッション', port: 22, user: '', expiresAt: '' }] };
}

export function useTabs() {
  const [workspace, setWorkspace] = useState<Workspace>(loadWorkspace);
  const { tabs, activeTabId } = workspace;
  useEffect(() => {
    if (new URLSearchParams(window.location.search).has('share')) {
      window.history.replaceState(null, '', window.location.pathname);
    }
  }, []);
  useEffect(() => {
    writeJSON(STORAGE_KEYS.WORKSPACE, { ...workspace, tabs: tabs.map(storedTab) });
    removeKey(STORAGE_KEYS.SESSION);
  }, [workspace, tabs]);

  const updateTab = useCallback((id: string, patch: Partial<SessionTab>) => {
    setWorkspace((prev) => ({ ...prev, tabs: prev.tabs.map((t) => t.id === id ? { ...t, ...patch } : t) }));
  }, []);

  // Local timestamps are hints. Only the server decides whether a session lives.
  useEffect(() => {
    const controller = new AbortController();
    for (const tab of workspace.tabs) {
      if (!tab.sessionToken || tab.ended) continue;
      fetchOwnSession(tab.sessionToken, controller.signal).then((info) => {
        if (controller.signal.aborted) return;
        updateTab(tab.id, { expiresAt: info.expires_at });
      }).catch((err: unknown) => {
        if (!controller.signal.aborted && err instanceof ApiRequestError && err.status === 410) {
          updateTab(tab.id, { ended: true, endReason: 'セッションが終了したか、再接続期限を過ぎています。' });
        }
      });
    }
    return () => controller.abort();
    // Validate the restored snapshot once; polling is owned by each terminal.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [updateTab]);

  const selectTab = useCallback((id: string) => {
    setWorkspace((prev) => prev.tabs.some((t) => t.id === id && !t.paused) ? { ...prev, activeTabId: id } : prev);
  }, []);
  const addTab = useCallback((tab: SessionTab) => {
    setWorkspace((prev) => ({ ...prev, tabs: [...prev.tabs, tab], activeTabId: tab.id }));
  }, []);
  const removeTab = useCallback((id: string) => {
    setWorkspace((prev) => {
      const tabs = prev.tabs.filter((t) => t.id !== id);
      const idx = prev.tabs.findIndex((t) => t.id === id);
      const activeTabId = prev.activeTabId !== id ? prev.activeTabId
        : [...prev.tabs.slice(0, idx).reverse(), ...prev.tabs.slice(idx + 1)].find((t) => !t.paused)?.id ?? null;
      return { ...prev, tabs, activeTabId };
    });
    try { sessionStorage.removeItem(`conduit-output:${id}`); } catch { /* unavailable */ }
  }, []);
  const pauseTab = useCallback((id: string) => {
    setWorkspace((prev) => ({ ...prev,
      tabs: prev.tabs.map((t) => t.id === id ? { ...t, paused: true, expiresAt: new Date(Date.now() + (t.gracePeriodSeconds ?? 900) * 1000).toISOString() } : t),
      activeTabId: prev.activeTabId === id ? prev.tabs.find((t) => t.id !== id && !t.paused)?.id ?? null : prev.activeTabId,
    }));
  }, []);
  const resumeTab = useCallback((id: string) => {
    setWorkspace((prev) => ({ ...prev, activeTabId: id,
      tabs: prev.tabs.map((t) => t.id === id ? { ...t, paused: false } : t),
    }));
  }, []);
  const reorderTabs = useCallback((fromId: string, toId: string) => {
    setWorkspace((prev) => {
      const tabs = [...prev.tabs];
      const from = tabs.findIndex((t) => t.id === fromId), to = tabs.findIndex((t) => t.id === toId);
      if (from < 0 || to < 0 || from === to) return prev;
      const [moved] = tabs.splice(from, 1);
      tabs.splice(to, 0, moved);
      return { ...prev, tabs };
    });
  }, []);
  return { tabs, activeTabId, selectTab, addTab, removeTab, pauseTab, resumeTab, reorderTabs, updateTab };
}
