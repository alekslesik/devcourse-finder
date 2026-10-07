// Analytics must never change the outcome of a successful catalog request.
function eventId(): string | undefined {
  const crypto = globalThis.crypto;
  if (typeof crypto?.randomUUID === 'function') return crypto.randomUUID();
  if (typeof crypto?.getRandomValues !== 'function') return;
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  bytes[6] = (bytes[6] & 15) | 64;
  bytes[8] = (bytes[8] & 63) | 128;
  const hex = Array.from(bytes, byte => byte.toString(16).padStart(2, '0')).join('');
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

export function recordEvent(kind: string, extra: Record<string, unknown> = {}): void {
  try {
    const id = eventId();
    if (!id) return;
    void fetch('/api/v1/events', {
      method: 'POST', headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({id, kind, ...extra}), keepalive: true,
    }).catch(() => {});
  } catch {
    // Unsupported browser APIs and synchronous transmission errors are optional.
  }
}
