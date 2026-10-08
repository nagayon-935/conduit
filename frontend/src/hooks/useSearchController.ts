import {
  useState,
  useRef,
  useEffect,
  useCallback,
  type KeyboardEvent as ReactKeyboardEvent,
} from "react";
import { MAX_SEARCH_HISTORY } from "../constants";

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
  openSearch: () => void;
}

const FOCUS_DELAY_MS = 50;
const BLUR_HIDE_DELAY_MS = 150;

/**
 * Owns the terminal search overlay: open/close (Ctrl+Shift+F / Cmd+F, Escape), the query,
 * result feedback, and the recent-query history dropdown.
 */
export function useSearchController(
  search: SearchFn,
  active = true,
  onClose?: () => void,
): SearchController {
  const [open, setOpen] = useState(false);
  const [query, setQueryState] = useState("");
  const [resultMsg, setResultMsg] = useState("");
  const [history, setHistory] = useState<string[]>([]);
  const [showHistory, setShowHistory] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;
  const close = useCallback(() => {
    setOpen(false);
    onCloseRef.current?.();
  }, []);
  const openSearch = useCallback(() => setOpen(true), []);
  useEffect(() => {
    if (!open || !active) return;
    const timer = setTimeout(() => inputRef.current?.focus(), FOCUS_DELAY_MS);
    return () => clearTimeout(timer);
  }, [open, active]);

  const setQuery = useCallback((value: string) => {
    setQueryState(value);
    setResultMsg("");
  }, []);

  const runSearch = useCallback(
    (findNext: boolean) => {
      if (!query) return;
      setHistory((prev) =>
        [query, ...prev.filter((value) => value !== query)].slice(
          0,
          MAX_SEARCH_HISTORY,
        ),
      );
      setShowHistory(false);
      const found = search(query, { findNext });
      setResultMsg(found ? "" : "No results");
    },
    [query, search],
  );

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
    setResultMsg("");
    setTimeout(() => inputRef.current?.focus(), 0);
  }, []);

  const onInputKeyDown = useCallback(
    (e: ReactKeyboardEvent<HTMLInputElement>) => {
      if (e.key === "Enter") {
        if (e.shiftKey) findPrevious();
        else findNext();
        return;
      }
      if (e.key === "ArrowDown" && showHistory) {
        e.preventDefault();
        document
          .querySelector<HTMLButtonElement>(".search-history-item")
          ?.focus();
        return;
      }
      if (e.key === "Escape") {
        e.stopPropagation();
        if (showHistory) setShowHistory(false);
        else close();
      }
    },
    [findNext, findPrevious, showHistory, close],
  );

  // ── Ctrl+F toggle + global Escape ────────────────────────────────────────
  useEffect(() => {
    if (!active) return;
    function handleKeyDown(e: KeyboardEvent) {
      if (
        ((e.ctrlKey && e.shiftKey) || e.metaKey) &&
        e.key.toLowerCase() === "f"
      ) {
        e.preventDefault();
        if (open) close();
        else openSearch();
        return;
      }
      if (e.key === "Escape" && open) close();
    }
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [active, open, close, openSearch]);

  return {
    open,
    query,
    resultMsg,
    history,
    showHistory,
    inputRef,
    setQuery,
    onInputFocus,
    onInputBlur,
    onInputKeyDown,
    findNext,
    findPrevious,
    selectHistory,
    close,
    openSearch,
  };
}
