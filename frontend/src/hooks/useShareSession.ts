import { useState, useRef, useEffect, useCallback } from 'react';
import { shareSession, revokeShare } from '../api/sessions';

const COPIED_FEEDBACK_MS = 2000;

export interface UseShareSessionResult {
  activeShareToken: string | null;
  shareCopied: boolean;
  /** Issues a share token (first call) or copies the existing share URL. */
  share: () => Promise<void>;
  /** Revokes the active share token. */
  revoke: () => Promise<void>;
}

/**
 * Manages read-only session sharing: issuing a share token, copying the share
 * URL to the clipboard with transient "Copied!" feedback, and revoking it.
 */
export function useShareSession(sessionToken: string): UseShareSessionResult {
  const [activeShareToken, setActiveShareToken] = useState<string | null>(null);
  const [shareCopied, setShareCopied] = useState(false);
  const copyTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => () => {
    if (copyTimerRef.current) clearTimeout(copyTimerRef.current);
  }, []);

  const flashCopied = useCallback(() => {
    setShareCopied(true);
    if (copyTimerRef.current) clearTimeout(copyTimerRef.current);
    copyTimerRef.current = setTimeout(() => setShareCopied(false), COPIED_FEEDBACK_MS);
  }, []);

  const share = useCallback(async () => {
    if (activeShareToken) {
      const url = `${window.location.origin}/?share=${activeShareToken}`;
      await navigator.clipboard.writeText(url).catch(() => {});
      flashCopied();
      return;
    }
    try {
      const res = await shareSession(sessionToken);
      setActiveShareToken(res.share_token);
      await navigator.clipboard.writeText(res.url).catch(() => {});
      flashCopied();
    } catch {
      // silently ignore
    }
  }, [activeShareToken, sessionToken, flashCopied]);

  const revoke = useCallback(async () => {
    if (!activeShareToken) return;
    await revokeShare(sessionToken, activeShareToken).catch(() => {});
    setActiveShareToken(null);
  }, [activeShareToken, sessionToken]);

  return { activeShareToken, shareCopied, share, revoke };
}
