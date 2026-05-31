import type { SessionInfo, ShareResponse } from '../types';
import { apiFetch } from './fetch';

export function fetchSessions(): Promise<SessionInfo[]> {
  return apiFetch<SessionInfo[]>('/api/sessions');
}

export function killSession(token: string): Promise<void> {
  return apiFetch<void>(`/api/sessions/${encodeURIComponent(token)}`, {
    method: 'DELETE',
  });
}

export function shareSession(sessionToken: string): Promise<ShareResponse> {
  return apiFetch<ShareResponse>(`/api/sessions/${encodeURIComponent(sessionToken)}/share`, {
    method: 'POST',
  });
}

export function revokeShare(sessionToken: string, shareToken: string): Promise<void> {
  return apiFetch<void>(
    `/api/sessions/${encodeURIComponent(sessionToken)}/share/${encodeURIComponent(shareToken)}`,
    { method: 'DELETE' },
  );
}
