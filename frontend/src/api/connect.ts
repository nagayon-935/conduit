import type { ConnectRequest, ConnectResponse } from '../types';
import { apiFetch } from './fetch';
import { CONNECT_TIMEOUT_MS } from '../constants';

export async function connectToHost(req: ConnectRequest, signal?: AbortSignal): Promise<ConnectResponse> {
  const controller = new AbortController();
  const abort = () => controller.abort();
  signal?.addEventListener('abort', abort, { once: true });
  if (signal?.aborted) controller.abort();
  const timeoutId = setTimeout(() => controller.abort(), CONNECT_TIMEOUT_MS);

  try {
    return await apiFetch<ConnectResponse>('/api/connect', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
      signal: controller.signal,
    });
  } catch (err) {
    if (err instanceof Error && err.name === 'AbortError') {
      if (signal?.aborted) throw new Error('接続をキャンセルしました。');
      throw new Error(`Connection timed out after ${CONNECT_TIMEOUT_MS / 1000} seconds`);
    }
    throw err;
  } finally {
    clearTimeout(timeoutId);
    signal?.removeEventListener('abort', abort);
  }
}
