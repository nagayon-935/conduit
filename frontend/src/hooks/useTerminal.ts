import { useRef, useState, useCallback, useContext, useEffect } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebglAddon } from "@xterm/addon-webgl";
import { SearchAddon } from "@xterm/addon-search";
import { themes, defaultThemeKey, type Theme } from "../themes";
import { TerminalPreferences } from "../portal/TerminalPreferences";
import { FONT_SIZE_MIN, FONT_SIZE_MAX, FONT_SIZE_DEFAULT } from "../constants";

/**
 * Theme objects already match xterm's ITheme shape — destructure only
 * the xterm-relevant fields so extra properties (like `name`) are excluded.
 */
function themeToXterm({
  background,
  foreground,
  cursor,
  selectionBackground,
  black,
  red,
  green,
  yellow,
  blue,
  magenta,
  cyan,
  white,
  brightBlack,
  brightRed,
  brightGreen,
  brightYellow,
  brightBlue,
  brightMagenta,
  brightCyan,
  brightWhite,
}: Theme) {
  return {
    background,
    foreground,
    cursor,
    selectionBackground,
    black,
    red,
    green,
    yellow,
    blue,
    magenta,
    cyan,
    white,
    brightBlack,
    brightRed,
    brightGreen,
    brightYellow,
    brightBlue,
    brightMagenta,
    brightCyan,
    brightWhite,
  };
}

interface UseTerminalOptions {
  themeKey?: string;
}

interface UseTerminalReturn {
  terminalRef: React.RefObject<HTMLDivElement>;
  terminal: Terminal | null;
  fitAddon: FitAddon | null;
  searchAddon: SearchAddon | null;
  initTerminal: () => void;
  disposeTerminal: () => void;
  writeToTerminal: (data: string | Uint8Array) => void;
  resizePTY: () => { cols: number; rows: number } | null;
  changeFontSize: (delta: number) => void;
  setTheme: (key: string) => void;
  currentThemeKey: string;
  search: (query: string, options?: { findNext?: boolean }) => boolean;
}

