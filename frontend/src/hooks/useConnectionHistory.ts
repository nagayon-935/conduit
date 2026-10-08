import { useState, useCallback } from 'react';
import type { HistoryEntry, AuthType } from '../types';
import { readJSON, writeJSON, removeKey } from '../utils/storage';
import { STORAGE_KEYS, MAX_HISTORY_ENTRIES } from '../constants';

function loadHistory(): HistoryEntry[] {
  const raw = readJSON<HistoryEntry[]>(STORAGE_KEYS.HISTORY, []);
  // Default authType to 'vault' for old entries that lack it
  return raw.map((e) => ({ ...e, authType: e.authType ?? 'vault' }));
}

export function useConnectionHistory() {
  const [history, setHistory] = useState<HistoryEntry[]>(loadHistory);

  const addEntry = useCallback((host: string, port: number, user: string, authType: AuthType) => {
    setHistory((prev) => {
      const entry: HistoryEntry = {
        host,
        port,
        user,
        authType,
        connectedAt: new Date().toISOString(),
      };
      const filtered = prev.filter(
        (h) => !(h.host === host && h.port === port && h.user === user),
      );
      const updated = [entry, ...filtered].slice(0, MAX_HISTORY_ENTRIES);
      writeJSON(STORAGE_KEYS.HISTORY, updated);
      return updated;
    });
  }, []);

  const clearHistory = useCallback(() => {
    setHistory([]);
    removeKey(STORAGE_KEYS.HISTORY);
  }, []);

  return { history, addEntry, clearHistory };
}
