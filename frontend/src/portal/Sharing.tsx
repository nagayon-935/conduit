import { useState } from "react";
import { mutate } from "../api/fetch";
import type { Share } from "../portalTypes";
import { Dialog, ErrorMessage, Toggle, useData, date } from "./common";

export function Sharing({
  sessionId,
  onClose,
}: {
  sessionId: string;
  onClose: () => void;
}) {
  const candidates = useData<{ id: string; display_name: string }[]>(
    `/api/app/sessions/${sessionId}/share-candidates`,
    [],
  );
  const shares = useData<Share[]>(`/api/app/sessions/${sessionId}/shares`, []);
  const [recipients, setRecipients] = useState<string[]>([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [url, setUrl] = useState("");
  async function create() {
    setBusy(true);
    setError("");
    try {
      const result = await mutate<{ url: string }>(
        `/api/app/sessions/${sessionId}/shares`,
        { recipient_user_ids: recipients },
      );
      setUrl(new URL(result.url, location.origin).href);
      shares.refresh();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function revoke(id: string) {
    setBusy(true);
    try {
      await mutate(`/api/app/sessions/${sessionId}/shares/${id}`, {}, "DELETE");
      shares.refresh();
      setUrl("");
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog title="読み取り専用で共有" onClose={onClose}>
      <p>
        選択したユーザーだけが、ログイン後に閲覧できます。入力や端末のサイズ変更はできません。
      </p>
      <ErrorMessage message={error || candidates.error || shares.error} />
      {candidates.loading ? (
        <p>読み込み中…</p>
      ) : candidates.data.length === 0 ? (
        <p>
          共有できるユーザーがいません。管理者に閲覧権限を依頼してください。
        </p>
      ) : (
        candidates.data.map((u) => (
          <Toggle
            key={u.id}
            label={u.display_name}
            checked={recipients.includes(u.id)}
            onChange={(checked) =>
              setRecipients((ids) =>
                checked ? [...ids, u.id] : ids.filter((id) => id !== u.id),
              )
            }
          />
        ))
      )}
      <button
        disabled={busy || recipients.length === 0}
        onClick={() => void create()}
      >
        共有リンクを作成
      </button>
      {url && (
        <label className="portal-field">
          共有リンク
          <input readOnly value={url} onFocus={(e) => e.target.select()} />
          <button
            onClick={() =>
              navigator.clipboard
                .writeText(url)
                .catch(() => setError("リンクを選択してコピーしてください"))
            }
          >
            コピー
          </button>
        </label>
      )}
      <h3>有効な共有</h3>
      {shares.data.map((share) => (
        <div className="portal-row" key={share.id}>
          <span>
            {share.recipient_user_ids
              .map(
                (id) =>
                  candidates.data.find((u) => u.id === id)?.display_name ?? id,
              )
              .join(", ")}{" "}
            · {date(share.expires_at)} まで
          </span>
          <button disabled={busy} onClick={() => void revoke(share.id)}>
            共有を停止
          </button>
        </div>
      ))}
    </Dialog>
  );
}
