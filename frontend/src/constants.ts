// ── LocalStorage keys ────────────────────────────────────────────────────
export const STORAGE_KEYS = {
  SESSION:   'conduit-session',
  HISTORY:   'conduit-history',
  PROFILES:  'conduit-profiles',
  FONT_SIZE: 'conduit-fontSize',
  THEME:     'conduit-theme',
} as const;

// ── Limits ───────────────────────────────────────────────────────────────
export const MAX_PROFILES       = 20;
export const MAX_HISTORY_ENTRIES = 10;

// ── Terminal ─────────────────────────────────────────────────────────────
export const FONT_SIZE_MIN     = 8;
export const FONT_SIZE_MAX     = 32;
export const FONT_SIZE_DEFAULT = 14;

// ── Terminal search ──────────────────────────────────────────────────────
export const SEARCH_HISTORY_KEY = 'conduit:search-history';
export const MAX_SEARCH_HISTORY = 5;

// ── Network ──────────────────────────────────────────────────────────────
export const CONNECT_TIMEOUT_MS       = 15_000;
export const HEARTBEAT_INTERVAL_MS    = 30_000;
export const RECONNECT_BASE_DELAY_MS  = 2_000;
export const MAX_RECONNECT_ATTEMPTS   = 5;

// ── ANSI terminal messages ───────────────────────────────────────────────
export const ANSI = {
  RECONNECTED:     '\r\n\x1b[32m[Conduit] Reconnected.\x1b[0m',
  SESSION_ENDED:   '\r\n\x1b[90m[Conduit] Session ended.\x1b[0m',
  RECONNECTING:    '\r\n\x1b[33m[Conduit] Reconnecting...\x1b[0m',
  CONNECTION_LOST:  '\r\n\x1b[31m[Conduit] Connection lost. Max reconnect attempts reached.\x1b[0m',
} as const;
