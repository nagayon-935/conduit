import type { SearchController } from '../hooks/useSearchController';

/** Presentational search bar driven entirely by a SearchController. */
export function SearchOverlay({ controller }: { controller: SearchController }) {
  const {
    query, resultMsg, history, showHistory, inputRef,
    setQuery, onInputFocus, onInputBlur, onInputKeyDown,
    findNext, findPrevious, selectHistory, close,
  } = controller;

  return (
    <div className="search-overlay">
      <div className="search-input-wrap">
        <input
          ref={inputRef}
          className="search-input"
          type="text"
          placeholder="Search..."
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onFocus={onInputFocus}
          onBlur={onInputBlur}
          onKeyDown={onInputKeyDown}
        />
        {showHistory && history.length > 0 && (
          <div className="search-history-dropdown">
            {history.map((q) => (
              <button
                key={q}
                type="button"
                className="search-history-item"
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => selectHistory(q)}
              >
                <span className="search-history-icon" aria-hidden="true">⟳</span>
                {q}
              </button>
            ))}
          </div>
        )}
      </div>
      <button type="button" className="search-btn" onClick={findPrevious} title="Previous match (Shift+Enter)">
        ↑
      </button>
      <button type="button" className="search-btn" onClick={findNext} title="Next match (Enter)">
        ↓
      </button>
      {resultMsg && <span className="search-result-msg">{resultMsg}</span>}
      <button type="button" className="search-close-btn" onClick={close} title="Close search (Escape)">
        ✕
      </button>
    </div>
  );
}
