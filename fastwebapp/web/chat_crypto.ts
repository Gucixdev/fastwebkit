// End-to-end encryption for chat messages: ECDH (P-256) key agreement derives a shared AES-GCM
// key between two participants, so anything relaying ciphertext between them (the server
// included) never has the shared key - only the two sides that generated a keypair can derive
// it. All native Web Crypto (SubtleCrypto) - no crypto library, because the browser already is
// one and reimplementing AES/ECDH by hand would be the one place "write it yourself" is a bad
// idea (constant-time guarantees, side-channel resistance - this is exactly what SubtleCrypto
// exists to get right so nobody else has to).
export interface KeyPair {
  publicKey: CryptoKey;
  privateKey: CryptoKey;
}

export async function generateKeyPair(): Promise<KeyPair> {
  const pair = await crypto.subtle.generateKey({ name: "ECDH", namedCurve: "P-256" }, true, ["deriveKey"]);
  return { publicKey: pair.publicKey, privateKey: pair.privateKey };
}

// JWK (JSON Web Key) is the exchange format: send this over whatever transport carries the
// chat's signaling (the server can see this - it's a public key, that's the point).
export async function exportPublicKey(key: CryptoKey): Promise<JsonWebKey> {
  return crypto.subtle.exportKey("jwk", key);
}

export async function importPublicKey(jwk: JsonWebKey): Promise<CryptoKey> {
  return crypto.subtle.importKey("jwk", jwk, { name: "ECDH", namedCurve: "P-256" }, true, []);
}

// Both sides call this with their own private key + the other's public key and land on the
// same AES-GCM key - ECDH's actual property, not something this code enforces itself.
export async function deriveSharedKey(privateKey: CryptoKey, peerPublicKey: CryptoKey): Promise<CryptoKey> {
  return crypto.subtle.deriveKey(
    { name: "ECDH", public: peerPublicKey },
    privateKey,
    { name: "AES-GCM", length: 256 },
    false,
    ["encrypt", "decrypt"],
  );
}

export interface EncryptedPayload {
  iv: string; // base64
  ciphertext: string; // base64, includes the GCM auth tag
}

function toBase64(buf: ArrayBuffer): string {
  return btoa(String.fromCharCode(...new Uint8Array(buf)));
}
// Built via `new Uint8Array(length)` + set(), not Uint8Array.from(), so TS sees a concrete
// ArrayBuffer-backed Uint8Array<ArrayBuffer> - the type SubtleCrypto's BufferSource needs,
// which Uint8Array.from()'s inferred Uint8Array<ArrayBufferLike> doesn't satisfy.
function fromBase64(b64: string): Uint8Array<ArrayBuffer> {
  const binary = atob(b64);
  const out = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) out[i] = binary.charCodeAt(i);
  return out;
}

// A fresh random IV every call - required for AES-GCM: reusing an IV with the same key breaks
// its confidentiality guarantee entirely, so this never takes one as a parameter.
export async function encryptMessage(key: CryptoKey, plaintext: string): Promise<EncryptedPayload> {
  const iv = crypto.getRandomValues(new Uint8Array(12));
  const data = new TextEncoder().encode(plaintext);
  const ciphertext = await crypto.subtle.encrypt({ name: "AES-GCM", iv }, key, data);
  return { iv: toBase64(iv.buffer), ciphertext: toBase64(ciphertext) };
}

export async function decryptMessage(key: CryptoKey, payload: EncryptedPayload): Promise<string> {
  const iv = fromBase64(payload.iv);
  const ciphertext = fromBase64(payload.ciphertext);
  const plaintext = await crypto.subtle.decrypt({ name: "AES-GCM", iv }, key, ciphertext);
  return new TextDecoder().decode(plaintext);
}
