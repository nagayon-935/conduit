import { useState, useEffect, useRef } from 'react';
import './LogPage.css';
import 'asciinema-player/dist/bundle/asciinema-player.css';

// asciinema-player is loaded as a side-effect import; types are minimal.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
type AsciinemaPlayer = any;

interface LogEntry {
  id: string;
  host: string;
  port: number;
  user: string;
  connected_at: string;
  disconnected_at?: string;
  error?: string;
  recording_path?: string;
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

interface PlaybackModalProps {
  logId: string;
  title: string;
  onClose: () => void;
}

function PlaybackModal({ logId, title, onClose }: PlaybackModalProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const playerRef = useRef<AsciinemaPlayer>(null);

  useEffect(() => {
    let mod: AsciinemaPlayer = null;
    import('asciinema-player').then((m) => {
      mod = m;
      if (containerRef.current) {
        playerRef.current = m.create(
          `/api/recordings/${encodeURIComponent(logId)}`,
          containerRef.current,
          { fit: 'both', terminalFontSize: 'small' },
        );
      }
    });
    return () => {
      playerRef.current?.dispose?.();
      mod?.dispose?.();
    };
  }, [logId]);

  return (
    <div className="lp-modal-backdrop" onClick={onClose}>
      <div className="lp-modal" onClick={(e) => e.stopPropagation()}>
        <div className="lp-modal-header">
          <span className="lp-modal-title">{title}</span>
          <button className="lp-modal-close" onClick={onClose} title="Close">✕</button>
        </div>
        <div className="lp-modal-player" ref={containerRef} />
      </div>
    </div>
  );
}

export function LogPage({ onBack }: LogPageProps) {
  const [entries, setEntries] = useState<LogEntry[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [playback, setPlayback] = useState<{ id: string; title: string } | null>(null);

  useEffect(() => {
    let cancelled = false;

    async function load() {
      try {
        const res = await fetch('/api/logs');
        if (!res.ok) throw new Error(`Failed to fetch logs: ${res.status}`);
        const data = await res.json() as LogEntry[];
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
  const hasRecordings = entries.some((e) => e.recording_path);

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
                  {hasRecordings && <th>Recording</th>}
                </tr>
              </thead>
              <tbody>
                {entries.map((e) => (
                  <tr key={e.id} className={e.error ? 'lp-row-error' : ''}>
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
                    {hasRecordings && (
                      <td className="lp-cell-recording">
                        {e.recording_path ? (
                          <button
                            type="button"
                            className="lp-play-btn"
                            onClick={() => setPlayback({
                              id: e.id,
                              title: `${e.user}@${e.host} — ${formatDateTime(e.connected_at)}`,
                            })}
                          >
                            ▶ Play
                          </button>
                        ) : '—'}
                      </td>
                    )}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </main>

      {playback && (
        <PlaybackModal
          logId={playback.id}
          title={playback.title}
          onClose={() => setPlayback(null)}
        />
      )}
    </div>
  );
}
