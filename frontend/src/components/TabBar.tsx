import { useState, useRef, useEffect, useCallback } from 'react';
import type { LayoutType, Profile } from '../types';
import { matchProfile } from '../utils/form';
import './TabBar.css';

export interface Tab {
  id: string;
  host: string;
  port: number;
  user: string;
}

interface TabBarProps {
  tabs: Tab[];
  activeId: string | null;
  onSelect: (id: string) => void;
  onClose: (id: string) => void;
  onNew: () => void;
  layoutType: LayoutType;
  paneTabIds: (string | null)[];
  onLayoutChange: (layout: LayoutType) => void;
  profiles?: Profile[];
  onReorder?: (fromId: string, toId: string) => void;
}

function tabLabel(tab: Tab, profiles: Profile[] = []): string {
  const matched = matchProfile(profiles, tab.host, tab.port, tab.user);
  if (matched) return matched.name;
  const portSuffix = tab.port === 22 ? '' : `:${tab.port}`;
  return `${tab.user}@${tab.host}${portSuffix}`;
}

function LayoutIcon({ type }: { type: LayoutType }) {
  return (
    <span className={`layout-icon layout-icon--${type}`} aria-hidden="true">
      {type === '1' && <span />}
      {type === '2v' && <><span /><span /></>}
      {type === '2h' && <><span /><span /></>}
      {type === '4' && <><span /><span /><span /><span /></>}
    </span>
  );
}

const LAYOUT_BTNS: { key: LayoutType; title: string; minTabs: number }[] = [
  { key: '1',  title: 'Single pane',  minTabs: 1 },
  { key: '2v', title: 'Side by side', minTabs: 2 },
  { key: '2h', title: 'Top / Bottom', minTabs: 2 },
  { key: '4',  title: '2×2 grid',     minTabs: 2 },
];

