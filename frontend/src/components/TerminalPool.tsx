import { type MouseEvent as ReactMouseEvent } from 'react';
import { Terminal } from './Terminal';
import { TerminalWterm } from './TerminalWterm';
import type { LayoutType, SessionTab } from '../types';
import { getSlotStyle, getTabStyle } from '../utils/paneGeometry';

// PoC gate: the first tab renders via @wterm/react instead of xterm.js so we
// can evaluate the DOM-based renderer against a live SSH session without
// touching the other tabs. Remove this flag (and TerminalWterm.tsx /
// useWtermSocket.ts) once the PoC decision is made.
const WTERM_POC_TAB_INDEX = 0;

interface TerminalPoolProps {
  tabs: SessionTab[];
  layoutType: LayoutType;
  paneTabIds: (string | null)[];
  activeTabId: string | null;
  splitRatioV: number;
  splitRatioH: number;
  onCloseTab: (id: string) => void;
  onDividerVMouseDown: (e: ReactMouseEvent) => void;
  onDividerHMouseDown: (e: ReactMouseEvent) => void;
  onResetRatioV: () => void;
  onResetRatioH: () => void;
}

/**
 * Renders every session terminal once, kept mounted at a stable position in
 * the React tree. Layout is expressed purely via absolute CSS positioning so
 * switching layouts never remounts (and therefore never clears) a terminal.
 */
export function TerminalPool({
  tabs,
  layoutType,
  paneTabIds,
  activeTabId,
  splitRatioV,
  splitRatioH,
  onCloseTab,
  onDividerVMouseDown,
  onDividerHMouseDown,
  onResetRatioV,
  onResetRatioH,
}: TerminalPoolProps) {
  const showVDivider = layoutType === '2v' || layoutType === '4';
  const showHDivider = layoutType === '2h' || layoutType === '4';
  const emptySlotCount = layoutType === '1' ? 0 : layoutType === '4' ? 4 : 2;

  return (
    <div style={{ flex: 1, position: 'relative', minHeight: 0, overflow: 'hidden' }}>
      {tabs.map((tab, index) => {
        const TerminalComponent = index === WTERM_POC_TAB_INDEX ? TerminalWterm : Terminal;
        return (
          <div
            key={tab.id}
            style={getTabStyle(tab.id, layoutType, paneTabIds, activeTabId, splitRatioV, splitRatioH)}
          >
            <TerminalComponent
              sessionToken={tab.sessionToken}
              host={tab.host}
              port={tab.port}
              user={tab.user}
              expiresAt={tab.expiresAt}
              onDisconnect={() => onCloseTab(tab.id)}
              shareToken={tab.shareToken}
            />
          </div>
        );
      })}

      {/* Placeholders for unoccupied split slots */}
      {Array.from({ length: emptySlotCount }, (_, slotIdx) => {
        const tabId = paneTabIds[slotIdx];
        if (tabId != null && tabs.some((t) => t.id === tabId)) return null;
        return (
          <div key={`empty-${slotIdx}`} style={getSlotStyle(slotIdx, layoutType, splitRatioV, splitRatioH)}>
            <div className="split-empty-pane">
              <span>No session selected</span>
            </div>
          </div>
        );
      })}

      {showVDivider && (
        <div
          className="split-divider-v"
          style={{ position: 'absolute', top: 0, bottom: 0, left: `${splitRatioV * 100}%`, transform: 'translateX(-50%)', zIndex: 10 }}
          onMouseDown={onDividerVMouseDown}
          onDoubleClick={onResetRatioV}
          title="Double-click to reset"
        />
      )}
      {showHDivider && (
        <div
          className="split-divider-h"
          style={{ position: 'absolute', left: 0, right: 0, top: `${splitRatioH * 100}%`, transform: 'translateY(-50%)', zIndex: 10 }}
          onMouseDown={onDividerHMouseDown}
          onDoubleClick={onResetRatioH}
          title="Double-click to reset"
        />
      )}
    </div>
  );
}
