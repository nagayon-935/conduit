import { useState, useEffect } from 'react';
import './LogPage.css';

interface LogEntry {
  id: string;
  host: string;
  port: number;
  user: string;
  connected_at: string;
  disconnected_at?: string;
  error?: string;
}

interface LogPageProps {
  onBack: () => void;
}

function formatDateTime(iso: string): string {
  return new Date(iso).toLocaleString([], {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
}

export function LogPage({ onBack }: LogPageProps) {
  const [entries, setEntries] = useState<LogEntry[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;

    async function load() {
      try {
        const res = await fetch('/api/logs');
        if (!res.ok) throw new Error(`Failed to fetch logs: ${res.status}`);
        const data = await res.json();
        if (!cancelled) {
          setEntries(data ?? []);
          setError(null);
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : 'Failed to load logs');
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    load();
    return () => { cancelled = true; };
  }, []);

  const hasErrors = entries.some((e) => e.error);

  return (
    <div className="lp-page">
      <header className="lp-header">
        <button className="lp-back-btn" onClick={onBack}>← Back</button>
        <span className="lp-title">Connection Log</span>
      </header>

      <main className="lp-main">
        {loading && <p className="lp-loading">Loading…</p>}

        {error && <div className="lp-error">{error}</div>}

        {!loading && entries.length === 0 && !error && (
          <div className="lp-empty">
            <p className="lp-empty-title">No connection history yet.</p>
            <p className="lp-empty-hint">Connections will appear here once you connect to an SSH server.</p>
          </div>
        )}

        {entries.length > 0 && (
          <div className="lp-table-wrap">
            <table className="lp-table">
              <thead>
                <tr>
                  <th>Host</th>
                  <th>Port</th>
                  <th>User</th>
                  <th>Connected At</th>
                  <th>Disconnected At</th>
                  {hasErrors && <th>Error</th>}
                </tr>
              </thead>
              <tbody>
                {entries.map((e) => (
                  <tr key={e.id}>
                    <td className="lp-cell-host">{e.host}</td>
                    <td className="lp-cell-port">{e.port}</td>
                    <td className="lp-cell-user">{e.user}</td>
                    <td className="lp-cell-time">{formatDateTime(e.connected_at)}</td>
                    <td className="lp-cell-disconnected">
                      {e.disconnected_at ? formatDateTime(e.disconnected_at) : '—'}
                    </td>
                    {hasErrors && (
                      <td className="lp-cell-error" title={e.error ?? ''}>
                        {e.error ?? ''}
                      </td>
                    )}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </main>
    </div>
  );
}
