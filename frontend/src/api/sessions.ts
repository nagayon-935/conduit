import type { SessionInfo, ShareResponse, ShareLink } from '../types';
import { apiFetch } from './fetch';

function ownerHeaders(token: string) { return { Authorization: `Bearer ${token}` }; }
export function fetchOwnSession(token: string, signal?: AbortSignal): Promise<SessionInfo> {
  return apiFetch('/api/session', { headers: ownerHeaders(token), signal });
}
export function endOwnSession(token: string): Promise<void> {
  return apiFetch('/api/session', { method: 'DELETE', headers: ownerHeaders(token) });
}
export function fetchShares(token: string, signal?: AbortSignal): Promise<ShareLink[]> {
  return apiFetch('/api/session/shares', { headers: ownerHeaders(token), signal });
}
export function fetchSessions(adminToken = ''): Promise<SessionInfo[]> {
  return apiFetch('/api/sessions', { headers: ownerHeaders(adminToken) });
}
export function killSession(id: string, adminToken = ''): Promise<void> {
  return apiFetch(`/api/sessions/${encodeURIComponent(id)}`, { method: 'DELETE', headers: ownerHeaders(adminToken) });
}
export function shareSession(sessionToken: string, ttlSeconds = 3600): Promise<ShareResponse> {
  return apiFetch(`/api/sessions/${encodeURIComponent(sessionToken)}/share`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ ttl_seconds: ttlSeconds }),
  });
}
export function revokeShare(sessionToken: string, shareToken: string): Promise<void> {
  return apiFetch(`/api/sessions/${encodeURIComponent(sessionToken)}/share/${encodeURIComponent(shareToken)}`, { method: 'DELETE' });
}

export function fetchSharedSession(shareToken: string, signal?: AbortSignal): Promise<SessionInfo> {
  return apiFetch<SessionInfo>(`/api/shared-session/${encodeURIComponent(shareToken)}`, { signal });
}
