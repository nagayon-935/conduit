import { themes } from '../themes';
import type { ConnectionState } from '../types';
import { formatReconnectDeadline } from '../utils/format';

interface Props {
  host: string; user: string; port: number; name?: string; tag?: string;
  state: ConnectionState; reason: string; expiresAt: string; readOnly: boolean;
  currentThemeKey: string; onThemeChange: (key: string) => void;
  onClose: () => void; onEnd: () => void; onReconnect: () => void; onNew: () => void;
  onShare: () => void; onSearch: () => void; onCopy: () => void; onPaste: () => void; onHelp: () => void;
}
const STATE_LABELS: Record<ConnectionState, string> = { connecting: '接続中', connected: '接続済み', reconnecting: '再接続中', disconnected: '切断中', ended: '終了済み' };
export function TerminalStatusBar(props: Props) {
  return <div className="terminal-status-bar ux-terminal-status">
    <div className="status-left"><span className={`ux-state ux-state-${props.state}`} role="status">{props.readOnly && props.state === 'connected' ? '閲覧中' : STATE_LABELS[props.state]}</span>
      {props.tag && <span className="ux-tag">{props.tag}</span>}
      {props.name && <strong>{props.name}</strong>}<span className="status-session">{props.user}@{props.host}:{props.port}</span>
      {props.state !== 'connected' && props.state !== 'ended' && props.expiresAt && <small>再接続期限 {formatReconnectDeadline(props.expiresAt)}</small>}
    </div>
    <div className="status-right">
      <button onClick={props.onSearch} title="Ctrl+Shift+F / ⌘F">検索</button>
      <button onClick={props.onCopy}>コピー</button>
      {!props.readOnly && <button onClick={props.onPaste} disabled={props.state !== 'connected'}>貼り付け</button>}
      <select aria-label="端末テーマ" className="theme-select" value={props.currentThemeKey} onChange={(e) => props.onThemeChange(e.target.value)}>{Object.entries(themes).map(([key, theme]) => <option key={key} value={key}>{theme.name}</option>)}</select>
      {props.state === 'disconnected' && <button onClick={props.onReconnect}>再接続</button>}
      {props.state === 'ended' && !props.readOnly && <button onClick={props.onNew}>新しく接続</button>}
      {!props.readOnly && props.state === 'connected' && <button onClick={props.onShare}>閲覧共有</button>}
      {!props.readOnly && props.state !== 'ended' && <button className="ux-danger" onClick={props.onEnd}>SSH を終了</button>}
      <button onClick={props.onHelp} aria-label="キーボード操作">?</button>
      <button onClick={props.onClose}>{props.readOnly ? '閲覧を閉じる' : 'タブを閉じる'}</button>
    </div>
  </div>;
}
