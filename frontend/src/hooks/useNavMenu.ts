import { useState, useRef, useEffect } from 'react';

export interface UseNavMenuResult {
  open: boolean;
  toggle: () => void;
  close: () => void;
  menuRef: React.RefObject<HTMLDivElement>;
}

/** Open/close state for the nav dropdown, with outside-click dismissal. */
export function useNavMenu(): UseNavMenuResult {
  const [open, setOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    function onOutside(e: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener('mousedown', onOutside);
    return () => document.removeEventListener('mousedown', onOutside);
  }, [open]);

  return {
    open,
    toggle: () => setOpen((v) => !v),
    close: () => setOpen(false),
    menuRef,
  };
}
