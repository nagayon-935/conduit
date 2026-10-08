import type { ApiError } from "../types";

export class ApiFailure extends Error {
  constructor(
    message: string,
    public status: number,
    public code: string = "",
  ) {
    super(message);
  }
}

export async function apiFetch<T>(
  url: string,
  init: RequestInit = {},
): Promise<T> {
  const headers = new Headers(init.headers);
  if (init.method && !["GET", "HEAD"].includes(init.method.toUpperCase())) {
    const csrf = await fetch("/api/auth/csrf", {
      credentials: "same-origin",
      cache: "no-store",
      signal: init.signal,
    });
    if (!csrf.ok)
      throw new ApiFailure(
        "ログイン画面を更新してください",
        csrf.status,
        "CSRF_FAILED",
      );
    const body = (await csrf.json()) as { csrf_token: string };
    headers.set("X-CSRF-Token", body.csrf_token);
    headers.set("Content-Type", "application/json");
  }
  const response = await fetch(url, {
    ...init,
    headers,
    credentials: "same-origin",
    cache: "no-store",
  });
  if (!response.ok) {
    let body: ApiError = { error: `HTTP ${response.status}`, code: "" };
    try {
      body = (await response.json()) as ApiError;
    } catch {
      /* use HTTP status */
    }
    if (response.status === 401 && url !== "/api/auth/login")
      window.dispatchEvent(new Event("conduit:reauthenticate"));
    throw new ApiFailure(body.error, response.status, body.code);
  }
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}

export function mutate<T>(
  url: string,
  body: unknown = {},
  method = "POST",
  signal?: AbortSignal,
): Promise<T> {
  return apiFetch<T>(url, { method, body: JSON.stringify(body), signal });
}

// Legacy components share the same structured error until they are migrated.
export { ApiFailure as ApiRequestError };