export function TabBar({
  tabs,
  activeId,
  onSelect,
  onClose,
  onNew,
  layoutType,
  paneTabIds,
  onLayoutChange,
  profiles = [],
  onReorder,
}: TabBarProps) {
  const [draggingId, setDraggingId] = useState<string | null>(null);
  const [dragOverId, setDragOverId] = useState<string | null>(null);
  const [canScrollLeft, setCanScrollLeft] = useState(false);
  const [canScrollRight, setCanScrollRight] = useState(false);
  const scrollAreaRef = useRef<HTMLDivElement>(null);

  const updateScrollIndicators = useCallback(() => {
    const el = scrollAreaRef.current;
    if (!el) return;
    setCanScrollLeft(el.scrollLeft > 0);
    setCanScrollRight(el.scrollLeft + el.clientWidth < el.scrollWidth - 1);
  }, []);

  useEffect(() => {
    const el = scrollAreaRef.current;
    if (!el) return;
    updateScrollIndicators();
    el.addEventListener('scroll', updateScrollIndicators, { passive: true });
    const ro = new ResizeObserver(updateScrollIndicators);
    ro.observe(el);
    return () => {
      el.removeEventListener('scroll', updateScrollIndicators);
      ro.disconnect();
    };
  }, [updateScrollIndicators, tabs]);

  const isInSplit = layoutType !== '1';

  // Sessions currently shown in split panes (in pane order, skipping nulls)
  const paneSessions = paneTabIds
    .filter((id): id is string => id !== null)
    .map((id) => tabs.find((t) => t.id === id))
    .filter((t): t is Tab => t !== undefined);

  // Tabs NOT assigned to any pane (background sessions)
  const backgroundTabs = tabs.filter((t) => !paneTabIds.includes(t.id));

  return (
    <div className="tab-bar" role="tablist">
      {/* Scrollable tab area with fade indicators */}
      <div
        className={`tab-scroll-area${canScrollLeft ? ' tab-scroll-area--fade-left' : ''}${canScrollRight ? ' tab-scroll-area--fade-right' : ''}`}
        ref={scrollAreaRef}
      >
        {isInSplit ? (
          <>
            {/* Background tabs (not in any current pane) */}
            {backgroundTabs.map((tab) => (
              <div
                key={tab.id}
                className={`tab-item tab-item--bg${draggingId === tab.id ? ' tab-item--dragging' : ''}${dragOverId === tab.id && draggingId !== tab.id ? ' tab-item--drag-over' : ''}`}
                role="tab"
                aria-selected={false}
                draggable
                onDragStart={() => setDraggingId(tab.id)}
                onDragOver={(e) => { e.preventDefault(); setDragOverId(tab.id); }}
                onDrop={() => { if (draggingId && draggingId !== tab.id) onReorder?.(draggingId, tab.id); setDraggingId(null); setDragOverId(null); }}
                onDragEnd={() => { setDraggingId(null); setDragOverId(null); }}
                onClick={() => onSelect(tab.id)}
                title={tabLabel(tab, profiles)}
              >
                <span>{tabLabel(tab, profiles)}</span>
                <button
                  className="tab-close"
                  aria-label={`Close ${tabLabel(tab, profiles)}`}
                  onClick={(e) => { e.stopPropagation(); onClose(tab.id); }}
                >✕</button>
              </div>
            ))}

            {/* Aggregated chip — all pane sessions in one */}
            {paneSessions.length > 0 && (
              <div className="tab-split-group" title="Split view sessions">
                <span className="tab-split-group-icon" aria-hidden="true">⊞</span>
                {paneSessions.map((tab, i) => (
                  <span key={tab.id} className="tab-split-group-entry">
                    {i > 0 && <span className="tab-split-group-sep" aria-hidden="true">│</span>}
                    <span>{tabLabel(tab, profiles)}</span>
                    <button
                      className="tab-close"
                      aria-label={`Close ${tabLabel(tab, profiles)}`}
                      onClick={(e) => { e.stopPropagation(); onClose(tab.id); }}
                    >✕</button>
                  </span>
                ))}
              </div>
            )}
          </>
        ) : (
          /* Normal mode: all tabs individually */
          tabs.map((tab) => {
            const isActive = tab.id === activeId;
            return (
              <div
                key={tab.id}
                className={`tab-item${isActive ? ' active' : ''}${draggingId === tab.id ? ' tab-item--dragging' : ''}${dragOverId === tab.id && draggingId !== tab.id ? ' tab-item--drag-over' : ''}`}
                role="tab"
                aria-selected={isActive}
                draggable
                onDragStart={() => setDraggingId(tab.id)}
                onDragOver={(e) => { e.preventDefault(); setDragOverId(tab.id); }}
                onDrop={() => { if (draggingId && draggingId !== tab.id) onReorder?.(draggingId, tab.id); setDraggingId(null); setDragOverId(null); }}
                onDragEnd={() => { setDraggingId(null); setDragOverId(null); }}
                onClick={() => onSelect(tab.id)}
                title={tabLabel(tab, profiles)}
              >
                <span>{tabLabel(tab, profiles)}</span>
                <button
                  className="tab-close"
                  aria-label={`Close ${tabLabel(tab, profiles)}`}
                  onClick={(e) => { e.stopPropagation(); onClose(tab.id); }}
                >✕</button>
              </div>
            );
          })
        )}

        {/* + button — inside scroll area, stays next to last tab */}
        <button
          className="tab-new-btn"
          aria-label="New connection"
          title="New connection"
          onClick={onNew}
        >+</button>
      </div>

      {/* Layout buttons — always right end, outside scroll area */}
      <div className="tab-layout-group">
        {LAYOUT_BTNS.map(({ key, title, minTabs }) => (
          <button
            key={key}
            className={`tab-layout-btn${layoutType === key ? ' active' : ''}`}
            title={`${title} (Alt+${['1','2','3','4'][LAYOUT_BTNS.findIndex(b => b.key === key)]})`}
            onClick={() => onLayoutChange(key)}
            disabled={tabs.length < minTabs}
          >
            <LayoutIcon type={key} />
          </button>
        ))}
      </div>

    </div>
  );
}
