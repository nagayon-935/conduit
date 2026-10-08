import { useState, useCallback, useEffect } from 'react';
import { ConnectForm } from './components/ConnectForm';
import { TabBar } from './components/TabBar';
import { TerminalPool } from './components/TerminalPool';
import { NewConnectionOverlay } from './components/NewConnectionOverlay';
import { AdminPage } from './components/AdminPage';
import { Dialog } from './components/Dialog';
import type { ConnectResponse, SessionTab } from './types';
import { useConnectionHistory } from './hooks/useConnectionHistory';
import { useProfiles } from './hooks/useProfiles';
import { useTabs } from './hooks/useTabs';
import { useSplitLayout } from './hooks/useSplitLayout';
import { useConnectionJobs } from './hooks/useConnectionJobs';
import { endOwnSession, fetchOwnSession } from './api/sessions';
import { ApiRequestError } from './api/fetch';
import { defaultFields, fieldsFromProfile, type FormFields } from './utils/form';
import './App.css';
import './Workspace.css';

function Workspace() {
  const { history, addEntry } = useConnectionHistory();
  const profiles = useProfiles();
  const { tabs, activeTabId, selectTab, addTab, removeTab, pauseTab, resumeTab, reorderTabs, updateTab } = useTabs();
  const visibleTabs = tabs.filter((tab) => !tab.paused);
  const [home, setHome] = useState(false);
  const [overlay, setOverlay] = useState<FormFields[] | null>(null);
  const [closing, setClosing] = useState<{ id: string; endOnly: boolean } | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const showHome = home || !visibleTabs.length;
  const layout = useSplitLayout(visibleTabs.map((tab) => tab.id), activeTabId, !showHome && !overlay && !closing);

  const connected = useCallback((response: ConnectResponse, fields: FormFields) => {
    const profile = profiles.profiles.find((p) => p.host === fields.host.trim() && p.port === Number(fields.port) && p.user === fields.user.trim());
    const tab: SessionTab = { id: crypto.randomUUID(), sessionToken: response.session_token,
      host: fields.host.trim(), port: Number(fields.port), user: fields.user.trim(), expiresAt: response.expires_at,
      authType: fields.authType, name: profile?.name, tag: profile?.tag, color: profile?.color,
      jumpHost: fields.jumpHost, jumpPort: Number(fields.jumpPort), jumpUser: fields.jumpUser, jumpAuthType: fields.jumpAuthType,
      privateKeyName: fields.privateKeyName, jumpPrivateKeyName: fields.jumpPrivateKeyName };
    if (profile && profile.authType !== fields.authType) profiles.updateProfile(profile.id, { authType: fields.authType });
    addTab(tab); addEntry(tab.host, tab.port, tab.user, fields.authType); layout.fillEmptyPane(tab.id); setHome(false);
  }, [profiles.profiles, profiles.updateProfile, addTab, addEntry, layout.fillEmptyPane]);
  const jobs = useConnectionJobs(connected);
  useEffect(() => { if (!showHome && activeTabId) layout.showTab(activeTabId); }, [showHome, activeTabId, layout.showTab]);
  const chooseTab = useCallback((id: string) => { layout.showTab(id); selectTab(id); setHome(false); }, [layout.showTab, selectTab]);
  function newFromTab(id: string) {
    const tab = tabs.find((entry) => entry.id === id); if (!tab) return;
    const profile = profiles.profiles.find((p) => p.host === tab.host && p.port === tab.port && p.user === tab.user);
    setOverlay([{ ...(profile ? fieldsFromProfile(profile) : defaultFields()), host: tab.host, port: String(tab.port), user: tab.user,
      authType: tab.authType ?? profile?.authType ?? 'vault', jumpHost: tab.jumpHost ?? '', jumpPort: String(tab.jumpPort || 22), jumpUser: tab.jumpUser ?? '', jumpAuthType: tab.jumpAuthType ?? 'vault' }]);
  }
  function closeTab(id: string) {
    const tab = tabs.find((entry) => entry.id === id);
    if (tab?.ended || tab?.shareToken) { layout.releasePane(id); removeTab(id); }
    else { setError(''); setClosing({ id, endOnly: false }); }
  }
  async function endTab() {
    const tab = tabs.find((entry) => entry.id === closing?.id); if (!tab) return;
    setBusy(true); setError('');
    try {
      try { await endOwnSession(tab.sessionToken); } catch (err) { if (!(err instanceof ApiRequestError && err.status === 410)) throw err; }
      updateTab(tab.id, { ended: true, endReason: '利用者が SSH セッションを終了しました。' }); setClosing(null);
    } catch (err) { setError(err instanceof Error ? err.message : 'セッションを終了できませんでした。'); }
    finally { setBusy(false); }
  }
  async function resume(id: string) {
    const tab = tabs.find((entry) => entry.id === id); if (!tab) return;
    setError('');
    if (tab.sessionToken && !tab.ended) {
      try { const info = await fetchOwnSession(tab.sessionToken); updateTab(id, { expiresAt: info.expires_at }); }
      catch (err) {
        if (err instanceof ApiRequestError && err.status === 410) updateTab(id, { ended: true, endReason: 'セッションが終了したか、再接続期限を過ぎています。' });
        else { setError(err instanceof Error ? err.message : '接続状態を確認できませんでした。'); return; }
      }
    }
    resumeTab(id); layout.fillEmptyPane(id); setHome(false);
  }
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (document.querySelector('[role="dialog"]')) return;
      if (event.target instanceof HTMLElement && event.target.closest('input, textarea, select, [role="dialog"]') && !event.target.closest('.xterm')) return;
      if (showHome || overlay || closing || !event.altKey || !['ArrowLeft', 'ArrowRight'].includes(event.key)) return;
      event.preventDefault();
      const index = visibleTabs.findIndex((tab) => tab.id === activeTabId);
      const next = visibleTabs[(index + (event.key === 'ArrowRight' ? 1 : -1) + visibleTabs.length) % visibleTabs.length];
      if (next) chooseTab(next.id);
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [showHome, overlay, closing, visibleTabs, activeTabId, chooseTab]);

  const target = tabs.find((tab) => tab.id === closing?.id);
  return <div className="ux-workspace">
    <header className="ux-topbar"><strong>Conduit</strong><span className="ux-muted">Web SSH Terminal</span>
      <button aria-pressed={showHome} onClick={() => setHome(true)}>接続先・再開</button>
      {!!visibleTabs.length && <button aria-pressed={!showHome} onClick={() => setHome(false)}>端末 ({visibleTabs.length})</button>}
      <button onClick={() => setOverlay([defaultFields()])}>＋ 新しい接続</button>
      <a className="ux-admin-link" href="/admin" target="_blank" rel="noreferrer">管理者ページ ↗</a>
    </header>
    {jobs.jobs.length > 0 && <section className="ux-jobs" aria-label="ホストごとの接続結果" aria-live="polite">
      {jobs.jobs.map((job) => <div className={`ux-job ux-job-${job.state}`} key={job.id}>
        <strong>{job.user}@{job.host}:{job.port}</strong><span>{({ connecting: '接続中…', connected: '接続済み', failed: '失敗', cancelled: 'キャンセル済み' })[job.state]}</span>
        {job.error && <span className="ux-job-error">{job.error}</span>}
        {job.state === 'failed' && <><button onClick={() => jobs.retry(job.id)}>このホストだけ再試行</button><button onClick={() => { const fields = jobs.edit(job.id); if (fields) setOverlay([fields]); }}>設定を修正</button></>}
        {job.state === 'connecting' ? <button onClick={() => jobs.cancel(job.id)}>キャンセル</button> : <button aria-label={`${job.host}の接続結果を閉じる`} onClick={() => jobs.dismiss(job.id)}>×</button>}
      </div>)}
    </section>}
    {error && !closing && <div className="ux-global-error" role="alert">{error}<button onClick={() => setError('')}>閉じる</button></div>}
    {showHome && <div className="ux-home-scroll">
      {!!tabs.length && <section className="ux-resume"><h2>作業を再開</h2><div className="ux-session-cards">{tabs.map((tab) => <article key={tab.id} style={{ borderLeftColor: tab.color || '#7aa2f7' }}>
        <strong>{tab.tag && `[${tab.tag}] `}{tab.name || tab.host}</strong><span>{tab.user}@{tab.host}:{tab.port}</span>
        <small>{tab.ended ? '終了済み · 前回の出力を確認できます' : tab.paused ? '接続を残して閉じています' : '端末を開いています'}</small>
        {tab.paused && !tab.ended && <small>再接続期限（目安）: {new Date(tab.expiresAt).toLocaleString()}</small>}
        <div className="ux-row"><button onClick={() => { void resume(tab.id); }}>{tab.ended ? '出力を見る' : '作業に戻る'}</button>{!tab.shareToken && <button onClick={() => newFromTab(tab.id)}>新しく接続</button>}{tab.paused && !tab.ended && <button onClick={() => setClosing({ id: tab.id, endOnly: true })}>SSH を終了</button>}{tab.ended && <button onClick={() => { layout.releasePane(tab.id); removeTab(tab.id); }}>一覧から削除</button>}</div>
      </article>)}</div></section>}
      <ConnectForm store={profiles} history={history} onSubmit={jobs.connectEntries} />
    </div>}
    <div className="ux-terminal-workspace" style={{ display: showHome ? 'none' : 'flex' }}>
      <TabBar tabs={visibleTabs} activeId={activeTabId} onSelect={chooseTab} onClose={closeTab} onNew={() => setOverlay([defaultFields()])}
        layoutType={layout.layoutType} paneTabIds={layout.paneTabIds} onLayoutChange={layout.switchLayout} profiles={profiles.profiles} onReorder={reorderTabs} />
      <TerminalPool tabs={tabs} layoutType={layout.layoutType} paneTabIds={layout.paneTabIds} activeTabId={activeTabId}
        splitRatioV={layout.splitRatioV} splitRatioH={layout.splitRatioH} visible={!showHome} interactive={!showHome && !overlay && !closing}
        onSelectTab={selectTab} onCloseTab={closeTab} onEndTab={(id) => { setError(''); setClosing({ id, endOnly: true }); }} onNewFromTab={newFromTab} onUpdateTab={updateTab}
        onDividerVMouseDown={layout.onDividerVMouseDown} onDividerHMouseDown={layout.onDividerHMouseDown} onResetRatioV={layout.resetRatioV} onResetRatioH={layout.resetRatioH} />
    </div>
    {overlay && <NewConnectionOverlay store={profiles} history={history} initialFields={overlay} onClose={() => setOverlay(null)} onSubmit={(entries) => { jobs.connectEntries(entries); setOverlay(null); }} />}
    {closing && target && <Dialog title={closing.endOnly ? 'SSH セッションを終了' : 'タブを閉じる'} onClose={() => { if (!busy) { setClosing(null); setError(''); } }}>
      <p><strong>{target.user}@{target.host}:{target.port}</strong></p>
      {!closing.endOnly && <><p>接続を残して閉じると、再接続猶予の間は「作業を再開」から戻れます。</p><button disabled={busy} onClick={() => { pauseTab(target.id); layout.releasePane(target.id); setClosing(null); }}>接続を残して閉じる</button></>}
      <p>SSH を終了すると、接続先とのセッションを切断します。端末の出力は終了後も確認できます。</p>
      <button className="ux-danger" disabled={busy} onClick={() => { void endTab(); }}>{busy ? '終了中…' : 'SSH セッションを終了する'}</button>
      {error && <p role="alert">{error}</p>}
    </Dialog>}
  </div>;
}
export default function App() {
  return (/^\/admin(?:\/|$)/).test(window.location.pathname) ? <AdminPage /> : <Workspace />;
}
