import { useEffect, useCallback, useRef, useState } from 'react';
import { useTerminal } from '../hooks/useTerminal';
import { useWebSocket } from '../hooks/useWebSocket';
import { useShareSession } from '../hooks/useShareSession';
import { useFontSizeShortcuts } from '../hooks/useFontSizeShortcuts';
import { useSearchController } from '../hooks/useSearchController';
import { TerminalStatusBar } from './TerminalStatusBar';
import { SearchOverlay } from './SearchOverlay';
import { Dialog } from './Dialog';
import type { SessionTab, SessionInfo } from '../types';
import { FONT_SIZE_DEFAULT } from '../constants';
import '@xterm/xterm/css/xterm.css';
import './Terminal.css';

export function Terminal({ tab, active, onSelect, onClose, onEnd, onNew, onUpdate }: {
  tab: SessionTab; active: boolean; onSelect: () => void; onClose: () => void; onEnd: () => void; onNew: () => void;
  onUpdate: (patch: Partial<SessionTab>) => void;
}) {
  const readOnly = !!tab.shareToken;
  const { terminalRef, terminal, fitAddon, initTerminal, disposeTerminal, changeFontSize, setTheme, currentThemeKey, search } = useTerminal();
  const [error, setError] = useState('');
  const [panel, setPanel] = useState<'share' | 'paste' | 'help' | null>(null);
  const [pasteText, setPasteText] = useState('');
  const latestUpdate = useRef(onUpdate); latestUpdate.current = onUpdate;
  const ended = useCallback((reason: string) => latestUpdate.current({ ended: true, endReason: reason }), []);
  const sessionInfo = useCallback((info: SessionInfo) => latestUpdate.current({ host: info.host, user: info.user, port: info.port, expiresAt: info.expires_at, gracePeriodSeconds: info.grace_period_seconds }), []);
  const ws = useWebSocket({ token: tab.sessionToken, shareToken: tab.shareToken, terminal, fitAddon,
    onDisconnect: () => {}, onError: setError, onEnded: ended, onSessionInfo: sessionInfo });
  const sharing = useShareSession(readOnly ? '' : tab.sessionToken);
  const focused = active && !panel;
  const focus = useCallback(() => terminal?.focus(), [terminal]);
  const getFontSize = useCallback(() => terminal?.options.fontSize ?? FONT_SIZE_DEFAULT, [terminal]);
  const fontSizeToast = useFontSizeShortcuts(changeFontSize, getFontSize, focused);
  const searchController = useSearchController(search, focused, focus);
  const restored = useRef(false);
  const state = tab.ended ? 'ended' : ws.state;
  const reason = tab.endReason || ws.reason;

  useEffect(() => { initTerminal(); return disposeTerminal; }, [initTerminal, disposeTerminal]);
  useEffect(() => {
    if (!terminal) return;
    const key = `conduit-output:${tab.id}`;
    if (!restored.current) {
      restored.current = true;
      try { const text = sessionStorage.getItem(key); if (text) terminal.write(`[Conduit] 前回の表示（最大500行）\r\n${text.replace(/\n/g, '\r\n')}\r\n`); } catch { /* optional scrollback */ }
    }
    let timer: ReturnType<typeof setTimeout> | undefined;
    const save = () => {
      try {
      const buffer = terminal.buffer.normal;
      const lines: string[] = [];
      for (let i = Math.max(0, buffer.length - 500); i < buffer.length; i++) lines.push(buffer.getLine(i)?.translateToString(true) ?? '');
      while (lines.length && !lines[lines.length - 1].trim()) lines.pop();
      sessionStorage.setItem(key, lines.join('\n').slice(-128000));
      } catch { /* storage unavailable or terminal disposed */ }
    };
    const listener = terminal.onWriteParsed(() => { if (!timer) timer = setTimeout(() => { timer = undefined; save(); }, 500); });
    window.addEventListener('pagehide', save);
    return () => { clearTimeout(timer); save(); listener.dispose(); window.removeEventListener('pagehide', save); };
  }, [terminal, tab.id]);
  useEffect(() => {
    if (terminal && !tab.paused && !tab.ended) ws.connect();
    return ws.disconnect;
  }, [terminal, tab.paused, tab.ended, ws.connect, ws.disconnect]);
  useEffect(() => { if (terminal) terminal.options.disableStdin = readOnly || state !== 'connected'; }, [terminal, readOnly, state]);
  useEffect(() => { if (focused && terminal && !searchController.open) terminal.focus(); }, [focused, terminal, searchController.open]);

  const copy = useCallback(async () => {
    const text = terminal?.getSelection();
    if (!text) { setError('コピーする文字を端末内で選択してください。'); return; }
    try { await navigator.clipboard.writeText(text); setError(''); } catch { setError('コピーできませんでした。ブラウザのコピー操作を使ってください。'); }
  }, [terminal]);
  const paste = useCallback(async () => {
    setPasteText(''); setPanel('paste');
    try { setPasteText(await navigator.clipboard.readText()); } catch { /* manual paste into the preview */ }
  }, []);
  useEffect(() => {
    if (!terminal) return;
    terminal.attachCustomKeyEventHandler((event) => {
      if (!focused || event.type !== 'keydown') return true;
      const key = event.key.toLowerCase();
      if ((event.metaKey || event.ctrlKey && event.shiftKey) && ['f', 'c', 'v'].includes(key)) {
        if (key === 'c') { event.preventDefault(); void copy(); }
        if (key === 'v' && !readOnly) { event.preventDefault(); void paste(); }
        return false; // search is handled by the scoped search hook
      }
      if ((event.ctrlKey || event.metaKey) && ['+', '=', '-'].includes(key)) return false;
      if (event.altKey && (['ArrowLeft', 'ArrowRight'].includes(event.key) || ['Digit1', 'Digit2', 'Digit3', 'Digit4'].includes(event.code))) return false;
      return true;
    });
    const guardPaste = (event: ClipboardEvent) => {
      const text = event.clipboardData?.getData('text/plain') ?? '';
      if (text.includes('\n') || text.includes('\r')) {
        event.preventDefault(); event.stopPropagation(); setPasteText(text); setPanel('paste');
      }
    };
    const element = terminalRef.current;
    if (!readOnly) element?.addEventListener('paste', guardPaste, true);
    return () => { terminal.attachCustomKeyEventHandler(() => true); element?.removeEventListener('paste', guardPaste, true); };
  }, [terminal, terminalRef, focused, copy, paste, readOnly]);

  return <div className={`terminal-wrapper ux-terminal${active ? ' ux-terminal-active' : ''}`} style={{ borderColor: tab.color || '#7aa2f7' }} onFocusCapture={onSelect} onPointerDown={onSelect}>
    {readOnly && <div className="readonly-banner">閲覧専用 — {tab.user}@{tab.host}（入力はできません）</div>}
    <TerminalStatusBar host={tab.host} user={tab.user} port={tab.port} name={tab.name} tag={tab.tag} state={state} reason={reason} expiresAt={ws.info?.expires_at || tab.expiresAt}
      readOnly={readOnly} currentThemeKey={currentThemeKey} onThemeChange={setTheme} onClose={onClose} onEnd={onEnd} onReconnect={ws.connect} onNew={onNew}
      onShare={() => setPanel('share')} onSearch={searchController.openSearch} onCopy={() => { void copy(); }} onPaste={() => { void paste(); }} onHelp={() => setPanel('help')} />
    {(reason || error) && <div className="ux-terminal-message" role="status">{error || reason}{error && <button onClick={() => setError('')}>閉じる</button>}</div>}
    <div className="terminal-container" ref={terminalRef} />
    {fontSizeToast !== null && active && <div className="font-size-toast">文字サイズ: {fontSizeToast}px</div>}
    {searchController.open && active && <SearchOverlay controller={searchController} />}
    {panel === 'share' && <Dialog title="端末を閲覧共有" onClose={() => setPanel(null)}>
      <p>このリンクを持つ人は端末の出力を閲覧できます。入力・リサイズはできません。</p>
      <p>閲覧者: {ws.info?.viewer_count ?? 0} 人 · 有効なリンク: {sharing.linkCount} 件</p>
      {!sharing.activeShareToken && <label>有効期間<select value={sharing.ttlSeconds} onChange={(e) => sharing.setTTLSeconds(Number(e.target.value))}><option value="900">15分</option><option value="3600">1時間</option><option value="14400">4時間</option></select></label>}
      <button className="ux-primary" disabled={sharing.busy || state !== 'connected'} onClick={() => { void sharing.share(); }}>{sharing.shareCopied ? 'コピーしました' : sharing.activeShareToken ? 'リンクをコピー' : 'リンクを作成してコピー'}</button>
      {sharing.shareURL && <label>共有リンク<input readOnly value={sharing.shareURL} onFocus={(e) => e.target.select()} /><small>有効期限: {sharing.expiresAt && new Date(sharing.expiresAt).toLocaleString()}</small></label>}
      {sharing.error && <p role="alert">{sharing.error}</p>}
      {sharing.activeShareToken && <button className="ux-danger" disabled={sharing.busy} onClick={() => { void sharing.revoke(); }}>すべての共有を停止（接続中の閲覧者も退出）</button>}
    </Dialog>}
    {panel === 'paste' && <Dialog title="貼り付け内容を確認" onClose={() => setPanel(null)}>
      <p>{tab.user}@{tab.host}:{tab.port} に送信します。改行を含む内容は、接続先によってコマンドとして実行される場合があります。</p>
      <textarea aria-label="貼り付け内容" rows={8} value={pasteText} onChange={(e) => setPasteText(e.target.value)} placeholder="ここに貼り付けてください" />
      <button className="ux-primary" disabled={!pasteText || state !== 'connected'} onClick={() => { terminal?.paste(pasteText); setPanel(null); }}>この端末に貼り付ける</button>
    </Dialog>}
    {panel === 'help' && <Dialog title="キーボード操作" onClose={() => setPanel(null)}>
      <dl><dt>端末内検索</dt><dd>Ctrl+Shift+F / ⌘F</dd><dt>コピー・貼り付け</dt><dd>Ctrl+Shift+C / V、macOS は ⌘C / V</dd><dt>文字サイズ</dt><dd>Ctrl / ⌘ と ＋・−</dd><dt>端末を切り替える</dt><dd>Alt+← / →</dd><dt>分割レイアウト</dt><dd>Alt+1 / 2 / 3 / 4</dd></dl>
      <p>操作は選択中の端末に適用されます。Ctrl+C は接続先へ送信し、実行中の処理を中断できます。</p>
    </Dialog>}
  </div>;
}
