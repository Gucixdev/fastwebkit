// @vitest-environment node
//
// Forced to the node environment rather than jsdom: Web Crypto (SubtleCrypto) doesn't need a
// DOM, and testing it against Node's real implementation (not a jsdom shim) is the honest way
// to exercise it - this is exactly the "verify real changes for real" rule applied to crypto.
import { describe, it, expect } from "vitest";
import { generateKeyPair, exportPublicKey, importPublicKey, deriveSharedKey, encryptMessage, decryptMessage } from "./chat_crypto";

describe("ECDH + AES-GCM end-to-end flow", () => {
  it("two independently generated keypairs derive the same shared key", async () => {
    const alice = await generateKeyPair();
    const bob = await generateKeyPair();

    // Simulate exchanging public keys over the wire as JWK.
    const aliceJwk = await exportPublicKey(alice.publicKey);
    const bobJwk = await exportPublicKey(bob.publicKey);
    const bobPublicForAlice = await importPublicKey(bobJwk);
    const alicePublicForBob = await importPublicKey(aliceJwk);

    const aliceSharedKey = await deriveSharedKey(alice.privateKey, bobPublicForAlice);
    const bobSharedKey = await deriveSharedKey(bob.privateKey, alicePublicForBob);

    // Prove it by using one side's key to encrypt and the other's to decrypt.
    const payload = await encryptMessage(aliceSharedKey, "hello bob, this is end-to-end encrypted");
    const plaintext = await decryptMessage(bobSharedKey, payload);
    expect(plaintext).toBe("hello bob, this is end-to-end encrypted");
  });

  it("a third party's keypair cannot decrypt the message", async () => {
    const alice = await generateKeyPair();
    const bob = await generateKeyPair();
    const eve = await generateKeyPair();

    const bobPublicForAlice = await importPublicKey(await exportPublicKey(bob.publicKey));
    const alicePublicForEve = await importPublicKey(await exportPublicKey(alice.publicKey));

    const aliceSharedKey = await deriveSharedKey(alice.privateKey, bobPublicForAlice);
    const eveWrongKey = await deriveSharedKey(eve.privateKey, alicePublicForEve);

    const payload = await encryptMessage(aliceSharedKey, "secret");
    await expect(decryptMessage(eveWrongKey, payload)).rejects.toThrow();
  });

  it("produces a different ciphertext (and IV) for the same plaintext each time", async () => {
    const alice = await generateKeyPair();
    const bob = await generateKeyPair();
    const bobPublicForAlice = await importPublicKey(await exportPublicKey(bob.publicKey));
    const key = await deriveSharedKey(alice.privateKey, bobPublicForAlice);

    const first = await encryptMessage(key, "same message");
    const second = await encryptMessage(key, "same message");

    expect(first.iv).not.toBe(second.iv);
    expect(first.ciphertext).not.toBe(second.ciphertext);
  });

  it("rejects a tampered ciphertext instead of returning corrupted plaintext", async () => {
    const alice = await generateKeyPair();
    const bob = await generateKeyPair();
    const bobPublicForAlice = await importPublicKey(await exportPublicKey(bob.publicKey));
    const alicePublicForBob = await importPublicKey(await exportPublicKey(alice.publicKey));
    const aliceSharedKey = await deriveSharedKey(alice.privateKey, bobPublicForAlice);
    const bobSharedKey = await deriveSharedKey(bob.privateKey, alicePublicForBob);

    const payload = await encryptMessage(aliceSharedKey, "untampered");
    const tampered = { ...payload, ciphertext: payload.ciphertext.slice(0, -4) + "AAAA" };

    await expect(decryptMessage(bobSharedKey, tampered)).rejects.toThrow();
  });
});
