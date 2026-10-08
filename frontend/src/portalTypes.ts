export interface User {
  id: string;
  login: string;
  display_name: string;
  role: "admin" | "user";
  enabled: boolean;
  must_change_password: boolean;
}
export interface Identity {
  user: User;
  csrf_token: string;
  expires_at: number;
  idle_expires_at?: number;
  reconnect_grace_minutes?: number;
}
export interface Target {
  id: string;
  name: string;
  host: string;
  port: number;
  environment: string;
  enabled: boolean;
  jump_account_id?: string;
}
export interface Account {
  id: string;
  target_id: string;
  ssh_username: string;
  auth_type: "vault" | "password" | "pubkey";
  enabled: boolean;
  sharing_enabled: boolean;
  recording_enabled: boolean;
}
export interface Destination {
  target: Target;
  account: Account;
  jump_account?: Account;
  jump_target?: Target;
}
export interface Session {
  id: string;
  owner_user_id: string;
  target_account_id: string;
  host: string;
  port: number;
  user: string;
  state: string;
  created_at: string;
  expires_at: string;
  viewer_count: number;
  ws_count: number;
  recording_enabled: boolean;
}
export interface Grant {
  user_id: string;
  target_account_id: string;
  can_connect: boolean;
  can_view_shared: boolean;
}
export interface Share {
  id: string;
  session_id: string;
  recipient_user_ids: string[];
  expires_at: number;
}
export interface Preferences {
  theme: string;
  font_size: number;
  favorites: string[];
}
export interface Log {
  id: string;
  owner_user_id?: string;
  host: string;
  port: number;
  user: string;
  connected_at: number;
  disconnected_at?: number;
  error?: string;
  termination_reason?: string;
  has_recording: boolean;
}

export interface SharedSession {
  session: Session;
  share: Share;
  creator_name: string;
}
