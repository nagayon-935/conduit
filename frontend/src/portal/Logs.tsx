import { useEffect, useRef, useState } from "react";
import { apiFetch } from "../api/fetch";
import type { Log } from "../portalTypes";
import { Dialog, ErrorMessage, useData, date } from "./common";
import "asciinema-player/dist/bundle/asciinema-player.css";

function Playback({ url, onClose }: { url: string; onClose: () => void }) {
  const container = useRef<HTMLDivElement>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    let disposed = false;
    let player: { dispose?: () => void } | undefined;
    let objectURL = "";
    const controller = new AbortController();
    void Promise.all([
      import("asciinema-player"),
      fetch(url, { credentials: "same-origin", signal: controller.signal }),
    ])
      .then(async ([module, response]) => {
        if (!response.ok) throw new Error("録画を読み込めませんでした");
        const blob = await response.blob();
        if (disposed || !container.current) return;
        objectURL = URL.createObjectURL(blob);
        player = module.create(objectURL, container.current, {
          fit: "both",
          terminalFontSize: "small",
        });
      })
      .catch((e) => {
        if (!disposed) setError((e as Error).message);
      });
    return () => {
      disposed = true;
      controller.abort();
      player?.dispose?.();
      if (objectURL) URL.revokeObjectURL(objectURL);
    };
  }, [url]);
  return (
    <Dialog title="接続の録画" onClose={onClose}>
      <ErrorMessage message={error} />
      <div ref={container} />
    </Dialog>
  );
}
export function Logs({ admin }: { admin: boolean }) {
  const prefix = `/api/${admin ? "admin" : "app"}`;
  const initial = useData<{ items: Log[]; next_cursor: string }>(
    `${prefix}/logs`,
    { items: [], next_cursor: "" },
  );
  const [extra, setExtra] = useState<Log[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [playback, setPlayback] = useState("");
  const entries = [...initial.data.items, ...extra];
  const next = cursor ?? initial.data.next_cursor;
  async function more() {
    setBusy(true);
    try {
      const result = await apiFetch<{ items: Log[]; next_cursor: string }>(
        `${prefix}/logs?cursor=${encodeURIComponent(next)}`,
      );
      setExtra((items) => [...items, ...result.items]);
      setCursor(result.next_cursor);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <section>
      <h1>{admin ? "全ユーザーの接続履歴" : "自分の接続履歴"}</h1>
      <p>録画には端末に表示された機密情報が含まれることがあります。</p>
      <ErrorMessage message={error || initial.error} />
      {initial.loading ? (
        <p>読み込み中…</p>
      ) : entries.length === 0 ? (
        <p>まだ接続履歴がありません。</p>
      ) : (
        <div className="portal-table-wrap">
          <table>
            <thead>
              <tr>
                {admin && <th>所有者</th>}
                <th>SSH 接続</th>
                <th>開始</th>
                <th>終了</th>
                <th>結果</th>
                <th>録画</th>
              </tr>
            </thead>
            <tbody>
              {entries.map((l) => (
                <tr key={l.id}>
                  {admin && <td>{l.owner_user_id || "移行前の履歴"}</td>}
                  <td>
                    {l.user}@{l.host}:{l.port}
                  </td>
                  <td>{date(l.connected_at)}</td>
                  <td>{date(l.disconnected_at)}</td>
                  <td>{l.error || l.termination_reason || "接続中"}</td>
                  <td>
                    {l.has_recording && (
                      <button
                        onClick={() =>
                          setPlayback(`${prefix}/recordings/${l.id}`)
                        }
                      >
                        再生
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {next && (
        <button disabled={busy} onClick={() => void more()}>
          次の 50 件
        </button>
      )}
      {playback && <Playback url={playback} onClose={() => setPlayback("")} />}
    </section>
  );
}
