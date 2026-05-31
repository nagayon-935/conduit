export type AuthType = 'vault' | 'password' | 'pubkey';

export interface ConnectRequest {
  host: string;
  port: number;
  user: string;
  auth_type: AuthType;
  password?: string;
  private_key?: string;
  // ProxyJump (omit or set jump_host='' to disable)
  jump_host?: string;
  jump_port?: number;
  jump_user?: string;
  jump_auth_type?: AuthType;
  jump_password?: string;
  jump_private_key?: string;
}

export interface ConnectResponse {
  session_token: string;
  expires_at: string;
  message: string;
}

export interface ApiError {
  error: string;
  code: string;
}

// WebSocket control message envelope
export type WsControlMessage =
  | { type: 'ping' }
  | { type: 'pong' }
  | { type: 'resize'; cols: number; rows: number }
  | { type: 'error'; message: string }
  | { type: 'exit' };

export type AppState = 'idle' | 'connecting';

/** Terminal pane layout: single / side-by-side / top-bottom / 2×2 grid */
export type LayoutType = '1' | '2v' | '2h' | '4';

// ── Frontend-only domain types ───────────────────────────────────────────

export interface Profile {
  id: string;
  name: string;
  host: string;
  port: number;
  user: string;
  authType: AuthType;
  createdAt: string;
  /** 保存済み秘密鍵の PEM 内容 */
  privateKeyContent?: string;
  /** 保存済み秘密鍵のファイル名（表示用） */
  privateKeyName?: string;
  jumpHost?: string;
  jumpPort?: number;
  jumpUser?: string;
  jumpAuthType?: AuthType;
  jumpPrivateKeyContent?: string;
  jumpPrivateKeyName?: string;
}

export interface HistoryEntry {
  host: string;
  port: number;
  user: string;
  authType: AuthType;
  connectedAt: string;
}

export interface StoredSession {
  token: string;
  expiresAt: string;
  host: string;
  port: number;
  user: string;
}

export interface SessionInfo {
  token: string;
  host: string;
  port: number;
  user: string;
  state: 'connected' | 'disconnected' | 'terminated';
  created_at: string;
  expires_at: string;
  ws_count: number;
  viewer_count: number;
}

export interface ShareResponse {
  share_token: string;
  url: string;
  expires_at: string;
}
