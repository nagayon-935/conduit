import { describe, it, expect } from 'vitest';
import { getSlotStyle, getTabStyle, DIVIDER_HALF } from './paneGeometry';

describe('getSlotStyle', () => {
  it('returns hidden for single layout', () => {
    expect(getSlotStyle(0, '1', 0.5, 0.5)).toEqual({ display: 'none' });
  });

  it('splits 2v into left/right around ratioV', () => {
    const left = getSlotStyle(0, '2v', 0.5, 0.5);
    const right = getSlotStyle(1, '2v', 0.5, 0.5);
    expect(left.position).toBe('absolute');
    expect(left.left).toBe(0);
    expect(left.right).toBe(`calc(50% + ${DIVIDER_HALF}px)`);
    expect(right.right).toBe(0);
    expect(right.left).toBe(`calc(50% + ${DIVIDER_HALF}px)`);
  });

  it('honors a non-centered vertical ratio', () => {
    const left = getSlotStyle(0, '2v', 0.3, 0.5);
    expect(left.right).toBe(`calc(70% + ${DIVIDER_HALF}px)`);
    const right = getSlotStyle(1, '2v', 0.3, 0.5);
    expect(right.left).toBe(`calc(30% + ${DIVIDER_HALF}px)`);
  });

  it('splits 2h into top/bottom around ratioH', () => {
    const top = getSlotStyle(0, '2h', 0.5, 0.4);
    const bottom = getSlotStyle(1, '2h', 0.5, 0.4);
    expect(top.top).toBe(0);
    expect(top.bottom).toBe(`calc(60% + ${DIVIDER_HALF}px)`);
    expect(bottom.bottom).toBe(0);
    expect(bottom.top).toBe(`calc(40% + ${DIVIDER_HALF}px)`);
  });

  it('positions the four quadrants of a 2x2 grid', () => {
    const tl = getSlotStyle(0, '4', 0.5, 0.5);
    const tr = getSlotStyle(1, '4', 0.5, 0.5);
    const bl = getSlotStyle(2, '4', 0.5, 0.5);
    const br = getSlotStyle(3, '4', 0.5, 0.5);
    expect(tl.top).toBe(0);
    expect(tl.left).toBe(0);
    expect(tr.top).toBe(0);
    expect(tr.right).toBe(0);
    expect(bl.bottom).toBe(0);
    expect(bl.left).toBe(0);
    expect(br.bottom).toBe(0);
    expect(br.right).toBe(0);
  });
});

describe('getTabStyle', () => {
  it('shows only the active tab in single layout', () => {
    const active = getTabStyle('a', '1', [], 'a', 0.5, 0.5);
    const inactive = getTabStyle('b', '1', [], 'a', 0.5, 0.5);
    expect(active.display).toBe('flex');
    expect(inactive.display).toBe('none');
  });

  it('hides tabs not assigned to a pane slot', () => {
    const style = getTabStyle('x', '2v', ['a', 'b', null, null], 'a', 0.5, 0.5);
    expect(style.display).toBe('none');
  });

  it('positions a tab by its pane slot index', () => {
    const style = getTabStyle('b', '2v', ['a', 'b', null, null], 'a', 0.5, 0.5);
    expect(style).toEqual(getSlotStyle(1, '2v', 0.5, 0.5));
  });
});
