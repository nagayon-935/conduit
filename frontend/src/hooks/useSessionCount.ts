import { useState, useEffect } from 'react';
import { fetchSessions } from '../api/sessions';

/**
 * Polls the active session count for the TabBar badge.
 * Network errors are swallowed — the badge simply stops updating.
 */
export function useSessionCount(intervalMs = 10_000): number | undefined {
  const [count, setCount] = useState<number | undefined>(undefined);

  useEffect(() => {
    let cancelled = false;
    async function refresh() {
      try {
        const data = await fetchSessions();
        if (!cancelled) setCount(data.length);
      } catch {
        // ignore — badge just won't update
      }
    }
    refresh();
    const iv = setInterval(refresh, intervalMs);
    return () => {
      cancelled = true;
      clearInterval(iv);
    };
  }, [intervalMs]);

  return count;
}
