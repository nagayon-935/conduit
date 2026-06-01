import { useState, useCallback, useEffect, useRef, type MouseEvent as ReactMouseEvent } from 'react';
import type { LayoutType } from '../types';

const RATIO_MIN = 0.2;
const RATIO_MAX = 0.8;
const TAB_BAR_HEIGHT = 40;

const LAYOUT_CODES: Record<string, LayoutType> = {
  Digit1: '1',
  Digit2: '2v',
  Digit3: '2h',
  Digit4: '4',
};

export interface UseSplitLayoutResult {
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
export function useSplitLayout(orderedTabIds: string[], activeTabId: string | null): UseSplitLayoutResult {
  const [layoutType, setLayoutType] = useState<LayoutType>('1');
  const [paneTabIds, setPaneTabIds] = useState<(string | null)[]>([null, null, null, null]);
  const [splitRatioV, setSplitRatioV] = useState(0.5);
  const [splitRatioH, setSplitRatioH] = useState(0.5);

  const isDraggingVRef = useRef(false);
  const isDraggingHRef = useRef(false);

  // Keep latest tab data available to the (stable) keyboard handler.
  const dataRef = useRef({ orderedTabIds, activeTabId });
  dataRef.current = { orderedTabIds, activeTabId };
  const layoutTypeRef = useRef(layoutType);
  layoutTypeRef.current = layoutType;

  const switchLayout = useCallback((newLayout: LayoutType) => {
    if (newLayout === '1') {
      setLayoutType('1');
      setPaneTabIds((prev) => [prev[0] ?? dataRef.current.activeTabId, null, null, null]);
      return;
    }
    const numPanes = newLayout === '4' ? 4 : 2;
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
    if (layoutTypeRef.current === '1') return;
    setPaneTabIds((prev) => {
      const emptyIdx = prev.findIndex((p) => p === null);
      if (emptyIdx === -1) return prev;
      const updated = [...prev];
      updated[emptyIdx] = id;
      return updated;
    });
  }, []);

  // Remove `id` from any pane; collapse to single view if ≤ 1 pane remains.
  const releasePane = useCallback((id: string) => {
    setPaneTabIds((prev) => {
      const newPanes = prev.map((p) => (p === id ? null : p));
      const occupied = newPanes.filter(Boolean).length;
      if (occupied <= 1) setLayoutType('1');
      return occupied > 0 ? newPanes : [null, null, null, null];
    });
  }, []);

  const resetRatioV = useCallback(() => setSplitRatioV(0.5), []);
  const resetRatioH = useCallback(() => setSplitRatioH(0.5), []);

  const onDividerVMouseDown = useCallback((e: ReactMouseEvent) => {
    e.preventDefault();
    isDraggingVRef.current = true;
  }, []);
  const onDividerHMouseDown = useCallback((e: ReactMouseEvent) => {
    e.preventDefault();
    isDraggingHRef.current = true;
  }, []);

  // ── Alt+1/2/3/4 layout shortcuts ─────────────────────────────────────────
  // Use e.code (physical key) so macOS Option+Digit2 (which yields '™' in
  // e.key) still maps correctly.
  useEffect(() => {
    function handleLayoutKey(e: KeyboardEvent) {
      if (!e.altKey) return;
      const layout = LAYOUT_CODES[e.code];
      if (!layout) return;
      e.preventDefault();
      switchLayout(layout);
    }
    window.addEventListener('keydown', handleLayoutKey);
    return () => window.removeEventListener('keydown', handleLayoutKey);
  }, [switchLayout]);

  // ── Divider dragging ─────────────────────────────────────────────────────
  useEffect(() => {
    function onMouseMove(e: MouseEvent) {
      if (isDraggingVRef.current) {
        setSplitRatioV(Math.min(RATIO_MAX, Math.max(RATIO_MIN, e.clientX / window.innerWidth)));
      }
      if (isDraggingHRef.current) {
        const ratio = (e.clientY - TAB_BAR_HEIGHT) / (window.innerHeight - TAB_BAR_HEIGHT);
        setSplitRatioH(Math.min(RATIO_MAX, Math.max(RATIO_MIN, ratio)));
      }
    }
    function onMouseUp() {
      isDraggingVRef.current = false;
      isDraggingHRef.current = false;
    }
    window.addEventListener('mousemove', onMouseMove);
    window.addEventListener('mouseup', onMouseUp);
    return () => {
      window.removeEventListener('mousemove', onMouseMove);
      window.removeEventListener('mouseup', onMouseUp);
    };
  }, []);

  return {
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
