import { useEffect, useState, type FormEvent } from "react";
import { apiFetch, mutate } from "../api/fetch";
import type {
  Account,
  Grant,
  Session,
  Share,
  Target,
  User,
} from "../portalTypes";
import { Dialog, ErrorMessage, Field, Toggle, useData, date } from "./common";
import { Logs } from "./Logs";
import { LegacyProfiles } from "./LegacyProfiles";

export const adminPages: Record<string, string> = {
  users: "ユーザー",
  targets: "接続先",
  grants: "接続・閲覧権限",
  sessions: "セッション",
  shares: "共有",
  logs: "接続履歴",
  audit: "監査ログ",
  settings: "運用設定",
  health: "稼働状態",
};

function useAction() {
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  async function run(action: () => Promise<void>) {
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await action();
      setMessage("保存しました");
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return { error, busy, message, run };
}
function Feedback({ action }: { action: ReturnType<typeof useAction> }) {
  return (
    <>
      <ErrorMessage message={action.error} />
      {action.message && <p role="status">{action.message}</p>}
    </>
  );
}

function Users() {
  const list = useData<User[]>("/api/admin/users", []);
  const [editing, setEditing] = useState<User | null>(null);
  return (
    <section>
      <h1>ユーザー管理</h1>
      <p>
        Conduit のログインユーザーを管理します。SSH
        のユーザー名は接続先ごとに設定します。
      </p>
      <ErrorMessage message={list.error} />
      <button
        onClick={() =>
          setEditing({
            id: "",
            login: "",
            display_name: "",
            role: "user",
            enabled: true,
            must_change_password: true,
          })
        }
      >
        ユーザーを追加
      </button>
      {list.loading && <p>読み込み中…</p>}
      <div className="portal-table-wrap">
        <table>
          <thead>
            <tr>
              <th>表示名</th>
              <th>ログイン名</th>
              <th>役割</th>
              <th>状態</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {list.data.map((u) => (
              <tr key={u.id}>
                <td>{u.display_name}</td>
                <td>{u.login}</td>
                <td>{u.role === "admin" ? "管理者" : "ユーザー"}</td>
                <td>
                  {!u.enabled
                    ? "無効"
                    : u.must_change_password
                      ? "パスワード変更待ち"
                      : "有効"}
                </td>
                <td>
                  <button onClick={() => setEditing(u)}>編集</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {editing && (
        <UserEditor
          key={editing.id}
          value={editing}
          onClose={() => setEditing(null)}
          onSaved={() => {
            list.refresh();
            setEditing(null);
          }}
        />
      )}
    </section>
  );
}
function UserEditor({
  value,
  onClose,
  onSaved,
}: {
  value: User;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [u, setUser] = useState(value);
  const [password, setPassword] = useState("");
  const action = useAction();
  const payload = {
    display_name: u.display_name,
    role: u.role,
    enabled: u.enabled,
  };
  return (
    <Dialog
      title={u.id ? "ユーザーを編集" : "ユーザーを追加"}
      onClose={onClose}
    >
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void action.run(async () => {
            if (u.id)
              await mutate(`/api/admin/users/${u.id}`, payload, "PATCH");
            else
              await mutate("/api/admin/users", {
                login: u.login,
                display_name: u.display_name,
                role: u.role,
                password,
              });
            onSaved();
          });
        }}
      >
        <Field label="ログイン名">
          <input
            value={u.login}
            disabled={!!u.id}
            required
            onChange={(e) => setUser({ ...u, login: e.target.value })}
          />
        </Field>
        <Field label="表示名">
          <input
            value={u.display_name}
            required
            onChange={(e) => setUser({ ...u, display_name: e.target.value })}
          />
        </Field>
        <Field label="役割">
          <select
            value={u.role}
            onChange={(e) =>
              setUser({ ...u, role: e.target.value as User["role"] })
            }
          >
            <option value="user">ユーザー</option>
            <option value="admin">管理者</option>
          </select>
        </Field>
        {u.id && (
          <>
            <Toggle
              label="有効"
              checked={u.enabled}
              onChange={(enabled) => setUser({ ...u, enabled })}
            />
            <p>
              無効化すると、このユーザーの SSH
              接続と共有も終了します。役割を変更すると、ログインし直す必要があります。
            </p>
          </>
        )}
        {!u.id && (
          <Field label="初回パスワード（12 文字以上）">
            <input
              type="password"
              minLength={12}
              required
              autoComplete="new-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </Field>
        )}
        <Feedback action={action} />
        <button disabled={action.busy}>保存</button>
      </form>
      {u.id && (
        <form
          className="portal-danger"
          onSubmit={(e) => {
            e.preventDefault();
            void action.run(async () => {
              await mutate(`/api/admin/users/${u.id}/reset-password`, {
                password,
              });
              setPassword("");
              onSaved();
            });
          }}
        >
          <h3>パスワードをリセット</h3>
          <p>
            すべてのログインと SSH
            接続を終了します。次のログイン時に変更が必要です。
          </p>
          <Field label="仮パスワード">
            <input
              type="password"
              minLength={12}
              required
              autoComplete="new-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </Field>
          <button disabled={action.busy}>リセット</button>
        </form>
      )}
    </Dialog>
  );
}
const emptyTarget: Target = {
  id: "",
  name: "",
  host: "",
  port: 22,
  environment: "development",
  enabled: true,
};
const emptyAccount: Account = {
  id: "",
  target_id: "",
  ssh_username: "",
  auth_type: "vault",
  enabled: true,
  sharing_enabled: false,
  recording_enabled: true,
};
function useAccounts(targets: Target[]) {
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [error, setError] = useState("");
  useEffect(() => {
    const controller = new AbortController();
    void Promise.all(
      targets.map((t) =>
        apiFetch<Account[]>(`/api/admin/targets/${t.id}/accounts`, {
          signal: controller.signal,
        }),
      ),
    )
      .then((groups) => {
        setAccounts(groups.flat());
        setError("");
      })
      .catch((e) => {
        if (!controller.signal.aborted) setError((e as Error).message);
      });
    return () => controller.abort();
  }, [targets]);
  return { accounts, error };
}
function Targets() {
  const [legacy, setLegacy] = useState(false);
  const list = useData<Target[]>("/api/admin/targets", []);
  const all = useAccounts(list.data);
  const [editing, setEditing] = useState<Target | null>(null);
  const [account, setAccount] = useState<Account | null>(null);
  return (
    <section>
      <h1>接続先管理</h1>
      <p>
        接続先、SSH
        アカウント、踏み台を登録します。変更すると、その設定を使用中の接続は終了します。
      </p>
      <ErrorMessage message={list.error || all.error} />
      <button onClick={() => setEditing(emptyTarget)}>接続先を追加</button>
      <button onClick={() => setLegacy(true)}>旧プロファイルを確認</button>
      {legacy && (
        <LegacyProfiles
          onClose={() => setLegacy(false)}
          onImported={list.refresh}
        />
      )}
      {!list.loading && list.data.length === 0 && (
        <div className="portal-empty">
          <h2>最初の接続先を登録しましょう</h2>
          <p>
            接続先を登録した後、SSH
            アカウントを追加し、ユーザーに接続権限を付与します。
          </p>
        </div>
      )}
      {list.data.map((t) => (
        <article className="portal-card" key={t.id}>
          <div className="portal-row">
            <h2>
              {t.name}{" "}
              <small>
                {t.environment} · {t.enabled ? "有効" : "無効"}
              </small>
            </h2>
            <button onClick={() => setEditing(t)}>接続先を編集</button>
          </div>
          <p>
            {t.host}:{t.port}
            {t.jump_account_id &&
              ` · 踏み台: ${all.accounts.find((a) => a.id === t.jump_account_id)?.ssh_username ?? t.jump_account_id}`}
          </p>
          <div className="portal-table-wrap">
            <table>
              <thead>
                <tr>
                  <th>SSH ユーザー</th>
                  <th>認証方式</th>
                  <th>共有</th>
                  <th>録画</th>
                  <th>状態</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {all.accounts
                  .filter((a) => a.target_id === t.id)
                  .map((a) => (
                    <tr key={a.id}>
                      <td>{a.ssh_username}</td>
                      <td>{a.auth_type}</td>
                      <td>{a.sharing_enabled ? "許可" : "不可"}</td>
                      <td>{a.recording_enabled ? "有効" : "無効"}</td>
                      <td>{a.enabled ? "有効" : "無効"}</td>
                      <td>
                        <button onClick={() => setAccount(a)}>編集</button>
                      </td>
                    </tr>
                  ))}
              </tbody>
            </table>
          </div>
          <button
            onClick={() => setAccount({ ...emptyAccount, target_id: t.id })}
          >
            SSH アカウントを追加
          </button>
        </article>
      ))}
      {editing && (
        <TargetEditor
          key={editing.id}
          value={editing}
          accounts={all.accounts}
          targets={list.data}
          onClose={() => setEditing(null)}
          onSaved={() => {
            list.refresh();
            setEditing(null);
          }}
        />
      )}
      {account && (
        <AccountEditor
          key={account.id}
          value={account}
          onClose={() => setAccount(null)}
          onSaved={() => {
            list.refresh();
            setAccount(null);
          }}
        />
      )}
    </section>
  );
}
function TargetEditor({
  value,
  accounts,
  targets,
  onClose,
  onSaved,
}: {
  value: Target;
  accounts: Account[];
  targets: Target[];
  onClose: () => void;
  onSaved: () => void;
}) {
  const [t, setTarget] = useState(value);
  const action = useAction();
  return (
    <Dialog title={t.id ? "接続先を編集" : "接続先を追加"} onClose={onClose}>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void action.run(async () => {
            await mutate(
              t.id ? `/api/admin/targets/${t.id}` : "/api/admin/targets",
              t,
              t.id ? "PATCH" : "POST",
            );
            onSaved();
          });
        }}
      >
        <Field label="接続先名">
          <input
            required
            value={t.name}
            onChange={(e) => setTarget({ ...t, name: e.target.value })}
          />
        </Field>
        <Field label="ホスト名または IP">
          <input
            required
            value={t.host}
            onChange={(e) => setTarget({ ...t, host: e.target.value })}
          />
        </Field>
        <Field label="SSH ポート">
          <input
            type="number"
            min={1}
            max={65535}
            required
            value={t.port}
            onChange={(e) => setTarget({ ...t, port: Number(e.target.value) })}
          />
        </Field>
        <Field label="環境">
          <input
            value={t.environment}
            onChange={(e) => setTarget({ ...t, environment: e.target.value })}
            placeholder="production / staging / development"
          />
        </Field>
        <Field label="踏み台アカウント">
          <select
            value={t.jump_account_id ?? ""}
            onChange={(e) =>
              setTarget({ ...t, jump_account_id: e.target.value })
            }
          >
            <option value="">踏み台なし</option>
            {accounts
              .filter(
                (a) =>
                  a.target_id !== t.id &&
                  !targets.find((target) => target.id === a.target_id)
                    ?.jump_account_id,
              )
              .map((a) => (
                <option key={a.id} value={a.id}>
                  {a.ssh_username}@
                  {targets.find((target) => target.id === a.target_id)?.name}
                </option>
              ))}
          </select>
        </Field>
        <Toggle
          label="有効"
          checked={t.enabled}
          onChange={(enabled) => setTarget({ ...t, enabled })}
        />
        <Feedback action={action} />
        <button disabled={action.busy}>保存</button>
      </form>
    </Dialog>
  );
}
function AccountEditor({
  value,
  onClose,
  onSaved,
}: {
  value: Account;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [a, setAccount] = useState(value);
  const action = useAction();
  return (
    <Dialog title="SSH アカウントを設定" onClose={onClose}>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void action.run(async () => {
            await mutate(
              `/api/admin/targets/${a.target_id}/accounts${a.id ? `/${a.id}` : ""}`,
              a,
              a.id ? "PATCH" : "POST",
            );
            onSaved();
          });
        }}
      >
        <Field label="SSH ユーザー名">
          <input
            required
            value={a.ssh_username}
            onChange={(e) => setAccount({ ...a, ssh_username: e.target.value })}
          />
        </Field>
        <Field label="SSH 認証方式">
          <select
            value={a.auth_type}
            onChange={(e) =>
              setAccount({
                ...a,
                auth_type: e.target.value as Account["auth_type"],
              })
            }
          >
            <option value="vault">Vault 証明書</option>
            <option value="password">SSH パスワード</option>
            <option value="pubkey">SSH 秘密鍵</option>
          </select>
        </Field>
        <p>
          SSH
          パスワードや秘密鍵は接続時にユーザーが入力します。サーバーには保存しません。
        </p>
        <Toggle
          label="有効"
          checked={a.enabled}
          onChange={(enabled) => setAccount({ ...a, enabled })}
        />
        <Toggle
          label="読み取り専用共有を許可"
          checked={a.sharing_enabled}
          onChange={(sharing_enabled) => setAccount({ ...a, sharing_enabled })}
        />
        <Toggle
          label="端末を録画（サーバーの録画設定が有効な場合）"
          checked={a.recording_enabled}
          onChange={(recording_enabled) =>
            setAccount({ ...a, recording_enabled })
          }
        />
        <Feedback action={action} />
        <button disabled={action.busy}>保存</button>
      </form>
    </Dialog>
  );
}
function Grants() {
  const users = useData<User[]>("/api/admin/users", []);
  const targets = useData<Target[]>("/api/admin/targets", []);
  const grants = useData<Grant[]>("/api/admin/access-grants", []);
  const all = useAccounts(targets.data);
  const [uid, setUid] = useState("");
  const [aid, setAid] = useState("");
  const [connect, setConnect] = useState(false);
  const [view, setView] = useState(false);
  const action = useAction();
  useEffect(() => {
    const g = grants.data.find(
      (g) => g.user_id === uid && g.target_account_id === aid,
    );
    setConnect(g?.can_connect ?? false);
    setView(g?.can_view_shared ?? false);
  }, [uid, aid, grants.data]);
  return (
    <section>
      <h1>接続・閲覧権限</h1>
      <p>
        管理者にも接続権限を明示的に付与します。登録された踏み台経路の通過は、接続先への権限で許可します。踏み台への直接ログインは別に許可してください。
      </p>
      <ErrorMessage
        message={users.error || targets.error || grants.error || all.error}
      />
      <form
        className="portal-card"
        onSubmit={(e) => {
          e.preventDefault();
          void action.run(async () => {
            await mutate(
              `/api/admin/users/${uid}/access-grants/${aid}`,
              { can_connect: connect, can_view_shared: view },
              "PUT",
            );
            grants.refresh();
          });
        }}
      >
        <Field label="Conduit ユーザー">
          <select required value={uid} onChange={(e) => setUid(e.target.value)}>
            <option value="">選択してください</option>
            {users.data.map((u) => (
              <option key={u.id} value={u.id}>
                {u.display_name} ({u.login})
              </option>
            ))}
          </select>
        </Field>
        <Field label="SSH アカウント">
          <select required value={aid} onChange={(e) => setAid(e.target.value)}>
            <option value="">選択してください</option>
            {all.accounts.map((a) => (
              <option key={a.id} value={a.id}>
                {a.ssh_username}@
                {targets.data.find((t) => t.id === a.target_id)?.name}
              </option>
            ))}
          </select>
        </Field>
        <Toggle
          label="SSH 接続を許可"
          checked={connect}
          onChange={setConnect}
        />
        <Toggle
          label="指定された共有の閲覧を許可"
          checked={view}
          onChange={setView}
        />
        <p>
          接続権限を取り消すと、対象の SSH
          接続も終了します。閲覧権限を取り消すと、共有の閲覧を停止します。
        </p>
        <Feedback action={action} />
        <button disabled={action.busy || !uid || !aid}>権限を保存</button>
      </form>
      <div className="portal-table-wrap">
        <table>
          <thead>
            <tr>
              <th>ユーザー</th>
              <th>SSH 接続</th>
              <th>接続</th>
              <th>閲覧</th>
            </tr>
          </thead>
          <tbody>
            {grants.data.map((g) => (
              <tr key={`${g.user_id}:${g.target_account_id}`}>
                <td>
                  {users.data.find((u) => u.id === g.user_id)?.display_name}
                </td>
                <td>
                  {
                    all.accounts.find((a) => a.id === g.target_account_id)
                      ?.ssh_username
                  }
                  @
                  {
                    targets.data.find(
                      (t) =>
                        t.id ===
                        all.accounts.find((a) => a.id === g.target_account_id)
                          ?.target_id,
                    )?.name
                  }
                </td>
                <td>{g.can_connect ? "許可" : "不可"}</td>
                <td>{g.can_view_shared ? "許可" : "不可"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}
function Sessions() {
  const list = useData<Session[]>("/api/admin/sessions", []);
  const [ending, setEnding] = useState<Session | null>(null);
  const [reason, setReason] = useState("");
  const action = useAction();
  return (
    <section>
      <h1>全ユーザーのセッション</h1>
      <p>管理者は接続の状態を確認し、理由を記録して終了できます。</p>
      <ErrorMessage message={list.error} />
      <button onClick={list.refresh}>更新</button>
      <div className="portal-table-wrap">
        <table>
          <thead>
            <tr>
              <th>所有者</th>
              <th>SSH 接続</th>
              <th>状態</th>
              <th>閲覧者</th>
              <th>作成</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {list.data.map((s) => (
              <tr key={s.id}>
                <td>{s.owner_user_id}</td>
                <td>
                  {s.user}@{s.host}:{s.port}
                </td>
                <td>{s.state}</td>
                <td>{s.viewer_count}</td>
                <td>{new Date(s.created_at).toLocaleString()}</td>
                <td>
                  <button
                    onClick={() => {
                      setEnding(s);
                      setReason("");
                    }}
                  >
                    終了
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {ending && (
        <Dialog title="セッションを終了" onClose={() => setEnding(null)}>
          <p>
            {ending.user}@{ending.host} の SSH プロセスを終了します。
          </p>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              void action.run(async () => {
                await mutate(
                  `/api/admin/sessions/${ending.id}`,
                  { reason },
                  "DELETE",
                );
                list.refresh();
                setEnding(null);
              });
            }}
          >
            <Field label="終了理由（監査ログに記録）">
              <textarea
                required
                value={reason}
                onChange={(e) => setReason(e.target.value)}
              />
            </Field>
            <Feedback action={action} />
            <button disabled={action.busy}>セッションを終了</button>
          </form>
        </Dialog>
      )}
    </section>
  );
}
function Shares() {
  const list = useData<Share[]>("/api/admin/shares", []);
  const [editing, setEditing] = useState<Share | null>(null);
  const [reason, setReason] = useState("");
  const action = useAction();
  return (
    <section>
      <h1>共有の管理</h1>
      <ErrorMessage message={list.error} />
      <button onClick={list.refresh}>更新</button>
      {list.data.map((s) => (
        <article key={s.id} className="portal-card">
          <p>
            セッション {s.session_id} · {s.recipient_user_ids.length} 名 ·{" "}
            {date(s.expires_at)} まで
          </p>
          <button
            onClick={() => {
              setEditing(s);
              setReason("");
            }}
          >
            共有を停止
          </button>
        </article>
      ))}
      {editing && (
        <Dialog title="共有を停止" onClose={() => setEditing(null)}>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              void action.run(async () => {
                await mutate(
                  `/api/admin/shares/${editing.id}`,
                  { reason },
                  "DELETE",
                );
                setEditing(null);
                list.refresh();
              });
            }}
          >
            <Field label="停止理由">
              <textarea
                required
                value={reason}
                onChange={(e) => setReason(e.target.value)}
              />
            </Field>
            <Feedback action={action} />
            <button disabled={action.busy}>共有を停止</button>
          </form>
        </Dialog>
      )}
    </section>
  );
}
interface Audit {
  id: number;
  actor_user_id: string;
  action: string;
  resource_id: string;
  reason: string;
  created_at: number;
}
function AuditLog() {
  const [cursor, setCursor] = useState(0);
  const list = useData<Audit[]>(`/api/admin/audit-events?cursor=${cursor}`, []);
  return (
    <section>
      <h1>監査ログ</h1>
      <p>
        ユーザー、接続先、権限、共有、管理者による終了・録画閲覧の履歴です。
      </p>
      <ErrorMessage message={list.error} />
      <div className="portal-table-wrap">
        <table>
          <thead>
            <tr>
              <th>日時</th>
              <th>操作者</th>
              <th>操作</th>
              <th>対象</th>
              <th>理由</th>
            </tr>
          </thead>
          <tbody>
            {list.data.map((a) => (
              <tr key={a.id}>
                <td>{date(a.created_at)}</td>
                <td>{a.actor_user_id}</td>
                <td>{a.action}</td>
                <td>{a.resource_id}</td>
                <td>{a.reason}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {cursor > 0 && <button onClick={() => setCursor(0)}>最新に戻る</button>}
      {list.data.length === 50 && (
        <button onClick={() => setCursor(list.data[49].id)}>次の 50 件</button>
      )}
    </section>
  );
}
interface Policy {
  revision: number;
  reconnect_grace_minutes: number;
  ssh_idle_minutes: number;
  login_absolute_hours: number;
  login_idle_minutes: number;
  share_ttl_minutes: number;
  recording_enabled: boolean;
  recording_retention_days: number;
  log_retention_days: number;
  audit_retention_days: number;
  recording_max_bytes: number;
}
const policyLabels = {
  reconnect_grace_minutes: "再接続の猶予（分）",
  ssh_idle_minutes: "SSH の入力がない場合の終了期限（分、0 は無効）",
  login_absolute_hours: "ログインの絶対期限（時間）",
  login_idle_minutes: "ログインの無操作期限（分）",
  share_ttl_minutes: "共有の有効期間（分）",
  recording_retention_days: "録画の保存期間（日）",
  log_retention_days: "接続履歴の保存期間（日）",
  audit_retention_days: "監査ログの保存期間（日）",
  recording_max_bytes: "終了済み録画の容量上限（bytes）",
};
function Settings() {
  const list = useData<Policy>("/api/admin/settings", {
    revision: 1,
    reconnect_grace_minutes: 15,
    ssh_idle_minutes: 30,
    login_absolute_hours: 8,
    login_idle_minutes: 60,
    recording_enabled: false,
    share_ttl_minutes: 60,
    recording_retention_days: 30,
    log_retention_days: 90,
    audit_retention_days: 180,
    recording_max_bytes: 10737418240,
  });
  const action = useAction();
  function submit(e: FormEvent) {
    e.preventDefault();
    void action.run(async () => {
      await mutate("/api/admin/settings", list.data, "PATCH");
      list.refresh();
    });
  }
  return (
    <section>
      <h1>運用設定</h1>
      <p>
        期限、録画、保存期間を設定します。録画中のファイルは削除しません。接続先ネットワーク、Vault、TLS、known_hosts
        はサーバー設定で管理します。
      </p>
      <ErrorMessage message={list.error} />
      <form className="portal-card" onSubmit={submit}>
        {(Object.keys(policyLabels) as (keyof typeof policyLabels)[]).map(
          (key) => (
            <Field key={key} label={policyLabels[key]}>
              <input
                type="number"
                required
                min={1}
                value={list.data[key]}
                onChange={(e) =>
                  list.setData({ ...list.data, [key]: Number(e.target.value) })
                }
              />
            </Field>
          ),
        )}
        <Toggle
          label="端末出力の録画を有効にする"
          checked={list.data.recording_enabled}
          onChange={(recording_enabled) =>
            list.setData({ ...list.data, recording_enabled })
          }
        />
        <p>
          録画の有効・無効を切り替えると、現在の SSH
          セッションを終了します。期限の変更は既存のログイン・SSH
          にも反映します。
        </p>
        <Feedback action={action} />
        <button disabled={action.busy || list.loading}>設定を保存</button>
      </form>
    </section>
  );
}
function Health() {
  const list = useData<{
    status: string;
    active_sessions: number;
    storage: string;
    recording_enabled: boolean;
  }>("/api/admin/health", {
    status: "",
    active_sessions: 0,
    storage: "",
    recording_enabled: false,
  });
  return (
    <section>
      <h1>稼働状態</h1>
      <ErrorMessage message={list.error} />
      <button onClick={list.refresh}>更新</button>
      <dl className="portal-card">
        <dt>サーバー</dt>
        <dd>{list.data.status}</dd>
        <dt>SSH セッション</dt>
        <dd>{list.data.active_sessions}</dd>
        <dt>保存先</dt>
        <dd>{list.data.storage}</dd>
        <dt>録画</dt>
        <dd>{list.data.recording_enabled ? "有効" : "無効"}</dd>
      </dl>
    </section>
  );
}
export function Admin({ page }: { page: string }) {
  switch (page) {
    case "targets":
      return <Targets />;
    case "grants":
      return <Grants />;
    case "sessions":
      return <Sessions />;
    case "shares":
      return <Shares />;
    case "logs":
      return <Logs admin />;
    case "audit":
      return <AuditLog />;
    case "settings":
      return <Settings />;
    case "health":
      return <Health />;
    default:
      return <Users />;
  }
}
