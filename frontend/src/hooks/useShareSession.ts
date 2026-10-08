import { useState, useRef, useEffect, useCallback } from 'react';
import { shareSession, revokeShare, fetchShares } from '../api/sessions';
import type { ShareLink } from '../types';

export function useShareSession(sessionToken: string) {
  const [links, setLinks] = useState<ShareLink[]>([]);
  const [shareCopied, setShareCopied] = useState(false);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [ttlSeconds, setTTLSeconds] = useState(3600);
  const timer = useRef<ReturnType<typeof setTimeout>>();
  const revision = useRef(0);
  const [now, setNow] = useState(Date.now);
  const working = useRef(false), alive = useRef(true);
  const activeLinks = links.filter((entry) => new Date(entry.expires_at).getTime() > now);
  const link = activeLinks[0];
  const shareURL = link ? `${window.location.origin}/app?share=${encodeURIComponent(link.share_token)}` : '';
  useEffect(() => {
    alive.current = true;
    const generation = revision.current;
    const ticker = setInterval(() => setNow(Date.now()), 1000);
    const controller = new AbortController();
    if (sessionToken) fetchShares(sessionToken, controller.signal).then((result) => {
      if (!controller.signal.aborted && generation === revision.current) setLinks(result);
    }).catch(() => {});
    return () => { alive.current = false; controller.abort(); clearInterval(ticker); clearTimeout(timer.current); };
  }, [sessionToken]);

  const share = useCallback(async () => {
    if (!sessionToken || working.current) return;
    working.current = true; revision.current++; setBusy(true); setError(''); setShareCopied(false);
    try {
      let current = links.find((entry) => new Date(entry.expires_at).getTime() > Date.now());
      if (!current) {
        current = await shareSession(sessionToken, ttlSeconds);
        if (alive.current) setLinks((prev) => [...prev, current!]);
      }
      const url = `${window.location.origin}/app?share=${encodeURIComponent(current.share_token)}`;
      try {
        await navigator.clipboard.writeText(url);
        if (alive.current) {
          setShareCopied(true); clearTimeout(timer.current);
          timer.current = setTimeout(() => setShareCopied(false), 2000);
        }
      } catch { if (alive.current) setError('コピーできませんでした。下のリンクを選択して手動でコピーしてください。'); }
    } catch (err) { if (alive.current) setError(err instanceof Error ? err.message : '共有リンクを作成できませんでした。'); }
    finally { working.current = false; if (alive.current) setBusy(false); }
  }, [links, sessionToken, ttlSeconds]);
  const revoke = useCallback(async () => {
    if (working.current) return;
    working.current = true; revision.current++; setBusy(true); setError('');
    try {
      const current = await fetchShares(sessionToken);
      const results = await Promise.allSettled(current.map((entry) => revokeShare(sessionToken, entry.share_token)));
      const remaining = await fetchShares(sessionToken);
      if (alive.current) {
        setLinks(remaining); setShareCopied(false);
        if (results.some((result) => result.status === 'rejected')) setError('一部の共有リンクを停止できませんでした。再試行してください。');
      }
    } catch (err) { if (alive.current) setError(err instanceof Error ? err.message : '共有を停止できませんでした。'); }
    finally { working.current = false; if (alive.current) setBusy(false); }
  }, [sessionToken]);
  return { activeShareToken: link?.share_token ?? null, shareCopied, shareURL, expiresAt: link?.expires_at,
    linkCount: activeLinks.length, share, revoke, error, busy, ttlSeconds, setTTLSeconds };
}
