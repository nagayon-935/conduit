import { useState, useRef, useEffect } from 'react';
import { FONT_SIZE_MIN, FONT_SIZE_MAX, FONT_SIZE_DEFAULT } from '../constants';

const TOAST_DURATION_MS = 1500;

/**
 * Registers Ctrl+= / Ctrl+- to change the terminal font size and returns the
 * transient toast value (the new size in px, or null when hidden).
 */
export function useFontSizeShortcuts(
  changeFontSize: (delta: number) => void,
  getFontSize: () => number,
): number | null {
  const [toast, setToast] = useState<number | null>(null);
  const toastTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => () => {
    if (toastTimerRef.current) clearTimeout(toastTimerRef.current);
  }, []);

  useEffect(() => {
    function showToast(size: number) {
      setToast(size);
      if (toastTimerRef.current) clearTimeout(toastTimerRef.current);
      toastTimerRef.current = setTimeout(() => setToast(null), TOAST_DURATION_MS);
    }

    function handleKeyDown(e: KeyboardEvent) {
      if (e.ctrlKey && (e.code === 'Equal' || e.key === '+')) {
        e.preventDefault();
        changeFontSize(1);
        showToast(Math.min(FONT_SIZE_MAX, (getFontSize() ?? FONT_SIZE_DEFAULT) + 1));
      } else if (e.ctrlKey && (e.code === 'Minus' || e.key === '-')) {
        e.preventDefault();
        changeFontSize(-1);
        showToast(Math.max(FONT_SIZE_MIN, (getFontSize() ?? FONT_SIZE_DEFAULT) - 1));
      }
    }
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [changeFontSize, getFontSize]);

  return toast;
}