export function useTerminal(
  options: UseTerminalOptions = {},
): UseTerminalReturn {
  const preferences = useContext(TerminalPreferences);
  const preferencesRef = useRef(preferences);
  preferencesRef.current = preferences;
  const optionsRef = useRef(options);
  optionsRef.current = options;
  const terminalRef = useRef<HTMLDivElement>(null);
  const [terminal, setTerminal] = useState<Terminal | null>(null);
  const [fitAddon, setFitAddon] = useState<FitAddon | null>(null);
  const [searchAddon, setSearchAddon] = useState<SearchAddon | null>(null);
  const [currentThemeKey, setCurrentThemeKey] = useState<string>(
    options.themeKey ?? preferences.theme,
  );

  // Keep mutable refs so callbacks always have access to current values
  // without needing to re-create callbacks on every render
  const terminalInstanceRef = useRef<Terminal | null>(null);
  const fitAddonInstanceRef = useRef<FitAddon | null>(null);
  const searchAddonInstanceRef = useRef<SearchAddon | null>(null);
  const currentThemeKeyRef = useRef<string>(currentThemeKey);

  const initTerminal = useCallback(() => {
    if (!terminalRef.current) return;
    // Avoid double-init
    if (terminalInstanceRef.current) return;

    const initialFontSize = preferencesRef.current.fontSize;
    const initialThemeKey =
      optionsRef.current.themeKey ?? preferencesRef.current.theme;
    const themeObj = themes[initialThemeKey] ?? themes[defaultThemeKey];

    const term = new Terminal({
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      fontSize: initialFontSize,
      cursorBlink: true,
      theme: themeToXterm(themeObj),
      allowProposedApi: true,
    });

    const fit = new FitAddon();
    const search = new SearchAddon();
    term.loadAddon(fit);
    term.loadAddon(search);
    term.open(terminalRef.current);

    // Attempt WebGL renderer; fall back to canvas on failure
    try {
      const webgl = new WebglAddon();
      webgl.onContextLoss(() => {
        webgl.dispose();
      });
      term.loadAddon(webgl);
    } catch {
      // WebGL not supported — canvas renderer is used automatically
    }

    fit.fit();

    // ResizeObserver でコンテナのサイズ変化（display:none→flex 切替含む）を検知して fit()
    const ro = new ResizeObserver(() => {
      if (terminalRef.current && terminalRef.current.offsetParent !== null) {
        fit.fit();
      }
    });
    ro.observe(terminalRef.current);

    (term as Terminal & { _resizeObserver?: ResizeObserver })._resizeObserver =
      ro;

    terminalInstanceRef.current = term;
    fitAddonInstanceRef.current = fit;
    searchAddonInstanceRef.current = search;
    currentThemeKeyRef.current = initialThemeKey;
    setTerminal(term);
    setFitAddon(fit);
    setSearchAddon(search);
    setCurrentThemeKey(initialThemeKey);
  }, []);

  const disposeTerminal = useCallback(() => {
    const term = terminalInstanceRef.current;
    const fit = fitAddonInstanceRef.current;

    if (term) {
      const ro = (term as Terminal & { _resizeObserver?: ResizeObserver })
        ._resizeObserver;
      ro?.disconnect();
      fit?.dispose();
      term.dispose();
      terminalInstanceRef.current = null;
      fitAddonInstanceRef.current = null;
      searchAddonInstanceRef.current = null;
      setTerminal(null);
      setFitAddon(null);
      setSearchAddon(null);
    }
  }, []);

  const writeToTerminal = useCallback((data: string | Uint8Array) => {
    terminalInstanceRef.current?.write(data);
  }, []);

  const resizePTY = useCallback((): { cols: number; rows: number } | null => {
    const term = terminalInstanceRef.current;
    const fit = fitAddonInstanceRef.current;
    if (!term || !fit) return null;
    fit.fit();
    return { cols: term.cols, rows: term.rows };
  }, []);

  const changeFontSize = useCallback((delta: number) => {
    const term = terminalInstanceRef.current;
    const fit = fitAddonInstanceRef.current;
    if (!term || !fit) return;
    const current = term.options.fontSize ?? FONT_SIZE_DEFAULT;
    const next = Math.min(
      FONT_SIZE_MAX,
      Math.max(FONT_SIZE_MIN, current + delta),
    );
    term.options.fontSize = next;
    fit.fit();
    preferencesRef.current.save({ font_size: next });
  }, []);

  const setTheme = useCallback((key: string) => {
    const term = terminalInstanceRef.current;
    const themeObj = themes[key] ?? themes[defaultThemeKey];
    const resolvedKey = themes[key] ? key : defaultThemeKey;
    if (term) {
      term.options.theme = themeToXterm(themeObj);
    }
    currentThemeKeyRef.current = resolvedKey;
    setCurrentThemeKey(resolvedKey);
    preferencesRef.current.save({ theme: resolvedKey });
  }, []);

  useEffect(() => {
    const term = terminalInstanceRef.current;
    if (!term) return;
    const key = themes[preferences.theme] ? preferences.theme : defaultThemeKey;
    term.options.theme = themeToXterm(themes[key]);
    term.options.fontSize = preferences.fontSize;
    currentThemeKeyRef.current = key;
    setCurrentThemeKey(key);
    fitAddonInstanceRef.current?.fit();
  }, [preferences.theme, preferences.fontSize, terminal]);

  const search = useCallback(
    (query: string, searchOptions?: { findNext?: boolean }): boolean => {
      const sa = searchAddonInstanceRef.current;
      if (!sa || !query) return false;
      if (searchOptions?.findNext) {
        return sa.findNext(query);
      }
      return sa.findPrevious(query);
    },
    [],
  );

  return {
    terminalRef,
    terminal,
    fitAddon,
    searchAddon,
    initTerminal,
    disposeTerminal,
    writeToTerminal,
    resizePTY,
    changeFontSize,
    setTheme,
    currentThemeKey,
    search,
  };
}
