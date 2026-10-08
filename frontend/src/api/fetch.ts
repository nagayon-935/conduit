import type { ApiError } from '../types';

/**
 * Thin wrapper around fetch that:
 * - Throws an Error with the server's error message on non-ok responses
 * - Supports an optional AbortSignal
 */
export class ApiRequestError extends Error {
  constructor(message: string, public status: number, public code?: string) { super(message); }
}

export async function apiFetch<T>(
  url: string,
  init?: RequestInit,
): Promise<T> {
  const response = await fetch(url, { cache: 'no-store', ...init });

  if (!response.ok) {
    let message = `HTTP ${response.status}: ${response.statusText}`;
    let code: string | undefined;
    try {
      const body: ApiError = await response.json();
      if (body.error) message = body.error;
      code = body.code;
    } catch {
      // keep the HTTP status message
    }
    throw new ApiRequestError(message, response.status, code);
  }

  // 204 No Content — return undefined cast as T
  if (response.status === 204) return undefined as T;

  return response.json() as Promise<T>;
}
