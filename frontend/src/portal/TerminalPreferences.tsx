import {
  createContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { apiFetch, mutate } from "../api/fetch";
import type { Preferences } from "../portalTypes";
import { defaultThemeKey } from "../themes";

export const TerminalPreferences = createContext({
  theme: defaultThemeKey,
  fontSize: 14,
  save: (_patch: { theme?: string; font_size?: number }) => {},
});
export function TerminalPreferencesProvider({
  children,
}: {
  children: ReactNode;
}) {
  const [preferences, setPreferences] = useState<Preferences>({
    theme: defaultThemeKey,
    font_size: 14,
    favorites: [],
  });
  const pending = useRef<{ theme?: string; font_size?: number }>({});
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const chain = useRef(Promise.resolve());
  const requests = useRef<AbortController | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    requests.current = controller;
    apiFetch<Preferences>("/api/app/preferences", { signal: controller.signal })
      .then(setPreferences)
      .catch(() => {});
    const handler = (event: Event) =>
      setPreferences((event as CustomEvent<Preferences>).detail);
    window.addEventListener("conduit:preferences", handler);
    return () => {
      controller.abort();
      window.removeEventListener("conduit:preferences", handler);
      if (timer.current) clearTimeout(timer.current);
    };
  }, []);
  function save(patch: { theme?: string; font_size?: number }) {
    setPreferences((p) => ({ ...p, ...patch }));
    pending.current = { ...pending.current, ...patch };
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(() => {
      const changes = pending.current;
      pending.current = {};
      chain.current = chain.current
        .then(async () => {
          const signal = requests.current?.signal;
          if (!signal || signal.aborted) return;
          const latest = await apiFetch<Preferences>("/api/app/preferences", {
            signal,
          });
          await mutate(
            "/api/app/preferences",
            { ...latest, ...changes },
            "PATCH",
            signal,
          );
        })
        .catch((error) => {
          if (requests.current?.signal.aborted) return;
          window.dispatchEvent(
            new CustomEvent("conduit:notice", {
              detail: (error as Error).message,
            }),
          );
        });
    }, 300);
  }
  return (
    <TerminalPreferences.Provider
      value={{
        theme: preferences.theme,
        fontSize: preferences.font_size,
        save,
      }}
    >
      {children}
    </TerminalPreferences.Provider>
  );
}
