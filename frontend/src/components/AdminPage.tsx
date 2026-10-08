import { useState } from 'react';
import { fetchSessions } from '../api/sessions';
import { SessionList } from './SessionList';
import { LogPage } from './LogPage';

export function AdminPage() {
  const [input, setInput] = useState('');
  const [token, setToken] = useState<string | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [view, setView] = useState<'sessions' | 'logs'>('sessions');
  if (token === null) return <main className="ux-admin-login"><h1>Conduit 管理者ページ</h1><p>管理者用トークンでセッション一覧・終了操作・接続ログを利用できます。</p>
    <form onSubmit={async (e) => { e.preventDefault(); setBusy(true); setError('');
      try { await fetchSessions(input); setToken(input); setInput(''); } catch (err) { setError(err instanceof Error ? err.message : '認証に失敗しました。'); } finally { setBusy(false); }
    }}><label>管理者トークン<input type="password" autoComplete="off" value={input} onChange={(e) => setInput(e.target.value)} /></label><button className="ux-primary" disabled={busy}>{busy ? '確認中…' : '管理画面を開く'}</button></form>
    <p className="ux-muted">管理者トークンはこのページのメモリにのみ保持します。ラボ構成でトークン未設定の場合は空欄で開けます。</p>
    {error && <p role="alert">{error}</p>}<a href="/app">ユーザーの作業画面へ</a>
  </main>;
  return <div className="ux-workspace ux-admin"><header className="ux-topbar"><strong>Conduit 管理</strong><button aria-pressed={view === 'sessions'} onClick={() => setView('sessions')}>全セッション</button><button aria-pressed={view === 'logs'} onClick={() => setView('logs')}>接続ログ・録画</button><a href="/app" target="_blank" rel="noreferrer">ユーザー画面</a><button onClick={() => setToken(null)}>管理画面を閉じる</button></header>
    <div className="ux-admin-body">{view === 'sessions' ? <SessionList adminToken={token} onBack={() => setToken(null)} /> : <LogPage adminToken={token} onBack={() => setView('sessions')} />}</div>
  </div>;
}
