import { useEffect, useRef, useState, type ReactNode } from "react";
import { apiFetch } from "../api/fetch";

export function useData<T>(url: string, initial: T) {
  const [data, setData] = useState<T>(initial);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [version, setVersion] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    apiFetch<T>(url, { signal: controller.signal })
      .then((value) => {
        setData(value);
        setError("");
      })
      .catch((error) => {
        if (!controller.signal.aborted)
          setError(
            error instanceof Error ? error.message : "読み込めませんでした",
          );
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [url, version]);
  return {
    data,
    setData,
    error,
    loading,
    refresh: () => setVersion((value) => value + 1),
  };
}
export function ErrorMessage({ message }: { message: string }) {
  return message ? (
    <p className="portal-error" role="alert">
      {message}
    </p>
  ) : null;
}
export function Field({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  return (
    <label className="portal-field">
      <span>{label}</span>
      {children}
    </label>
  );
}
export function Toggle({
  label,
  checked,
  onChange,
}: {
  label: string;
  checked: boolean;
  onChange: (value: boolean) => void;
}) {
  return (
    <label className="portal-toggle">
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
      />
      {label}
    </label>
  );
}
export function Dialog({
  title,
  onClose,
  children,
}: {
  title: string;
  onClose: () => void;
  children: ReactNode;
}) {
  const closeRef = useRef(onClose);
  closeRef.current = onClose;
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      if (e.key === "Escape") closeRef.current();
    };
    window.addEventListener("keydown", key);
    const previous = document.activeElement as HTMLElement | null;
    return () => {
      window.removeEventListener("keydown", key);
      previous?.focus();
    };
  }, []);
  return (
    <div className="portal-backdrop">
      <section
        className="portal-dialog"
        role="dialog"
        aria-modal="true"
        aria-label={title}
        onKeyDown={(e) => {
          if (e.key !== "Tab") return;
          const nodes = e.currentTarget.querySelectorAll<HTMLElement>(
            "button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), a[href]",
          );
          const first = nodes[0],
            last = nodes[nodes.length - 1];
          if (e.shiftKey && document.activeElement === first) {
            e.preventDefault();
            last?.focus();
          } else if (!e.shiftKey && document.activeElement === last) {
            e.preventDefault();
            first?.focus();
          }
        }}
      >
        <header>
          <h2>{title}</h2>
          <button autoFocus onClick={onClose} aria-label="閉じる">
            ×
          </button>
        </header>
        {children}
      </section>
    </div>
  );
}
export function date(value?: number) {
  return value ? new Date(value * 1000).toLocaleString() : "—";
}
