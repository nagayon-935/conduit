import { SEARCH_HISTORY_KEY, MAX_SEARCH_HISTORY } from '../constants';

/** Loads the recent terminal-search queries from sessionStorage (newest first). */
export function loadSearchHistory(): string[] {
  try {
    return JSON.parse(sessionStorage.getItem(SEARCH_HISTORY_KEY) ?? '[]') as string[];
  } catch {
    return [];
  }
}

/**
 * Returns a new history list with `query` moved to the front (deduplicated),
 * capped at MAX_SEARCH_HISTORY, and persists it to sessionStorage.
 * A blank query is ignored and the previous list is returned unchanged.
 */
export function pushSearchHistory(prev: string[], query: string): string[] {
  if (!query.trim()) return prev;
  const filtered = prev.filter((q) => q !== query);
  const updated = [query, ...filtered].slice(0, MAX_SEARCH_HISTORY);
  try {
    sessionStorage.setItem(SEARCH_HISTORY_KEY, JSON.stringify(updated));
  } catch {
    // ignore storage errors
  }
  return updated;
}
