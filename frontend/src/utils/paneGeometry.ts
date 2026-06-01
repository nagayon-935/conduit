import type { CSSProperties } from 'react';
import type { LayoutType } from '../types';

// Half of the 4 px divider — used to leave a gap so terminals don't slide under the divider.
export const DIVIDER_HALF = 2;

/** Absolute-position style for a specific pane slot (slot 0-3). */
export function getSlotStyle(
  slotIdx: number,
  layout: LayoutType,
  ratioV: number,
  ratioH: number,
): CSSProperties {
  const base: CSSProperties = { position: 'absolute', display: 'flex', flexDirection: 'column', overflow: 'hidden' };
  if (layout === '2v') {
    if (slotIdx === 0) return { ...base, top: 0, bottom: 0, left: 0, right: `calc(${(1 - ratioV) * 100}% + ${DIVIDER_HALF}px)` };
    if (slotIdx === 1) return { ...base, top: 0, bottom: 0, right: 0, left: `calc(${ratioV * 100}% + ${DIVIDER_HALF}px)` };
  }
  if (layout === '2h') {
    if (slotIdx === 0) return { ...base, top: 0, left: 0, right: 0, bottom: `calc(${(1 - ratioH) * 100}% + ${DIVIDER_HALF}px)` };
    if (slotIdx === 1) return { ...base, bottom: 0, left: 0, right: 0, top: `calc(${ratioH * 100}% + ${DIVIDER_HALF}px)` };
  }
  if (layout === '4') {
    const isLeft = slotIdx === 0 || slotIdx === 2;
    const isTop  = slotIdx === 0 || slotIdx === 1;
    return {
      ...base,
      top:    isTop  ? 0 : `calc(${ratioH * 100}% + ${DIVIDER_HALF}px)`,
      bottom: isTop  ? `calc(${(1 - ratioH) * 100}% + ${DIVIDER_HALF}px)` : 0,
      left:   isLeft ? 0 : `calc(${ratioV * 100}% + ${DIVIDER_HALF}px)`,
      right:  isLeft ? `calc(${(1 - ratioV) * 100}% + ${DIVIDER_HALF}px)` : 0,
    };
  }
  return { display: 'none' };
}

/** Absolute-position style for a tab's wrapper inside the terminal pool. */
export function getTabStyle(
  tabId: string,
  layout: LayoutType,
  paneTabIds: (string | null)[],
  activeTabId: string | null,
  ratioV: number,
  ratioH: number,
): CSSProperties {
  if (layout === '1') {
    const visible = tabId === activeTabId;
    return { position: 'absolute', top: 0, bottom: 0, left: 0, right: 0, display: visible ? 'flex' : 'none', flexDirection: 'column', overflow: 'hidden' };
  }
  const slotIdx = paneTabIds.indexOf(tabId);
  if (slotIdx === -1) return { position: 'absolute', top: 0, bottom: 0, left: 0, right: 0, display: 'none' };
  return getSlotStyle(slotIdx, layout, ratioV, ratioH);
}
