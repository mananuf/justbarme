import { describe, expect, it } from 'vitest';

import { base64Encode, base64UrlEncode } from './base64';
import { checkLease, type LeasePayload } from './lease';

async function issueTestLease(payload: LeasePayload, privateKey: CryptoKey): Promise<string> {
  const payloadBytes = new TextEncoder().encode(JSON.stringify(payload));
  const signature = new Uint8Array(
    await crypto.subtle.sign({ name: 'Ed25519' }, privateKey, payloadBytes),
  );
  return ['v1', base64UrlEncode(payloadBytes), base64UrlEncode(signature)].join('.');
}

function samplePayload(overrides: Partial<LeasePayload> = {}): LeasePayload {
  return {
    business_id: '01993f8b-f814-73c8-aa41-1fa86026743f',
    user_id: '01a0a59c-1ae8-7119-beed-565a3371016f',
    device_id: '01a0a59d-019b-7718-990e-95e71c9597c8',
    location_id: '01a0a59c-1b52-70de-84b1-bdaf405c0229',
    role: 'owner',
    capabilities: ['sales:record', 'bills:read'],
    issued_at: new Date().toISOString(),
    expires_at: new Date(Date.now() + 7 * 24 * 60 * 60 * 1000).toISOString(),
    ...overrides,
  };
}

describe('checkLease', () => {
  it('verifies a genuine, unexpired lease', async () => {
    const { publicKey, privateKey } = await crypto.subtle.generateKey({ name: 'Ed25519' }, true, [
      'sign',
      'verify',
    ]);
    const publicKeyB64 = base64Encode(
      new Uint8Array(await crypto.subtle.exportKey('raw', publicKey)),
    );

    const token = await issueTestLease(samplePayload(), privateKey);
    const result = await checkLease(token, publicKeyB64);

    expect(result.signatureVerified).toBe(true);
    expect(result.expired).toBe(false);
    expect(result.payload?.role).toBe('owner');
    expect(result.payload?.capabilities).toEqual(['sales:record', 'bills:read']);
  });

  it('reports an expired lease even though the signature is genuine', async () => {
    const { publicKey, privateKey } = await crypto.subtle.generateKey({ name: 'Ed25519' }, true, [
      'sign',
      'verify',
    ]);
    const publicKeyB64 = base64Encode(
      new Uint8Array(await crypto.subtle.exportKey('raw', publicKey)),
    );

    const token = await issueTestLease(
      samplePayload({ expires_at: new Date(Date.now() - 1000).toISOString() }),
      privateKey,
    );
    const result = await checkLease(token, publicKeyB64);

    expect(result.signatureVerified).toBe(true);
    expect(result.expired).toBe(true);
  });

  it('rejects a lease signed by a different key', async () => {
    const real = await crypto.subtle.generateKey({ name: 'Ed25519' }, true, ['sign', 'verify']);
    const attacker = await crypto.subtle.generateKey({ name: 'Ed25519' }, true, ['sign', 'verify']);
    const realPublicKeyB64 = base64Encode(
      new Uint8Array(await crypto.subtle.exportKey('raw', real.publicKey)),
    );

    const token = await issueTestLease(samplePayload(), attacker.privateKey);
    const result = await checkLease(token, realPublicKeyB64);

    expect(result.signatureVerified).toBe(false);
  });

  it('rejects a tampered payload', async () => {
    const { publicKey, privateKey } = await crypto.subtle.generateKey({ name: 'Ed25519' }, true, [
      'sign',
      'verify',
    ]);
    const publicKeyB64 = base64Encode(
      new Uint8Array(await crypto.subtle.exportKey('raw', publicKey)),
    );

    const token = await issueTestLease(samplePayload({ role: 'staff' }), privateKey);
    const [version, payloadPart, sigPart] = token.split('.');
    const tamperedPayload = base64UrlEncode(
      new TextEncoder().encode(JSON.stringify(samplePayload({ role: 'owner' }))),
    );
    const tampered = [version, tamperedPayload, sigPart].join('.');

    const result = await checkLease(tampered, publicKeyB64);
    expect(result.signatureVerified).toBe(false);
    void payloadPart;
  });

  it('reports malformed tokens without throwing', async () => {
    for (const bad of ['', 'not-a-lease', 'v1.onlyonepart', 'v2.aa.bb', 'v1..']) {
      const result = await checkLease(bad, 'irrelevant');
      expect(result.signatureVerified).toBe(false);
    }
  });
});
