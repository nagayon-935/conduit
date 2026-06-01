import { useState, useRef, useEffect, useCallback, type KeyboardEvent as ReactKeyboardEvent } from 'react';
import { loadSearchHistory, pushSearchHistory } from '../utils/searchHistory';

type SearchFn = (query: string, options?: { findNext?: boolean }) => boolean;

export interface SearchController {
  open: boolean;
  query: string;
  resultMsg: string;
  history: string[];
  showHistory: boolean;
  inputRef: React.RefObject<HTMLInputElement>;
  setQuery: (value: string) => void;
  onInputFocus: () => void;
  onInputBlur: () => void;
  onInputKeyDown: (e: ReactKeyboardEvent<HTMLInputElement>) => void;
  findNext: () => void;
  findPrevious: () => void;
  selectHistory: (query: string) => void;
  close: () => void;
}

const FOCUS_DELAY_MS = 50;
const BLUR_HIDE_DELAY_MS = 150;

/**
 * Owns the terminal search overlay: open/close (Ctrl+F, Escape), the query,
 * result feedback, and the recent-query history dropdown.
 */
export function useSearchController(search: SearchFn): SearchController {
  const [open, setOpen] = useState(false);
  const [query, setQueryState] = useState('');
  const [resultMsg, setResultMsg] = useState('');
  const [history, setHistory] = useState<string[]>(loadSearchHistory);
  const [showHistory, setShowHistory] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  const close = useCallback(() => setOpen(false), []);

  const setQuery = useCallback((value: string) => {
    setQueryState(value);
    setResultMsg('');
  }, []);

  const runSearch = useCallback((findNext: boolean) => {
    if (!query) return;
    setHistory((prev) => pushSearchHistory(prev, query));
    setShowHistory(false);
    const found = search(query, { findNext });
    setResultMsg(found ? '' : 'No results');
  }, [query, search]);

  const findNext = useCallback(() => runSearch(true), [runSearch]);
  const findPrevious = useCallback(() => runSearch(false), [runSearch]);

  const onInputFocus = useCallback(() => {
    setShowHistory(history.length > 0 && !query);
  }, [history.length, query]);

  const onInputBlur = useCallback(() => {
    setTimeout(() => setShowHistory(false), BLUR_HIDE_DELAY_MS);
  }, []);

  const selectHistory = useCallback((q: string) => {
    setQueryState(q);
    setShowHistory(false);
    setResultMsg('');
    setTimeout(() => inputRef.current?.focus(), 0);
  }, []);

  const onInputKeyDown = useCallback((e: ReactKeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      if (e.shiftKey) findPrevious();
      else findNext();
      return;
    }
    if (e.key === 'ArrowDown' && showHistory) {
      e.preventDefault();
      document.querySelector<HTMLButtonElement>('.search-history-item')?.focus();
      return;
    }
    if (e.key === 'Escape') {
      if (showHistory) setShowHistory(false);
      else setOpen(false);
    }
  }, [findNext, findPrevious, showHistory]);

  // ── Ctrl+F toggle + global Escape ────────────────────────────────────────
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (e.ctrlKey && e.key === 'f') {
        e.preventDefault();
        setOpen((prev) => {
          if (!prev) setTimeout(() => inputRef.current?.focus(), FOCUS_DELAY_MS);
          return !prev;
        });
        return;
      }
      if (e.key === 'Escape') setOpen(false);
    }
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, []);

  return {
    open, query, resultMsg, history, showHistory, inputRef,
    setQuery, onInputFocus, onInputBlur, onInputKeyDown,
    findNext, findPrevious, selectHistory, close,
  };
}
