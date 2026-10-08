import {
  useState,
  useCallback,
  useEffect,
  useRef,
  type MouseEvent as ReactMouseEvent,
} from "react";
import type { LayoutType } from "../types";
import { readJSON, writeJSON } from "../utils/storage";
import { STORAGE_KEYS } from "../constants";

const RATIO_MIN = 0.2;
const RATIO_MAX = 0.8;

const LAYOUT_CODES: Record<string, LayoutType> = {
  Digit1: "1",
  Digit2: "2v",
  Digit3: "2h",
  Digit4: "4",
};

interface SavedLayout {
  layoutType: LayoutType;
  paneTabIds: (string | null)[];
  splitRatioV: number;
  splitRatioH: number;
}
function loadLayout(): SavedLayout {
  const raw =
    readJSON<Partial<SavedLayout> | null>(STORAGE_KEYS.LAYOUT, {}) ?? {};
  const ratio = (n: unknown) =>
    typeof n === "number" && Number.isFinite(n)
      ? Math.min(RATIO_MAX, Math.max(RATIO_MIN, n))
      : 0.5;
  return {
    layoutType: ["1", "2v", "2h", "4"].includes(raw.layoutType ?? "")
      ? raw.layoutType!
      : "1",
    paneTabIds: Array.isArray(raw.paneTabIds)
      ? [0, 1, 2, 3].map((i) =>
          typeof raw.paneTabIds![i] === "string" ? raw.paneTabIds![i] : null,
        )
      : [null, null, null, null],
    splitRatioV: ratio(raw.splitRatioV),
    splitRatioH: ratio(raw.splitRatioH),
  };
}

export interface UseSplitLayoutResult {
  selectPaneTab: (id: string) => void;
  showTab: (id: string) => void;
  layoutType: LayoutType;
  paneTabIds: (string | null)[];
  splitRatioV: number;
  splitRatioH: number;
  switchLayout: (layout: LayoutType) => void;
  fillEmptyPane: (id: string) => void;
  releasePane: (id: string) => void;
  resetRatioV: () => void;
  resetRatioH: () => void;
  onDividerVMouseDown: (e: ReactMouseEvent) => void;
  onDividerHMouseDown: (e: ReactMouseEvent) => void;
}

/**
 * Owns split-layout state: layout type, pane assignments, split ratios,
 * divider dragging, and the Alt+1/2/3/4 shortcuts. Tab data (ordered ids and
 * the active id) is passed in so this hook stays decoupled from tab ownership.
 */
