import { beforeEach, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import { connectToHost } from '../api/connect';
import { useConnectionJobs } from './useConnectionJobs';
import { defaultFields } from '../utils/form';
vi.mock('../api/connect', () => ({ connectToHost: vi.fn() }));
beforeEach(() => { vi.mocked(connectToHost).mockReset(); localStorage.clear(); });
it('opens each successful host immediately and retries only the failed host', async () => {
  let completeSlow!: (response: never) => void;
  vi.mocked(connectToHost).mockImplementation(async (request) => {
    if (request.host === 'slow') return new Promise((resolve) => { completeSlow = resolve; });
    if (request.host === 'failed') throw new Error('認証失敗');
    return { session_token: 'fast-token', expires_at: '', message: 'connected' };
  });
  const onConnected = vi.fn(); const { result } = renderHook(() => useConnectionJobs(onConnected));
  act(() => result.current.connectEntries(['fast', 'slow', 'failed'].map((host) => ({ ...defaultFields(), host, user: 'u', authType: 'password', password: 'transient-secret' }))));
  await waitFor(() => expect(onConnected).toHaveBeenCalledTimes(1));
  expect(result.current.jobs.map((j) => j.state)).toEqual(['connected', 'connecting', 'failed']);
  expect(JSON.stringify(result.current.jobs)).not.toContain('transient-secret'); expect(localStorage.length).toBe(0);
  vi.mocked(connectToHost).mockResolvedValueOnce({ session_token: 'retry-token', expires_at: '', message: 'connected' });
  act(() => result.current.retry(result.current.jobs[2].id));
  await waitFor(() => expect(onConnected).toHaveBeenCalledTimes(2));
  expect(vi.mocked(connectToHost).mock.calls.map(([request]) => request.host)).toEqual(['fast', 'slow', 'failed', 'failed']);
  await act(async () => completeSlow({ session_token: 'slow-token', expires_at: '', message: 'connected' } as never));
  expect(onConnected).toHaveBeenCalledTimes(3);
});
it('aborts cancellation and releases credentials when a result is dismissed', async () => {
  vi.mocked(connectToHost).mockImplementation((_req, signal) => new Promise((_resolve, reject) => signal?.addEventListener('abort', () => reject(new Error('cancelled')))));
  const onConnected = vi.fn(); const { result } = renderHook(() => useConnectionJobs(onConnected));
  act(() => result.current.connectEntries([{ ...defaultFields(), host: 'h', user: 'u' }]));
  const id = result.current.jobs[0].id;
  act(() => result.current.cancel(id));
  await waitFor(() => expect(result.current.jobs[0].state).toBe('cancelled'));
  expect(result.current.edit(id)).toBeUndefined(); expect(onConnected).not.toHaveBeenCalled();
  act(() => result.current.dismiss(id)); expect(result.current.jobs).toEqual([]);
});
