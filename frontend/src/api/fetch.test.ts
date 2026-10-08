import { afterEach, expect, it, vi } from 'vitest';
import { apiFetch } from './fetch';
afterEach(() => vi.unstubAllGlobals());
it('does not reuse status responses across session capabilities and preserves the server error code', async () => {
  const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: 'session ended', code: 'SESSION_ENDED' }), { status: 410 }));
  vi.stubGlobal('fetch', fetch);
  await expect(apiFetch('/api/session', { headers: { Authorization: 'Bearer owner' } })).rejects.toMatchObject({ status: 410, code: 'SESSION_ENDED' });
  expect(fetch).toHaveBeenCalledWith('/api/session', { cache: 'no-store', headers: { Authorization: 'Bearer owner' } });
});
