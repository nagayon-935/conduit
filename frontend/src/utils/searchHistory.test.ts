import { describe, it, expect, beforeEach } from 'vitest';
import { loadSearchHistory, pushSearchHistory } from './searchHistory';
import { SEARCH_HISTORY_KEY, MAX_SEARCH_HISTORY } from '../constants';

describe('searchHistory', () => {
  beforeEach(() => {
    sessionStorage.clear();
  });

  it('loads an empty list when nothing is stored', () => {
    expect(loadSearchHistory()).toEqual([]);
  });

  it('returns [] on malformed JSON', () => {
    sessionStorage.setItem(SEARCH_HISTORY_KEY, '{not json');
    expect(loadSearchHistory()).toEqual([]);
  });

  it('prepends a new query and persists it', () => {
    const next = pushSearchHistory([], 'foo');
    expect(next).toEqual(['foo']);
    expect(loadSearchHistory()).toEqual(['foo']);
  });

  it('moves an existing query to the front without duplicating', () => {
    const next = pushSearchHistory(['a', 'b', 'c'], 'c');
    expect(next).toEqual(['c', 'a', 'b']);
  });

  it('ignores blank queries', () => {
    const prev = ['a'];
    expect(pushSearchHistory(prev, '   ')).toBe(prev);
  });

  it('caps the history at MAX_SEARCH_HISTORY', () => {
    let h: string[] = [];
    for (let i = 0; i < MAX_SEARCH_HISTORY + 3; i++) {
      h = pushSearchHistory(h, `q${i}`);
    }
    expect(h).toHaveLength(MAX_SEARCH_HISTORY);
    expect(h[0]).toBe(`q${MAX_SEARCH_HISTORY + 2}`);
  });
});
