/** Formats an ISO timestamp as a short local HH:MM string for the reconnect deadline. */
export function formatReconnectDeadline(expiresAt: string): string {
  try {
    const date = new Date(expiresAt);
    return date.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
  } catch {
    return expiresAt;
  }
}