export function useSplitLayout(
  orderedTabIds: string[],
  activeTabId: string | null,
  initialOrEnabled:
    | boolean
    | {
        type: LayoutType;
        paneIds: (string | null)[];
        ratioV: number;
        ratioH: number;
      } = true,
  interactive = true,
): UseSplitLayoutResult {
  const initial =
    typeof initialOrEnabled === "object" ? initialOrEnabled : undefined;
  const enabled =
    typeof initialOrEnabled === "boolean" ? initialOrEnabled : interactive;
  const [layoutType, setLayoutType] = useState<LayoutType>(
    () => initial?.type ?? loadLayout().layoutType,
  );
  const [paneTabIds, setPaneTabIds] = useState<(string | null)[]>(
    () =>
      initial?.paneIds ??
      loadLayout().paneTabIds.map((id) =>
        id && orderedTabIds.includes(id) ? id : null,
      ),
  );
  const [splitRatioV, setSplitRatioV] = useState(
    () => initial?.ratioV ?? loadLayout().splitRatioV,
  );
  const [splitRatioH, setSplitRatioH] = useState(
    () => initial?.ratioH ?? loadLayout().splitRatioH,
  );

  const dragBounds = useRef<DOMRect | null>(null);
  const isDraggingVRef = useRef(false);
  const isDraggingHRef = useRef(false);

  // Keep latest tab data available to the (stable) keyboard handler.
  const dataRef = useRef({ orderedTabIds, activeTabId });
  dataRef.current = { orderedTabIds, activeTabId };
  const layoutTypeRef = useRef(layoutType);
  layoutTypeRef.current = layoutType;

  const switchLayout = useCallback((newLayout: LayoutType) => {
    if (newLayout === "1") {
      setLayoutType("1");
      setPaneTabIds((prev) => [
        prev[0] ?? dataRef.current.activeTabId,
        null,
        null,
        null,
      ]);
      return;
    }
    const numPanes = newLayout === "4" ? 4 : 2;
    const ids = dataRef.current.orderedTabIds;
    const newPanes: (string | null)[] = [null, null, null, null];
    for (let i = 0; i < numPanes; i++) {
      newPanes[i] = ids[i] ?? null;
    }
    setLayoutType(newLayout);
    setPaneTabIds(newPanes);
  }, []);

  // Fill the first empty pane slot with `id` (no-op in single layout).
  const fillEmptyPane = useCallback((id: string) => {
    if (layoutTypeRef.current === "1") return;
    setPaneTabIds((prev) => {
      if (prev.includes(id)) return prev;
      const count = layoutTypeRef.current === "4" ? 4 : 2;
      const emptyIdx = prev.slice(0, count).findIndex((p) => p === null);
      const slot =
        emptyIdx >= 0
          ? emptyIdx
          : Math.max(
              0,
              prev.slice(0, count).indexOf(dataRef.current.activeTabId),
            );
      const updated = [...prev];
      updated[slot] = id;
      return updated;
    });
  }, []);

  // Remove `id` from any pane; collapse to single view if ≤ 1 pane remains.
  const releasePane = useCallback((id: string) => {
    setPaneTabIds((prev) => {
      const newPanes = prev.map((p) => (p === id ? null : p));
      const occupied = newPanes.filter(Boolean).length;
      if (occupied <= 1) setLayoutType("1");
      return occupied > 0 ? newPanes : [null, null, null, null];
    });
  }, []);

  const showTab = useCallback((id: string) => {
    if (layoutTypeRef.current === "1") return;
    setPaneTabIds((prev) => {
      if (prev.includes(id)) return prev;
      const slot = Math.max(0, prev.indexOf(dataRef.current.activeTabId));
      return prev.map((value, index) => (index === slot ? id : value));
    });
  }, []);

  useEffect(() => {
    if (initial) return;
    writeJSON(STORAGE_KEYS.LAYOUT, {
      layoutType,
      paneTabIds,
      splitRatioV,
      splitRatioH,
    });
  }, [layoutType, paneTabIds, splitRatioV, splitRatioH]);

  const resetRatioV = useCallback(() => setSplitRatioV(0.5), []);
  const resetRatioH = useCallback(() => setSplitRatioH(0.5), []);

  const onDividerVMouseDown = useCallback((e: ReactMouseEvent) => {
    e.preventDefault();
    dragBounds.current =
      e.currentTarget.parentElement?.getBoundingClientRect() ?? null;
    isDraggingVRef.current = true;
  }, []);
  const onDividerHMouseDown = useCallback((e: ReactMouseEvent) => {
    e.preventDefault();
    dragBounds.current =
      e.currentTarget.parentElement?.getBoundingClientRect() ?? null;
    isDraggingHRef.current = true;
  }, []);

  // ── Alt+1/2/3/4 layout shortcuts ─────────────────────────────────────────
  // Use e.code (physical key) so macOS Option+Digit2 (which yields '™' in
  // e.key) still maps correctly.
  useEffect(() => {
    function handleLayoutKey(e: KeyboardEvent) {
      if (document.querySelector('[role="dialog"]')) return;
      if (
        !enabled ||
        !e.altKey ||
        (e.target instanceof HTMLElement &&
          e.target.closest('input, textarea, select, [role="dialog"]') &&
          !e.target.closest(".xterm"))
      )
        return;
      const layout = LAYOUT_CODES[e.code];
      if (!layout) return;
      e.preventDefault();
      switchLayout(layout);
    }
    window.addEventListener("keydown", handleLayoutKey);
    return () => window.removeEventListener("keydown", handleLayoutKey);
  }, [switchLayout, enabled]);

  // ── Divider dragging ─────────────────────────────────────────────────────
  useEffect(() => {
    function onMouseMove(e: MouseEvent) {
      const bounds = dragBounds.current;
      if (!bounds || !bounds.width || !bounds.height) return;
      if (isDraggingVRef.current) {
        setSplitRatioV(
          Math.min(
            RATIO_MAX,
            Math.max(RATIO_MIN, (e.clientX - bounds.left) / bounds.width),
          ),
        );
      }
      if (isDraggingHRef.current) {
        const ratio = (e.clientY - bounds.top) / bounds.height;
        setSplitRatioH(Math.min(RATIO_MAX, Math.max(RATIO_MIN, ratio)));
      }
    }
    function onMouseUp() {
      isDraggingVRef.current = false;
      isDraggingHRef.current = false;
    }
    window.addEventListener("mousemove", onMouseMove);
    window.addEventListener("mouseup", onMouseUp);
    return () => {
      window.removeEventListener("mousemove", onMouseMove);
      window.removeEventListener("mouseup", onMouseUp);
    };
  }, []);

  return {
    showTab,
    selectPaneTab: showTab,
    layoutType,
    paneTabIds,
    splitRatioV,
    splitRatioH,
    switchLayout,
    fillEmptyPane,
    releasePane,
    resetRatioV,
    resetRatioH,
    onDividerVMouseDown,
    onDividerHMouseDown,
  };
}
