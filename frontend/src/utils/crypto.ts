const SESSION_KEY_NAME = 'conduit_ck';

let pendingKey: Promise<CryptoKey> | null = null;
async function getOrCreateKey(): Promise<CryptoKey> {
  // Parallel profile encryption must use the same newly generated key.
  if (!pendingKey) pendingKey = createKey().finally(() => { pendingKey = null; });
  return pendingKey;
}
async function createKey(): Promise<CryptoKey> {
  const stored = sessionStorage.getItem(SESSION_KEY_NAME);
  if (stored) {
    try {
      const raw = Uint8Array.from(atob(stored), (c) => c.charCodeAt(0));
      return await crypto.subtle.importKey('raw', raw, { name: 'AES-GCM' }, false, ['encrypt', 'decrypt']);
    } catch {
      // fall through to generate a new key
    }
  }
  const key = await crypto.subtle.generateKey({ name: 'AES-GCM', length: 256 }, true, ['encrypt', 'decrypt']);
  const raw = await crypto.subtle.exportKey('raw', key);
  sessionStorage.setItem(SESSION_KEY_NAME, btoa(String.fromCharCode(...new Uint8Array(raw))));
  return key;
}

export async function encryptText(text: string): Promise<string> {
  const key = await getOrCreateKey();
  const iv = crypto.getRandomValues(new Uint8Array(12));
  const encoded = new TextEncoder().encode(text);
  const cipher = await crypto.subtle.encrypt({ name: 'AES-GCM', iv }, key, encoded);
  // Concatenate iv + cipher, then base64-encode
  const combined = new Uint8Array(iv.byteLength + cipher.byteLength);
  combined.set(iv, 0);
  combined.set(new Uint8Array(cipher), iv.byteLength);
  return btoa(String.fromCharCode(...combined));
}

export async function decryptText(cipherB64: string): Promise<string | null> {
  try {
    const key = await getOrCreateKey();
    const combined = Uint8Array.from(atob(cipherB64), (c) => c.charCodeAt(0));
    const iv = combined.slice(0, 12);
    const cipher = combined.slice(12);
    const plain = await crypto.subtle.decrypt({ name: 'AES-GCM', iv }, key, cipher);
    return new TextDecoder().decode(plain);
  } catch {
    return null;
  }
}
