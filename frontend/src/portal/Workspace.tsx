import { useEffect, useMemo, useRef, useState } from "react";
import { apiFetch, mutate } from "../api/fetch";
import type {
  Destination,
  Log,
  Preferences,
  Session,
  SharedSession,
} from "../portalTypes";
import type { SessionTab, LayoutType } from "../types";
import { TerminalPool } from "../components/TerminalPool";
import { TabBar } from "../components/TabBar";
import { useSplitLayout } from "../hooks/useSplitLayout";
import { Dialog, ErrorMessage, Field, Toggle, useData, date } from "./common";
import { themes } from "../themes";
import { clearLegacyData } from "./LegacyProfiles";

interface StoredWorkspace {
  ids: { id: string; share?: string }[];
  active: string | null;
  type: LayoutType;
  paneIds: (string | null)[];
  ratioV: number;
  ratioH: number;
}
function readWorkspace(uid: string): StoredWorkspace {
  const empty: StoredWorkspace = {
    ids: [],
    active: null,
    type: "1",
    paneIds: [null, null, null, null],
    ratioV: 0.5,
    ratioH: 0.5,
  };
  try {
    const data = JSON.parse(
      localStorage.getItem(`conduit:workspace:${uid}`) ?? "null",
    ) as StoredWorkspace | null;
    if (
      !data ||
      !Array.isArray(data.ids) ||
      !Array.isArray(data.paneIds) ||
      !["1", "2v", "2h", "4"].includes(data.type)
    )
      return empty;
    return {
      ...empty,
      ...data,
      ids: data.ids
        .slice(0, 32)
        .filter(
          (x) =>
            typeof x.id === "string" &&
            (!x.share || typeof x.share === "string"),
        ),
      ratioV: Math.max(0.2, Math.min(0.8, data.ratioV || 0.5)),
      ratioH: Math.max(0.2, Math.min(0.8, data.ratioH || 0.5)),
    };
  } catch {
    return empty;
  }
}
function tab(s: Session, target?: Destination, share?: string): SessionTab {
  return {
    id: share ? `shared:${share}` : s.id,
    sessionToken: s.id,
    host: s.host,
    port: s.port,
    user: s.user,
    expiresAt: s.expires_at,
    shareToken: share,
    environment: target?.target.environment,
    recording: s.recording_enabled,
  };
}

