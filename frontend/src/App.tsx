import { useEffect, useRef, useState, type FormEvent } from "react";
import { apiFetch, mutate } from "./api/fetch";
import type { Identity } from "./portalTypes";
import { Admin, adminPages } from "./portal/Admin";
import { Workspace } from "./portal/Workspace";
import { Logs } from "./portal/Logs";
import { Dialog, ErrorMessage, Field, Toggle } from "./portal/common";
import { TerminalPreferencesProvider } from "./portal/TerminalPreferences";
import "./App.css";
import "./portal/Portal.css";

function Login({
  onLoggedIn,
  expired = false,
}: {
  onLoggedIn: (value: Identity) => void;
  expired?: boolean;
}) {
  const [login, setLogin] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const value = await mutate<Identity>("/api/auth/login", {
        login,
        password,
      });
      setPassword("");
      onLoggedIn(value);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <main className="portal-login">
      <div className="portal-login-brand">
        Conduit<span>Browser SSH workspace</span>
      </div>
      <h1>{expired ? "ログインし直してください" : "ログイン"}</h1>
      <p>
        {expired
          ? "端末の出力はこの画面に保持しています。保持中の SSH セッションには再接続できます。"
          : "Conduit アカウントでログインすると、許可された接続先を利用できます。"}
      </p>
      <form onSubmit={(e) => void submit(e)}>
        <Field label="ログイン名">
          <input
            autoFocus
            required
            autoComplete="username"
            value={login}
            onChange={(e) => setLogin(e.target.value)}
          />
        </Field>
        <Field label="パスワード">
          <input
            type="password"
            required
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </Field>
        <ErrorMessage message={error} />
        <button disabled={busy}>{busy ? "ログイン中…" : "ログイン"}</button>
      </form>
      <p className="portal-muted">
        アカウントがない場合は管理者にお問い合わせください。
      </p>
    </main>
  );
}
function Password({
  onSaved,
  forced = false,
}: {
  onSaved: (value: Identity) => void;
  forced?: boolean;
}) {
  const [current, setCurrent] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  return (
    <section>
      <h1>
        {forced ? "最初にパスワードを変更してください" : "パスワードを変更"}
      </h1>
      <p>
        12
        文字以上で設定してください。変更すると、他のログインと共有は取り消されます。
      </p>
      <form
        className="portal-card"
        onSubmit={(e) => {
          e.preventDefault();
          if (password !== confirm) {
            setError("新しいパスワードが一致しません");
            return;
          }
          setBusy(true);
          void mutate<Identity>("/api/auth/password", {
            current_password: current,
            password,
          })
            .then((value) => {
              setPassword("");
              setCurrent("");
              setConfirm("");
              onSaved(value);
            })
            .catch((e) => setError((e as Error).message))
            .finally(() => setBusy(false));
        }}
      >
        <Field label="現在のパスワード">
          <input
            type="password"
            required
            autoComplete="current-password"
            value={current}
            onChange={(e) => setCurrent(e.target.value)}
          />
        </Field>
        <Field label="新しいパスワード">
          <input
            type="password"
            required
            minLength={12}
            autoComplete="new-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </Field>
        <Field label="新しいパスワード（確認）">
          <input
            type="password"
            required
            minLength={12}
            autoComplete="new-password"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
          />
        </Field>
        <ErrorMessage message={error} />
        <button disabled={busy}>パスワードを変更</button>
      </form>
    </section>
  );
}
function Shell({
  identity,
  path,
  navigate,
  onLogout,
  onPassword,
  paused,
}: {
  identity: Identity;
  path: string;
  navigate: (path: string) => void;
  onLogout: () => void;
  onPassword: (identity: Identity) => void;
  paused: boolean;
}) {
  const admin = path === "/admin" || path.startsWith("/admin/");
  const adminMenu = admin && identity.user.role === "admin";
  const adminPage =
    path.split("/")[2] === "access" ? "grants" : path.split("/")[2] || "health";
  const sharedId = path.startsWith("/app/shared/")
    ? path.split("/")[3]
    : undefined;
  const rawPage = admin ? "" : path.split("/")[2] || "home";
  const page =
    path === "/account/password"
      ? "password"
      : ((
          { history: "logs", settings: "preferences" } as Record<string, string>
        )[rawPage] ?? rawPage);
  const [logout, setLogout] = useState(false);
  const [terminate, setTerminate] = useState(false);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  useEffect(() => {
    const handler = (e: Event) => setNotice((e as CustomEvent<string>).detail);
    window.addEventListener("conduit:notice", handler);
    return () => window.removeEventListener("conduit:notice", handler);
  }, []);
  const expiry = Math.min(
    identity.expires_at,
    identity.idle_expires_at ?? identity.expires_at,
  );
  const warning = expiry * 1000 - Date.now() < 5 * 60000;
  const appPages = {
    home: "接続先",
    workspace: "端末",
    sessions: "自分のセッション",
    logs: "接続履歴",
    preferences: "表示設定",
    password: "パスワード",
  };
  return (
    <div className="portal-shell">
      <header className="portal-header">
        <button className="portal-brand" onClick={() => navigate("/app")}>
          Conduit
        </button>
        <div className="portal-mode">
          <button aria-pressed={!adminMenu} onClick={() => navigate("/app")}>
            ユーザー画面
          </button>
          {identity.user.role === "admin" && (
            <button aria-pressed={admin} onClick={() => navigate("/admin")}>
              管理者画面
            </button>
          )}
        </div>
        <span>{identity.user.display_name}</span>
        <button onClick={() => setLogout(true)}>ログアウト</button>
      </header>
      <div className="portal-body">
        <nav aria-label={adminMenu ? "管理者メニュー" : "ユーザーメニュー"}>
          <span className="portal-nav-title">
            {adminMenu ? "管理・運用" : "SSH ワークスペース"}
          </span>
          {Object.entries(adminMenu ? adminPages : appPages).map(
            ([key, label]) => (
              <button
                key={key}
                aria-current={
                  (adminMenu ? adminPage : page) === key ? "page" : undefined
                }
                onClick={() =>
                  navigate(
                    adminMenu
                      ? `/admin/${key}`
                      : key === "home"
                        ? "/app"
                        : `/app/${key}`,
                  )
                }
              >
                {label}
              </button>
            ),
          )}
        </nav>
        <main
          className={`portal-content ${page === "workspace" || sharedId ? "is-terminal" : ""}`}
        >
          {warning && (
            <p className="portal-warning" role="status">
              ログインの有効期限が近づいています。端末入力や操作がない状態では、自動的にログアウトします。
            </p>
          )}
          <ErrorMessage message={notice} />
          {identity.user.must_change_password ? (
            <Password forced onSaved={onPassword} />
          ) : (
            <TerminalPreferencesProvider>
              {identity.user.role === "admin" && page === "home" && (
                <div className="portal-card">
                  <h2>管理者のセットアップ</h2>
                  <p>
                    接続先と SSH
                    アカウントを登録し、利用者に権限を割り当てます。自分の接続にも権限を設定してください。
                  </p>
                  <button onClick={() => navigate("/admin/targets")}>
                    接続先を登録
                  </button>
                  <button onClick={() => navigate("/admin/grants")}>
                    権限を設定
                  </button>
                </div>
              )}
              <Workspace
                uid={identity.user.id}
                suspended={paused}
                page={page}
                navigate={navigate}
                sharedId={sharedId}
              />
              {page === "logs" && <Logs admin={false} />}
              {page === "password" && <Password onSaved={onPassword} />}
              {admin &&
                (identity.user.role === "admin" ? (
                  <Admin key={adminPage} page={adminPage} />
                ) : (
                  <section>
                    <h1>管理者権限が必要です</h1>
                    <button onClick={() => navigate("/app")}>
                      ユーザー画面に戻る
                    </button>
                  </section>
                ))}
            </TerminalPreferencesProvider>
          )}
        </main>
      </div>
      {logout && (
        <Dialog title="ログアウト" onClose={() => setLogout(false)}>
          <p>
            このログインの共有リンクと端末接続を閉じます。SSH セッションは最大{" "}
            {identity.reconnect_grace_minutes ?? 15} 分間保持されます。
          </p>
          <Toggle
            label="自分の SSH セッションをすべて終了する"
            checked={terminate}
            onChange={setTerminate}
          />
          <ErrorMessage message={notice} />
          <button
            disabled={busy}
            onClick={() => {
              setBusy(true);
              void mutate("/api/auth/logout", { terminate_sessions: terminate })
                .then(() => {
                  try {
                    localStorage.removeItem(
                      `conduit:workspace:${identity.user.id}`,
                    );
                  } catch {
                    /* disabled storage */
                  }
                  onLogout();
                })
                .catch((e) => setNotice((e as Error).message))
                .finally(() => setBusy(false));
            }}
          >
            ログアウトする
          </button>
        </Dialog>
      )}
    </div>
  );
}
export default function App() {
  const [identity, setIdentity] = useState<Identity | null>(null);
  const [loading, setLoading] = useState(true);
  const [expired, setExpired] = useState(false);
  const [path, setPath] = useState(location.pathname);
  const [error, setError] = useState("");
  const shellRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (shellRef.current) shellRef.current.inert = expired;
  }, [expired, identity?.user.id]);
  function navigate(value: string) {
    history.pushState(null, "", value);
    setPath(value);
  }
  useEffect(() => {
    const pop = () => setPath(location.pathname);
    const reauth = () => setExpired(true);
    window.addEventListener("popstate", pop);
    window.addEventListener("conduit:reauthenticate", reauth);
    apiFetch<Identity>("/api/auth/me")
      .then((value) => {
        setIdentity(value);
        setExpired(false);
        if (
          !location.pathname.startsWith("/app") &&
          !location.pathname.startsWith("/admin")
        )
          navigate("/app");
      })
      .catch((e) => {
        if ((e as { status?: number }).status !== 401)
          setError((e as Error).message);
      })
      .finally(() => setLoading(false));
    return () => {
      window.removeEventListener("popstate", pop);
      window.removeEventListener("conduit:reauthenticate", reauth);
    };
  }, []);
  useEffect(() => {
    if (!identity || expired) return;
    const controller = new AbortController();
    const timer = setInterval(() => {
      apiFetch<Identity>("/api/auth/me", { signal: controller.signal })
        .then((value) => {
          if (!controller.signal.aborted) setIdentity(value);
        })
        .catch(() => {});
    }, 30000);
    return () => {
      controller.abort();
      clearInterval(timer);
    };
  }, [identity?.user.id, expired]);
  function loggedIn(value: Identity) {
    if (identity && identity.user.id !== value.user.id) {
      try {
        localStorage.removeItem(`conduit:workspace:${identity.user.id}`);
      } catch {
        /* disabled storage */
      }
    }
    setIdentity(value);
    setExpired(false);
    if (!path.startsWith("/app/shared/")) navigate("/app");
    window.dispatchEvent(new Event("conduit:authenticated"));
  }
  if (loading)
    return (
      <main className="portal-login">
        <p>Conduit を読み込んでいます…</p>
      </main>
    );
  return (
    <>
      <ErrorMessage message={error} />
      {identity ? (
        <div
          ref={shellRef}
          className="portal-shell-container"
          aria-hidden={expired || undefined}
        >
          <Shell
            key={identity.user.id}
            identity={identity}
            paused={expired}
            path={path}
            navigate={navigate}
            onPassword={loggedIn}
            onLogout={() => {
              setIdentity(null);
              setExpired(false);
              navigate("/login");
            }}
          />
        </div>
      ) : (
        <Login onLoggedIn={loggedIn} />
      )}
      {identity && expired && (
        <div className="portal-backdrop">
          <Login expired onLoggedIn={loggedIn} />
        </div>
      )}
    </>
  );
}
