import { describe, it, expect, beforeEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useTabs } from './useTabs';
import type { SessionTab } from '../types';

function makeTab(id: string, token = `tok-${id}`): SessionTab {
  return { id, sessionToken: token, host: 'h', port: 22, user: 'u', expiresAt: '' };
}

describe('useTabs', () => {
  beforeEach(() => {
    localStorage.clear();
    window.history.replaceState(null, '', '/');
  });

  it('starts empty with no stored session', () => {
    const { result } = renderHook(() => useTabs());
    expect(result.current.tabs).toEqual([]);
    expect(result.current.activeTabId).toBeNull();
  });

  it('adds a tab and makes it active', () => {
    const { result } = renderHook(() => useTabs());
    act(() => result.current.addTab(makeTab('a')));
    expect(result.current.tabs.map((t) => t.id)).toEqual(['a']);
    expect(result.current.activeTabId).toBe('a');
  });

  it('reassigns active tab to a neighbor when the active tab is removed', () => {
    const { result } = renderHook(() => useTabs());
    act(() => result.current.addTab(makeTab('a')));
    act(() => result.current.addTab(makeTab('b')));
    act(() => result.current.addTab(makeTab('c')));
    act(() => result.current.selectTab('b'));
    act(() => result.current.removeTab('b'));
    expect(result.current.tabs.map((t) => t.id)).toEqual(['a', 'c']);
    expect(result.current.activeTabId).toBe('a');
  });

  it('clears active tab when the last tab is removed', () => {
    const { result } = renderHook(() => useTabs());
    act(() => result.current.addTab(makeTab('a')));
    act(() => result.current.removeTab('a'));
    expect(result.current.tabs).toEqual([]);
    expect(result.current.activeTabId).toBeNull();
  });

  it('reorders tabs by moving one before another', () => {
    const { result } = renderHook(() => useTabs());
    act(() => result.current.addTab(makeTab('a')));
    act(() => result.current.addTab(makeTab('b')));
    act(() => result.current.addTab(makeTab('c')));
    act(() => result.current.reorderTabs('c', 'a'));
    expect(result.current.tabs.map((t) => t.id)).toEqual(['c', 'a', 'b']);
  });

  it('keeps the active tab unchanged when a non-active tab is removed', () => {
    const { result } = renderHook(() => useTabs());
    act(() => result.current.addTab(makeTab('a')));
    act(() => result.current.addTab(makeTab('b')));
    act(() => result.current.selectTab('a'));
    act(() => result.current.removeTab('b'));
    expect(result.current.activeTabId).toBe('a');
  });
});
