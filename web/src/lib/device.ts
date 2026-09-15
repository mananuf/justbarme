import { base64Decode, base64Encode } from './base64';
import { db, type DeviceRecord } from './db';

export class DeviceCryptoUnsupportedError extends Error {
  constructor() {
    super('This browser cannot generate the Ed25519 keys justbarme needs for offline device enrollment.');
    this.name = 'DeviceCryptoUnsupportedError';
  }
}

function defaultDisplayName(): string {
  const ua = navigator.userAgent;
  if (/Android/i.test(ua)) return 'Android phone';
  if (/iPhone|iPad|iPod/i.test(ua)) return 'iPhone';
  return 'This device';
}

// getOrCreateDeviceRecord returns this browser's durable device keypair,
// generating and persisting one on first use. The private key never leaves
// this function except as an opaque exported blob stored in IndexedDB --
// nothing else in the app needs to touch it directly yet (client-side event
// signing is Phase 4+ sync work), but establishing one durable identity per
// device now avoids re-architecting device enrollment later.
export async function getOrCreateDeviceRecord(): Promise<DeviceRecord> {
  const existing = await db.device.get('current');
  if (existing) return existing;

  let keyPair: CryptoKeyPair;
  try {
    keyPair = await crypto.subtle.generateKey({ name: 'Ed25519' }, true, ['sign', 'verify']);
  } catch {
    throw new DeviceCryptoUnsupportedError();
  }

  const publicKeyRaw = await crypto.subtle.exportKey('raw', keyPair.publicKey);
  const privateKeyPkcs8 = await crypto.subtle.exportKey('pkcs8', keyPair.privateKey);

  const record: DeviceRecord = {
    id: 'current',
    deviceId: null,
    publicKeyB64: base64Encode(new Uint8Array(publicKeyRaw)),
    privateKeyPkcs8B64: base64Encode(new Uint8Array(privateKeyPkcs8)),
    displayName: defaultDisplayName(),
    lease: null,
    enrolledAt: null,
  };
  await db.device.put(record);
  return record;
}

export async function saveEnrollment(deviceId: string, lease: string): Promise<void> {
  await db.device.update('current', { deviceId, lease, enrolledAt: new Date().toISOString() });
}

export async function clearDeviceEnrollment(): Promise<void> {
  await db.device.update('current', { deviceId: null, lease: null, enrolledAt: null });
}

// Re-imports the stored private key as a CryptoKey, for future use signing
// locally created events (not yet needed -- no caller uses this today).
export async function importDevicePrivateKey(record: DeviceRecord): Promise<CryptoKey> {
  const bytes = base64Decode(record.privateKeyPkcs8B64);
  return crypto.subtle.importKey('pkcs8', bytes.buffer, { name: 'Ed25519' }, false, ['sign']);
}
