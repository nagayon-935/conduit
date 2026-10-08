import type { StoredSession } from '../types';
import { readJSON, writeJSON, removeKey } from './storage';
import { STORAGE_KEYS } from '../constants';

/** Save session info to localStorage */
export function saveSession(session: StoredSession): void {
  writeJSON(STORAGE_KEYS.SESSION, session);
}

/**
 * Load session info from localStorage.
 * Returns null when missing, expired, or unparseable.
 */
export function loadSession(): StoredSession | null {
  const session = readJSON<StoredSession | null>(STORAGE_KEYS.SESSION, null);
  if (!session) return null;

  // The server validates expiry; a connected SSH session may outlive this hint.
  return session;
}

/** Remove session info from localStorage */
export function clearSession(): void {
  removeKey(STORAGE_KEYS.SESSION);
}
