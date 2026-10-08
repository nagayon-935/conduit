import { beforeEach, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import { fetchShares, revokeShare, shareSession } from '../api/sessions';
import { useShareSession } from './useShareSession';
vi.mock('../api/sessions', () => ({ fetchShares: vi.fn(), revokeShare: vi.fn(), shareSession: vi.fn() }));
const link = { url: '/app?share=link', share_token: 'link', expires_at: '2099-01-01T00:00:00Z' };
beforeEach(() => { vi.resetAllMocks(); vi.mocked(fetchShares).mockResolvedValue([]); Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: vi.fn().mockRejectedValue(new Error('denied')) } }); });
it('restores existing links and shows a manual fallback when clipboard access fails', async () => {
  vi.mocked(fetchShares).mockResolvedValue([link]);
  const { result } = renderHook(() => useShareSession('owner'));
  await waitFor(() => expect(result.current.linkCount).toBe(1));
  await act(async () => result.current.share());
  expect(shareSession).not.toHaveBeenCalled(); expect(result.current.shareCopied).toBe(false);
  expect(result.current.shareURL).toContain('/app?share=link'); expect(result.current.error).toContain('手動');
});
it('creates only one link on repeated clicks and keeps it after clipboard failure', async () => {
  let resolve!: (value: typeof link) => void;
  vi.mocked(shareSession).mockImplementation(() => new Promise((r) => { resolve = r; }));
  const { result } = renderHook(() => useShareSession('owner'));
  await act(async () => {});
  let promise!: Promise<void>;
  act(() => { promise = result.current.share(); void result.current.share(); });
  await act(async () => { resolve(link); await promise; });
  expect(shareSession).toHaveBeenCalledOnce(); expect(result.current.activeShareToken).toBe('link');
});
it('retains a failed revocation so it can be retried instead of claiming success', async () => {
  vi.mocked(fetchShares).mockResolvedValue([link]); vi.mocked(revokeShare).mockRejectedValue(new Error('network'));
  const { result } = renderHook(() => useShareSession('owner'));
  await waitFor(() => expect(result.current.linkCount).toBe(1));
  await act(async () => result.current.revoke());
  expect(result.current.activeShareToken).toBe('link'); expect(result.current.error).toContain('再試行');
});
