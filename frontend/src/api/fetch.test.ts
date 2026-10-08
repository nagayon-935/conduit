import { afterEach, expect, it, vi } from "vitest";
import { apiFetch } from "./fetch";
afterEach(() => vi.unstubAllGlobals());
it("uses cookies and disables caching while preserving server error codes", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValue(
      new Response(
        JSON.stringify({ error: "session ended", code: "SESSION_ENDED" }),
        { status: 410 },
      ),
    );
  vi.stubGlobal("fetch", fetch);
  await expect(apiFetch("/api/app/sessions/test")).rejects.toMatchObject({
    status: 410,
    code: "SESSION_ENDED",
  });
  expect(fetch).toHaveBeenCalledWith(
    "/api/app/sessions/test",
    expect.objectContaining({ cache: "no-store", credentials: "same-origin" }),
  );
});
it("fetches a fresh CSRF token and forwards cancellation for a mutation", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(
      new Response(JSON.stringify({ csrf_token: "fresh-csrf" })),
    )
    .mockResolvedValueOnce(new Response(null, { status: 204 }));
  vi.stubGlobal("fetch", fetch);
  const controller = new AbortController();
  await apiFetch("/api/auth/logout", {
    method: "POST",
    body: "{}",
    signal: controller.signal,
  });
  expect(fetch.mock.calls[0]).toEqual([
    "/api/auth/csrf",
    expect.objectContaining({
      credentials: "same-origin",
      signal: controller.signal,
    }),
  ]);
  const options = fetch.mock.calls[1][1] as RequestInit;
  expect((options.headers as Headers).get("X-CSRF-Token")).toBe("fresh-csrf");
  expect(options.signal).toBe(controller.signal);
});
