import Dexie, { type EntityTable } from 'dexie';

// Minimal local identity cache -- docs/ARCHITECTURE.md §5.3's `auth_meta`
// store. There is exactly one row per table, keyed by a fixed id, since a
// device holds one identity/business/lease at a time in the MVP.

export interface CachedIdentity {
  id: 'current';
  userId: string;
  name: string;
  email: string;
  memberships: { businessId: string; businessName: string; role: string }[];
  selectedBusinessId: string | null;
  cachedAt: string;
}

export interface DeviceRecord {
  id: 'current';
  deviceId: string | null; // null until POST /devices/enroll succeeds
  publicKeyB64: string;
  privateKeyPkcs8B64: string;
  displayName: string;
  lease: string | null; // the raw "v1.<payload>.<signature>" token
  enrolledAt: string | null;
}

class JustbarmeDB extends Dexie {
  authMeta!: EntityTable<CachedIdentity, 'id'>;
  device!: EntityTable<DeviceRecord, 'id'>;

  constructor() {
    super('justbarme');
    this.version(1).stores({
      authMeta: 'id',
      device: 'id',
    });
  }
}

export const db = new JustbarmeDB();
