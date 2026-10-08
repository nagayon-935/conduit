import { beforeEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import { useTabs } from './useTabs';
import { useSplitLayout } from './useSplitLayout';
import { useConnectionHistory } from './useConnectionHistory';
import { useSearchController } from './useSearchController';
import { useProfiles } from './useProfiles';
import { STORAGE_KEYS } from '../constants';
import { fetchOwnSession } from '../api/sessions';
import { ApiRequestError } from '../api/fetch';
vi.mock('../api/sessions', () => ({ fetchOwnSession: vi.fn() }));
vi.mock('../utils/crypto', () => ({ encryptText: async (text: string) => `encrypted-${text}`, decryptText: async () => null }));
const tab = (id: string) => ({ id, sessionToken: `secret-${id}`, host: 'host', port: 22, user: 'user', expiresAt: '2000-01-01T00:00:00Z' });
beforeEach(() => { localStorage.clear(); sessionStorage.clear(); window.history.replaceState(null, '', '/app'); vi.mocked(fetchOwnSession).mockReset(); vi.mocked(fetchOwnSession).mockResolvedValue({ expires_at: '2099-01-01T00:00:00Z' } as never); });
describe('restored workspace', () => {
  it('restores every tab, order and selection despite a stale local timestamp; the server decides expiry', async () => {
    localStorage.setItem(STORAGE_KEYS.WORKSPACE, JSON.stringify({ version: 1, tabs: [tab('b'), tab('a')], activeTabId: 'a' }));
    vi.mocked(fetchOwnSession).mockImplementation(async (token) => { if (token === 'secret-b') throw new ApiRequestError('gone', 410); return { expires_at: '2099-01-01T00:00:00Z' } as never; });
    const { result } = renderHook(useTabs);
    expect(result.current.tabs.map((t) => t.id)).toEqual(['b', 'a']);
    expect(result.current.activeTabId).toBe('a');
    await waitFor(() => expect(result.current.tabs[0].ended).toBe(true));
    expect(result.current.tabs[1].ended).toBeUndefined();
    expect(result.current.tabs[1].expiresAt).toContain('2099');
  });
  it('opening a share preserves existing owner tabs and saves no form secrets', async () => {
    localStorage.setItem(STORAGE_KEYS.WORKSPACE, JSON.stringify({ version: 1, tabs: [{ ...tab('a'), password: 'never-save', privateKey: 'never-save' }], activeTabId: 'a' }));
    window.history.replaceState(null, '', '/app?share=viewer');
    const { result } = renderHook(useTabs);
    expect(result.current.tabs).toHaveLength(2);
    expect(result.current.tabs[1].shareToken).toBe('viewer');
    expect(localStorage.getItem(STORAGE_KEYS.WORKSPACE)).not.toContain('never-save');
    await act(async () => {});
  });
  it('pausing preserves the session and resuming selects it', () => {
    const { result } = renderHook(useTabs);
    act(() => { result.current.addTab(tab('a')); result.current.addTab(tab('b')); });
    act(() => result.current.pauseTab('b'));
    expect(result.current.activeTabId).toBe('a');
    expect(result.current.tabs[1].paused).toBe(true);
    act(() => result.current.resumeTab('b'));
    expect(result.current.activeTabId).toBe('b');
  });
  it('restores pane assignments and ratios, and shows a selected tab in the active pane', () => {
    localStorage.setItem(STORAGE_KEYS.LAYOUT, JSON.stringify({ layoutType: '2v', paneTabIds: ['b', 'a', null, null], splitRatioV: 0.35, splitRatioH: 0.6 }));
    const { result } = renderHook(() => useSplitLayout(['a', 'b', 'c'], 'a'));
    expect(result.current.layoutType).toBe('2v'); expect(result.current.splitRatioV).toBe(0.35);
    act(() => result.current.showTab('c'));
    expect(result.current.paneTabIds.slice(0, 2)).toEqual(['b', 'c']);
    act(() => result.current.fillEmptyPane('a'));
    expect(result.current.paneTabIds.slice(0, 2)).toContain('a');
    expect(result.current.paneTabIds.slice(2)).toEqual([null, null]);
  });
});
it('records the latest successful authentication instead of preserving an earlier Vault method', () => {
  const { result } = renderHook(useConnectionHistory);
  act(() => result.current.addEntry('host', 22, 'user', 'vault'));
  act(() => result.current.addEntry('host', 22, 'user', 'password'));
  expect(result.current.history).toHaveLength(1); expect(result.current.history[0].authType).toBe('password');
});
it('only the active terminal handles search, leaves Ctrl+F to SSH, and restores focus on close', () => {
  const focus = vi.fn();
  const { result, rerender } = renderHook(({ active }) => useSearchController(vi.fn(), active, focus), { initialProps: { active: false } });
  act(() => window.dispatchEvent(new KeyboardEvent('keydown', { key: 'f', metaKey: true })));
  expect(result.current.open).toBe(false);
  rerender({ active: true });
  act(() => window.dispatchEvent(new KeyboardEvent('keydown', { key: 'f', ctrlKey: true })));
  expect(result.current.open).toBe(false);
  act(() => window.dispatchEvent(new KeyboardEvent('keydown', { key: 'f', metaKey: true })));
  expect(result.current.open).toBe(true);
  act(() => result.current.close()); expect(focus).toHaveBeenCalledOnce();
});
it('does not persist private keys without opt-in and clears keys when opt-in is removed', async () => {
  const { result } = renderHook(useProfiles);
  act(() => result.current.saveProfile('a', 'host', 22, 'user', 'pubkey', undefined, { privateKeyContent: 'SECRET-A' }));
  await waitFor(() => expect(localStorage.getItem(STORAGE_KEYS.PROFILES)).toContain('host'));
  expect(localStorage.getItem(STORAGE_KEYS.PROFILES)).not.toContain('SECRET-A');
  act(() => result.current.saveProfile('b', 'host', 22, 'user', 'pubkey', undefined, { privateKeyContent: 'SECRET-B' }, { rememberKeys: true }));
  await waitFor(() => expect(localStorage.getItem(STORAGE_KEYS.PROFILES)).toContain('encrypted-SECRET-B'));
  const id = result.current.profiles[0].id;
  act(() => result.current.updateProfile(id, { rememberKeys: false }));
  await waitFor(() => expect(localStorage.getItem(STORAGE_KEYS.PROFILES)).not.toContain('SECRET-B'));
});
