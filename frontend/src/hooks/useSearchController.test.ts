import { describe, it, expect, beforeEach, vi } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useSearchController } from './useSearchController';

describe('useSearchController', () => {
  beforeEach(() => {
    sessionStorage.clear();
  });

  it('starts closed with an empty query', () => {
    const { result } = renderHook(() => useSearchController(() => true));
    expect(result.current.open).toBe(false);
    expect(result.current.query).toBe('');
  });

  it('opens on Ctrl+F and closes on Escape', () => {
    const { result } = renderHook(() => useSearchController(() => true));
    act(() => {
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'f', ctrlKey: true }));
    });
    expect(result.current.open).toBe(true);
    act(() => {
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    });
    expect(result.current.open).toBe(false);
  });

  it('setQuery updates the query and clears the result message', () => {
    const { result } = renderHook(() => useSearchController(() => false));
    act(() => result.current.setQuery('foo'));
    expect(result.current.query).toBe('foo');
    act(() => result.current.findNext());
    expect(result.current.resultMsg).toBe('No results');
    act(() => result.current.setQuery('bar'));
    expect(result.current.resultMsg).toBe('');
  });

  it('runs the search and records history on findNext', () => {
    const search = vi.fn(() => true);
    const { result } = renderHook(() => useSearchController(search));
    act(() => result.current.setQuery('needle'));
    act(() => result.current.findNext());
    expect(search).toHaveBeenCalledWith('needle', { findNext: true });
    expect(result.current.history).toContain('needle');
    expect(result.current.resultMsg).toBe('');
  });

  it('does not search when the query is blank', () => {
    const search = vi.fn(() => true);
    const { result } = renderHook(() => useSearchController(search));
    act(() => result.current.findNext());
    expect(search).not.toHaveBeenCalled();
  });
});
