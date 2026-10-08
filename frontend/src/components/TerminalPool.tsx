import { type MouseEvent as ReactMouseEvent } from 'react';
import { Terminal } from './Terminal';
import type { LayoutType, SessionTab } from '../types';
import { getSlotStyle, getTabStyle } from '../utils/paneGeometry';

interface TerminalPoolProps {
  tabs: SessionTab[];
  layoutType: LayoutType;
  paneTabIds: (string | null)[];
  activeTabId: string | null;
  splitRatioV: number;
  splitRatioH: number;
  interactive: boolean;
  visible: boolean;
  onSelectTab: (id: string) => void;
  onEndTab: (id: string) => void;
  onNewFromTab: (id: string) => void;
  onUpdateTab: (id: string, patch: Partial<SessionTab>) => void;
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
  interactive, visible, onSelectTab, onEndTab, onNewFromTab, onUpdateTab,
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
    <div style={{ flex: 1, position: 'relative', minHeight: 0, overflow: 'hidden', display: visible ? 'block' : 'none' }}>
      {tabs.map((tab) => (
        <div
          key={tab.id}
          style={tab.paused ? { display: 'none' } : getTabStyle(tab.id, layoutType, paneTabIds, activeTabId, splitRatioV, splitRatioH)}
        >
          <Terminal tab={tab} active={interactive && tab.id === activeTabId && !tab.paused}
            onSelect={() => onSelectTab(tab.id)} onClose={() => onCloseTab(tab.id)}
            onEnd={() => onEndTab(tab.id)} onNew={() => onNewFromTab(tab.id)}
            onUpdate={(patch) => onUpdateTab(tab.id, patch)} />
        </div>
      ))}

      {/* Placeholders for unoccupied split slots */}
      {Array.from({ length: emptySlotCount }, (_, slotIdx) => {
        const tabId = paneTabIds[slotIdx];
        if (tabId != null && tabs.some((t) => t.id === tabId)) return null;
        return (
          <div key={`empty-${slotIdx}`} style={getSlotStyle(slotIdx, layoutType, splitRatioV, splitRatioH)}>
            <div className="split-empty-pane">
              <span>接続する端末をタブから選択してください</span>
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
          title="ダブルクリックで分割比率を戻す"
        />
      )}
      {showHDivider && (
        <div
          className="split-divider-h"
          style={{ position: 'absolute', left: 0, right: 0, top: `${splitRatioH * 100}%`, transform: 'translateY(-50%)', zIndex: 10 }}
          onMouseDown={onDividerHMouseDown}
          onDoubleClick={onResetRatioH}
          title="ダブルクリックで分割比率を戻す"
        />
      )}
    </div>
  );
}