function CredentialFields({
  auth,
  value,
  onChange,
  label,
}: {
  auth: string;
  value: { password: string; private_key: string; passphrase: string };
  onChange: (value: {
    password: string;
    private_key: string;
    passphrase: string;
  }) => void;
  label: string;
}) {
  return (
    <fieldset>
      <legend>{label}</legend>
      {auth === "vault" ? (
        <p>Vault の証明書で認証します。</p>
      ) : auth === "password" ? (
        <Field label="SSH パスワード">
          <input
            type="password"
            required
            autoComplete="off"
            value={value.password}
            onChange={(e) => onChange({ ...value, password: e.target.value })}
          />
        </Field>
      ) : (
        <>
          <Field label="SSH 秘密鍵（PEM / OpenSSH）">
            <textarea
              required
              rows={6}
              value={value.private_key}
              onChange={(e) =>
                onChange({ ...value, private_key: e.target.value })
              }
            />
          </Field>
          <Field label="鍵ファイルを選択">
            <input
              type="file"
              onChange={(e) => {
                const file = e.target.files?.[0];
                if (file && file.size <= 512000)
                  void file
                    .text()
                    .then((private_key) => onChange({ ...value, private_key }))
                    .catch(() => {});
                e.target.value = "";
              }}
            />
          </Field>
          <Field label="鍵のパスフレーズ（暗号化されている場合）">
            <input
              type="password"
              autoComplete="off"
              value={value.passphrase}
              onChange={(e) =>
                onChange({ ...value, passphrase: e.target.value })
              }
            />
          </Field>
        </>
      )}
    </fieldset>
  );
}
function ConnectDialog({
  destination,
  onClose,
  onConnected,
}: {
  destination: Destination;
  onClose: () => void;
  onConnected: (session: Session) => void;
}) {
  const empty = { password: "", private_key: "", passphrase: "" };
  const [credentials, setCredentials] = useState(empty);
  const [jump, setJump] = useState(empty);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [confirmed, setConfirmed] = useState(false);
  const controller = useRef<AbortController | null>(null);
  useEffect(
    () => () => {
      controller.current?.abort();
    },
    [],
  );
  async function connect() {
    setBusy(true);
    setError("");
    const request = new AbortController();
    controller.current = request;
    try {
      const s = await apiFetch<Session>("/api/app/sessions", {
        method: "POST",
        body: JSON.stringify({
          target_account_id: destination.account.id,
          credentials,
          jump_credentials: jump,
        }),
        signal: request.signal,
      });
      setCredentials(empty);
      setJump(empty);
      onConnected(s);
    } catch (e) {
      if (!request.signal.aborted) setError((e as Error).message);
    } finally {
      if (!request.signal.aborted) setBusy(false);
    }
  }
  const production = /prod|本番/i.test(destination.target.environment);
  return (
    <Dialog title={`${destination.target.name} に接続`} onClose={onClose}>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void connect();
        }}
      >
        <p>
          <strong>{destination.target.environment}</strong> ·{" "}
          {destination.account.ssh_username}@{destination.target.host}:
          {destination.target.port}
        </p>
        {destination.account.recording_enabled && (
          <p>サーバーの録画設定が有効な場合、端末の出力を記録します。</p>
        )}
        <CredentialFields
          auth={destination.account.auth_type}
          value={credentials}
          onChange={setCredentials}
          label="接続先の認証"
        />
        {destination.jump_account && (
          <CredentialFields
            auth={destination.jump_account.auth_type}
            value={jump}
            onChange={setJump}
            label={`踏み台 ${destination.jump_target?.name ?? ""} の認証`}
          />
        )}
        <p>入力した SSH 秘密鍵・パスワードは、この接続だけに使用します。</p>
        {production && (
          <Toggle
            label="本番環境への接続先を確認しました"
            checked={confirmed}
            onChange={setConfirmed}
          />
        )}
        <ErrorMessage message={error} />
        <button disabled={busy || (production && !confirmed)}>
          {busy ? "接続中…" : "SSH 接続を開始"}
        </button>
      </form>
    </Dialog>
  );
}
export function Workspace({
  uid,
  page,
  navigate,
  sharedId,
  suspended = false,
}: {
  uid: string;
  page: string;
  navigate: (path: string) => void;
  sharedId?: string;
  suspended?: boolean;
}) {
  const destinations = useData<Destination[]>("/api/app/targets", []);
  const sessions = useData<Session[]>("/api/app/sessions", []);
  const recent = useData<{ items: Log[] }>("/api/app/logs", { items: [] });
  const prefs = useData<Preferences>("/api/app/preferences", {
    theme: "tokyo-night",
    font_size: 14,
    favorites: [],
  });
  const [stored] = useState(() => readWorkspace(uid));
  const [tabs, setTabs] = useState<SessionTab[]>([]);
  const [active, setActive] = useState<string | null>(null);
  const [restored, setRestored] = useState(false);
  const [query, setQuery] = useState("");
  const [error, setError] = useState("");
  const [connecting, setConnecting] = useState<Destination | null>(null);
  const [ending, setEnding] = useState<Session | null>(null);
  const [busy, setBusy] = useState(false);
  const orderedIds = useMemo(() => tabs.map((t) => t.id), [tabs]);
  const layout = useSplitLayout(
    orderedIds,
    active,
    stored,
    page === "workspace" && !suspended,
  );
  useEffect(() => {
    if (suspended) return;
    sessions.refresh();
    destinations.refresh();
    prefs.refresh();
    const interval = setInterval(sessions.refresh, 10000);
    return () => clearInterval(interval);
  }, [suspended]); // GET polling never extends login inactivity.
  useEffect(() => {
    if (
      sessions.loading ||
      destinations.loading ||
      sessions.error ||
      destinations.error ||
      restored
    )
      return;
    let disposed = false;
    void Promise.all(
      stored.ids.map(async (item) => {
        if (!item.share) {
          const s = sessions.data.find((s) => s.id === item.id);
          return s
            ? tab(
                s,
                destinations.data.find(
                  (d) => d.account.id === s.target_account_id,
                ),
              )
            : null;
        }
        try {
          const v = await apiFetch<SharedSession>(
            `/api/app/shared/${encodeURIComponent(item.share)}`,
          );
          return {
            ...tab(v.session, undefined, item.share),
            shareCreator: v.creator_name,
            shareExpiresAt: v.share.expires_at,
          };
        } catch {
          return null;
        }
      }),
    ).then((values) => {
      if (disposed) return;
      const valid = values.filter((v): v is SessionTab => v !== null);
      setTabs(valid);
      setActive(
        valid.some((t) => t.id === stored.active)
          ? stored.active
          : (valid[0]?.id ?? null),
      );
      setRestored(true);
    });
    return () => {
      disposed = true;
    };
  }, [
    sessions.loading,
    destinations.loading,
    sessions.error,
    destinations.error,
    restored,
    stored,
    sessions.data,
    destinations.data,
  ]);
  useEffect(() => {
    if (!restored) return;
    const saved: StoredWorkspace = {
      ids: tabs.map((t) => ({ id: t.id, share: t.shareToken })),
      active,
      type: layout.layoutType,
      paneIds: layout.paneTabIds,
      ratioV: layout.splitRatioV,
      ratioH: layout.splitRatioH,
    };
    try {
      localStorage.setItem(`conduit:workspace:${uid}`, JSON.stringify(saved));
    } catch {
      /* storage may be disabled */
    }
  }, [
    uid,
    restored,
    tabs,
    active,
    layout.layoutType,
    layout.paneTabIds,
    layout.splitRatioV,
    layout.splitRatioH,
  ]);
  const openTab = (value: SessionTab) => {
    setTabs((tabs) =>
      tabs.some((t) => t.id === value.id) ? tabs : [...tabs, value],
    );
    setActive(value.id);
    layout.fillEmptyPane(value.id);
    navigate("/app/workspace");
  };
  useEffect(() => {
    if (!sharedId || !restored) return;
    const controller = new AbortController();
    apiFetch<SharedSession>(`/api/app/shared/${encodeURIComponent(sharedId)}`, {
      signal: controller.signal,
    })
      .then((v) => {
        const value = {
          ...tab(v.session, undefined, sharedId),
          shareCreator: v.creator_name,
          shareExpiresAt: v.share.expires_at,
        };
        setTabs((tabs) =>
          tabs.some((t) => t.id === value.id) ? tabs : [...tabs, value],
        );
        setActive(value.id);
      })
      .catch((e) => {
        if (!controller.signal.aborted) setError((e as Error).message);
      });
    return () => controller.abort();
  }, [sharedId, restored]);
  useEffect(() => {
    setTabs((tabs) =>
      tabs.map((t) => {
        const s = sessions.data.find((s) => s.id === t.sessionToken);
        return s ? { ...t, expiresAt: s.expires_at } : t;
      }),
    );
  }, [sessions.data]);
  const selectTab = (id: string) => {
    setActive(id);
    layout.selectPaneTab(id);
  };
  const closeTab = (id: string) => {
    setTabs((tabs) => tabs.filter((t) => t.id !== id));
    setActive((current) =>
      current === id ? (tabs.find((t) => t.id !== id)?.id ?? null) : current,
    );
    layout.releasePane(id);
    sessions.refresh();
  };
  async function favorite(id: string) {
    const favorites = prefs.data.favorites.includes(id)
      ? prefs.data.favorites.filter((value) => value !== id)
      : [...prefs.data.favorites, id];
    try {
      const p = await mutate<Preferences>(
        "/api/app/preferences",
        { ...prefs.data, favorites },
        "PATCH",
      );
      prefs.setData(p);
    } catch (e) {
      setError((e as Error).message);
    }
  }
  const filtered = destinations.data
    .filter((d) =>
      `${d.target.name} ${d.target.host} ${d.target.environment} ${d.account.ssh_username}`
        .toLowerCase()
        .includes(query.toLowerCase()),
    )
    .sort(
      (a, b) =>
        Number(prefs.data.favorites.includes(b.account.id)) -
        Number(prefs.data.favorites.includes(a.account.id)),
    );
  const terminalVisible = page === "workspace" || !!sharedId;
  return (
    <>
      <div
        className="portal-workspace"
        style={{ display: terminalVisible ? "flex" : "none" }}
      >
        <TabBar
          tabs={tabs}
          activeId={active}
          onSelect={selectTab}
          onClose={closeTab}
          onNew={() => navigate("/app")}
          layoutType={layout.layoutType}
          paneTabIds={layout.paneTabIds}
          onLayoutChange={layout.switchLayout}
          onReorder={(from, to) =>
            setTabs((tabs) => {
              const values = [...tabs];
              const a = values.findIndex((t) => t.id === from),
                b = values.findIndex((t) => t.id === to);
              if (a < 0 || b < 0) return tabs;
              values.splice(b, 0, ...values.splice(a, 1));
              return values;
            })
          }
        />
        {tabs.length === 0 && (
          <div className="portal-empty">
            <h2>端末を開きましょう</h2>
            <button onClick={() => navigate("/app")}>接続先を選ぶ</button>
          </div>
        )}
        <ErrorMessage message={error} />
        <TerminalPool
          tabs={tabs}
          visible={terminalVisible && !suspended}
          layoutType={layout.layoutType}
          paneTabIds={layout.paneTabIds}
          activeTabId={active}
          splitRatioV={layout.splitRatioV}
          splitRatioH={layout.splitRatioH}
          onCloseTab={closeTab}
          onDividerVMouseDown={layout.onDividerVMouseDown}
          onDividerHMouseDown={layout.onDividerHMouseDown}
          onResetRatioV={layout.resetRatioV}
          onResetRatioH={layout.resetRatioH}
        />
      </div>
      {page === "home" && !sharedId && (
        <section>
          <h1>接続先</h1>
          <p>
            許可された SSH
            アカウントから接続します。接続先が表示されない場合は管理者に権限を依頼してください。
          </p>
          <ErrorMessage message={error || destinations.error || prefs.error} />
          <Field label="接続先を検索">
            <input
              type="search"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="接続先名・環境・ホスト・SSH ユーザー名"
            />
          </Field>
          {destinations.loading ? (
            <p>読み込み中…</p>
          ) : filtered.length === 0 ? (
            <div className="portal-empty">
              {query
                ? "検索条件に一致する接続先がありません。"
                : "接続できる接続先がまだありません。管理者に接続権限を依頼してください。"}
            </div>
          ) : (
            <div className="portal-grid">
              {filtered.map((d) => (
                <article className="portal-card" key={d.account.id}>
                  <div className="portal-row">
                    <span
                      className={`portal-badge ${/prod|本番/i.test(d.target.environment) ? "production" : ""}`}
                    >
                      {d.target.environment || "環境未設定"}
                    </span>
                    <button
                      aria-label={`${d.target.name} をお気に入り${prefs.data.favorites.includes(d.account.id) ? "から外す" : "に追加"}`}
                      onClick={() => void favorite(d.account.id)}
                    >
                      {prefs.data.favorites.includes(d.account.id) ? "★" : "☆"}
                    </button>
                  </div>
                  <h2>{d.target.name}</h2>
                  <p>
                    {d.account.ssh_username}@{d.target.host}:{d.target.port}
                  </p>
                  <p>
                    {d.account.auth_type}
                    {d.jump_target && ` · ${d.jump_target.name} 経由`}
                  </p>
                  <button onClick={() => setConnecting(d)}>接続</button>
                </article>
              ))}
            </div>
          )}
          <h2>最近の接続</h2>
          {recent.data.items.slice(0, 5).map((log) => (
            <div className="portal-row" key={log.id}>
              <span>
                {log.user}@{log.host} · {date(log.connected_at)}
                {log.error && ` · ${log.error}`}
              </span>
            </div>
          ))}
          <h2>保持中のセッション</h2>
          {sessions.data.length === 0 ? (
            <p>保持中のセッションはありません。</p>
          ) : (
            sessions.data.map((s) => (
              <div className="portal-row" key={s.id}>
                <span>
                  {s.user}@{s.host} ·{" "}
                  {s.state === "connected"
                    ? "接続中"
                    : `再接続期限 ${new Date(s.expires_at).toLocaleString()}`}
                </span>
                <button
                  onClick={() =>
                    openTab(
                      tab(
                        s,
                        destinations.data.find(
                          (d) => d.account.id === s.target_account_id,
                        ),
                      ),
                    )
                  }
                >
                  開く
                </button>
              </div>
            ))
          )}
        </section>
      )}
      {page === "sessions" && (
        <section>
          <h1>自分のセッション</h1>
          <p>
            タブを閉じても SSH は猶予期間中保持されます。「終了」は SSH
            プロセスを終了します。
          </p>
          <ErrorMessage message={sessions.error || error} />
          <button onClick={sessions.refresh}>更新</button>
          {sessions.data.map((s) => (
            <article className="portal-card" key={s.id}>
              <h2>
                {s.user}@{s.host}:{s.port}
              </h2>
              <p>
                {s.state} · 閲覧者 {s.viewer_count} 名
                {s.recording_enabled && " · 録画中"}
              </p>
              {s.state !== "connected" && (
                <p>再接続期限: {new Date(s.expires_at).toLocaleString()}</p>
              )}
              <button
                onClick={() =>
                  openTab(
                    tab(
                      s,
                      destinations.data.find(
                        (d) => d.account.id === s.target_account_id,
                      ),
                    ),
                  )
                }
              >
                端末を開く
              </button>
              <button onClick={() => setEnding(s)}>終了</button>
            </article>
          ))}
        </section>
      )}
      {page === "preferences" && (
        <section>
          <h1>表示設定</h1>
          <p>このユーザーの設定として保存します。端末に反映します。</p>
          <ErrorMessage message={error || prefs.error} />
          <form
            className="portal-card"
            onSubmit={(e) => {
              e.preventDefault();
              setBusy(true);
              void mutate<Preferences>(
                "/api/app/preferences",
                prefs.data,
                "PATCH",
              )
                .then((value) => {
                  prefs.setData(value);
                  window.dispatchEvent(
                    new CustomEvent("conduit:preferences", { detail: value }),
                  );
                })
                .catch((e) => setError((e as Error).message))
                .finally(() => setBusy(false));
            }}
          >
            <Field label="テーマ">
              <select
                value={prefs.data.theme}
                onChange={(e) =>
                  prefs.setData({ ...prefs.data, theme: e.target.value })
                }
              >
                {Object.entries(themes).map(([key, value]) => (
                  <option key={key} value={key}>
                    {value.name}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="文字サイズ">
              <input
                type="number"
                min={8}
                max={32}
                value={prefs.data.font_size}
                onChange={(e) =>
                  prefs.setData({
                    ...prefs.data,
                    font_size: Number(e.target.value),
                  })
                }
              />
            </Field>
            <button disabled={busy}>保存</button>
          </form>
          <div className="portal-card">
            <h2>旧ブラウザデータの消去</h2>
            <p>
              以前の鍵・接続トークン・プロファイルは新しい接続では使用しません。必要な接続先を管理者に登録してもらった後、旧データを消去できます。
            </p>
            <button onClick={clearLegacyData}>旧データを消去</button>
          </div>
        </section>
      )}
      {connecting && (
        <ConnectDialog
          destination={connecting}
          onClose={() => setConnecting(null)}
          onConnected={(s) => {
            setConnecting(null);
            sessions.refresh();
            recent.refresh();
            openTab(tab(s, connecting));
          }}
        />
      )}
      {ending && (
        <Dialog title="SSH セッションを終了" onClose={() => setEnding(null)}>
          <p>
            {ending.user}@{ending.host} の実行中プロセスを終了します。
          </p>
          <ErrorMessage message={error} />
          <button
            disabled={busy}
            onClick={() => {
              setBusy(true);
              void mutate(`/api/app/sessions/${ending.id}`, {}, "DELETE")
                .then(() => {
                  sessions.refresh();
                  setEnding(null);
                })
                .catch((e) => setError((e as Error).message))
                .finally(() => setBusy(false));
            }}
          >
            終了する
          </button>
        </Dialog>
      )}
    </>
  );
}
