import { useState } from "react";
import { mutate } from "../api/fetch";
import type { Target, Account } from "../portalTypes";
import { Dialog, ErrorMessage, Toggle } from "./common";

interface Candidate {
  name: string;
  host: string;
  port: number;
  user: string;
  auth: Account["auth_type"];
  jump: boolean;
}
function candidates(): Candidate[] {
  try {
    const raw: unknown = JSON.parse(
      localStorage.getItem("conduit-profiles") ?? "[]",
    );
    if (!Array.isArray(raw)) return [];
    return raw.slice(0, 100).flatMap((value: Record<string, unknown>) => {
      if (
        !value ||
        typeof value.host !== "string" ||
        typeof value.user !== "string" ||
        !value.user
      )
        return [];
      return [
        {
          name: typeof value.name === "string" ? value.name : value.host,
          host: value.host,
          port: typeof value.port === "number" ? value.port : 22,
          user: value.user,
          auth: (["vault", "password", "pubkey"].includes(
            String(value.authType),
          )
            ? value.authType
            : "vault") as Account["auth_type"],
          jump: !!value.jumpHost,
        },
      ];
    });
  } catch {
    return [];
  }
}
export function clearLegacyData() {
  try {
    [
      "conduit-session",
      "conduit-workspace",
      "conduit-layout",
      "conduit-history",
      "conduit-profiles",
      "conduit-fontSize",
      "conduit-theme",
    ].forEach((key) => localStorage.removeItem(key));
    Object.keys(sessionStorage)
      .filter((key) => key.startsWith("conduit-output:"))
      .forEach((key) => sessionStorage.removeItem(key));
    sessionStorage.removeItem("conduit_ck");
    sessionStorage.removeItem("conduit:search-history");
  } catch {
    /* unavailable storage */
  }
}
export function LegacyProfiles({
  onClose,
  onImported,
}: {
  onClose: () => void;
  onImported: () => void;
}) {
  const [items] = useState(candidates);
  const [selected, setSelected] = useState<number[]>([]);
  const [completed, setCompleted] = useState<number[]>([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  async function importSelected() {
    setBusy(true);
    setError("");
    try {
      for (const index of selected.filter(
        (index) => !completed.includes(index),
      )) {
        const c = items[index];
        const target = await mutate<Target>("/api/admin/targets", {
          name: c.name,
          host: c.host,
          port: c.port,
          environment: "未分類",
          enabled: false,
        });
        try {
          await mutate(`/api/admin/targets/${target.id}/accounts`, {
            ssh_username: c.user,
            auth_type: c.auth,
            enabled: false,
            sharing_enabled: false,
            recording_enabled: true,
          });
        } catch (e) {
          throw new Error(
            `${c.name}: 接続先は作成しました。SSH アカウントは管理画面で追加してください。${(e as Error).message}`,
          );
        } finally {
          setCompleted((ids) => [...ids, index]);
          onImported();
        }
      }
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog title="旧プロファイルの移行" onClose={onClose}>
      <p>
        このブラウザの旧プロファイルから、接続先と SSH
        ユーザー名だけを候補として読み取ります。秘密鍵は読み込みません。選択した接続先を無効状態で登録します。内容・環境・踏み台を確認してから有効化し、権限を設定してください。
      </p>
      <ErrorMessage message={error} />
      {items.length === 0 ? (
        <p>このブラウザに移行候補はありません。</p>
      ) : (
        items.map((c, index) => (
          <div key={index}>
            <Toggle
              checked={selected.includes(index)}
              label={`${c.name} · ${c.user}@${c.host}:${c.port}${completed.includes(index) ? " · 登録済み" : ""}`}
              onChange={(checked) =>
                setSelected((ids) =>
                  checked ? [...ids, index] : ids.filter((id) => id !== index),
                )
              }
            />
            {c.jump && (
              <p>
                踏み台設定があります。管理画面で新しい経路を設定してください。
              </p>
            )}
          </div>
        ))
      )}
      <button
        disabled={busy || !selected.some((index) => !completed.includes(index))}
        onClick={() => void importSelected()}
      >
        確認した接続先を登録
      </button>
      <div className="portal-danger">
        <p>
          移行後に旧ブラウザデータを消去できます。古い鍵・トークン・プロファイルの保存データを削除します。
        </p>
        <button
          disabled={busy}
          onClick={() => {
            clearLegacyData();
            onClose();
          }}
        >
          旧データを消去
        </button>
      </div>
    </Dialog>
  );
}
