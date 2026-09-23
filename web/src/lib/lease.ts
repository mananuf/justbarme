import { base64Decode, base64UrlDecode } from './base64';

// Mirrors internal/auth.LeasePayload on the backend exactly -- field names
// and JSON shape must match byte-for-byte, since the signature covers the
// raw payload bytes the server marshaled.
export interface LeasePayload {
  business_id: string;
  user_id: string;
  device_id: string;
  location_id: string;
  role: string;
  capabilities: string[];
  issued_at: string;
  expires_at: string;
}

export interface LeaseCheckResult {
  payload: LeasePayload | null;
  signatureVerified: boolean;
  expired: boolean;
  error?: string;
}

const LEASE_FORMAT_VERSION = 'v1';

// checkLease decodes and verifies a "v1.<payload>.<signature>" lease token
// entirely client-side, using the server's Ed25519 public key embedded at
// build time (VITE_OFFLINE_SIGNING_PUBLIC_KEY). This lets the app know
// whether cached offline access is genuine and unexpired without a network
// round trip -- see docs/ARCHITECTURE.md §11.2.
//
// If this browser cannot verify Ed25519 signatures (older WebCrypto), that
// is reported via `error`, not treated as invalid: the authoritative check
// always happens server-side when the device eventually syncs.
export async function checkLease(token: string, publicKeyB64: string): Promise<LeaseCheckResult> {
  const parts = token.split('.');
  if (parts.length !== 3 || parts[0] !== LEASE_FORMAT_VERSION) {
    return { payload: null, signatureVerified: false, expired: true, error: 'malformed lease' };
  }

  let payloadBytes: Uint8Array<ArrayBuffer>;
  let signatureBytes: Uint8Array<ArrayBuffer>;
  try {
    payloadBytes = base64UrlDecode(parts[1]!);
    signatureBytes = base64UrlDecode(parts[2]!);
  } catch {
    return {
      payload: null,
      signatureVerified: false,
      expired: true,
      error: 'malformed lease encoding',
    };
  }

  let payload: LeasePayload;
  try {
    payload = JSON.parse(new TextDecoder().decode(payloadBytes)) as LeasePayload;
  } catch {
    return {
      payload: null,
      signatureVerified: false,
      expired: true,
      error: 'malformed lease payload',
    };
  }

  const expired =
    Number.isNaN(Date.parse(payload.expires_at)) || Date.parse(payload.expires_at) <= Date.now();

  try {
    const keyBytes = base64Decode(publicKeyB64);
    const key = await crypto.subtle.importKey('raw', keyBytes, { name: 'Ed25519' }, false, [
      'verify',
    ]);
    const signatureVerified = await crypto.subtle.verify(
      { name: 'Ed25519' },
      key,
      signatureBytes,
      payloadBytes,
    );
    return { payload, signatureVerified, expired };
  } catch {
    return {
      payload,
      signatureVerified: false,
      expired,
      error: 'signature could not be verified on this device',
    };
  }
}
