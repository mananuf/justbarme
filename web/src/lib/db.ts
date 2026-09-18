import Dexie, { type EntityTable } from 'dexie';

import type { CatalogueProduct } from '../api/catalogue';

// Minimal local identity cache -- docs/ARCHITECTURE.md §5.3's `auth_meta`
// store. There is exactly one row per table, keyed by a fixed id, since a
// device holds one identity/business/lease at a time in the MVP.

export interface CachedIdentity {
  id: 'current';
  userId: string;
  name: string;
  email: string;
  phone: string;
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

// A walk-in sale recorded on this device, written here BEFORE any network
// call is attempted -- this is what makes "sale recorded" true even fully
// offline, unlike the placeholder text that used to sit on this screen.
// idempotencyKey is both the Dexie primary key and the value sent to
// POST /api/v1/sales, so retrying a still-pending row after a failed
// attempt can never double-post once it does reach the server (see
// docs/PHASE_STOCK_RECEIVING.md's sibling reasoning in internal/sales).
export interface PendingSale {
  idempotencyKey: string;
  businessId: string;
  occurredAt: string;
  items: { variantId: string; description: string; quantity: number; unitPriceKobo: number }[];
  payment: { amountKobo: number; method: 'cash' | 'transfer' | 'card' };
  totalKobo: number;
  status: 'pending' | 'synced';
  createdAt: string;
}

// A device's last-known-good product catalogue for one business --
// docs/PHASE_OFFLINE_CATALOGUE_AND_REVIEWS.md. Keyed by businessId (not a
// fixed 'current' id, matching authMeta's own multi-membership handling)
// since a device can belong to more than one business. Always overwritten
// on a successful GET /products; only read back when that fetch fails, so
// Quick Sell still works on a device that's offline the first time it
// opens, or reloads while offline.
export interface CachedCatalogue {
  businessId: string;
  products: CatalogueProduct[];
  cachedAt: string;
}

class JustbarmeDB extends Dexie {
  authMeta!: EntityTable<CachedIdentity, 'id'>;
  device!: EntityTable<DeviceRecord, 'id'>;
  pendingSales!: EntityTable<PendingSale, 'idempotencyKey'>;
  catalogueCache!: EntityTable<CachedCatalogue, 'businessId'>;

  constructor() {
    super('justbarme');
    this.version(1).stores({
      authMeta: 'id',
      device: 'id',
    });
    this.version(2).stores({
      authMeta: 'id',
      device: 'id',
      pendingSales: 'idempotencyKey, businessId, status, createdAt',
    });
    this.version(3).stores({
      authMeta: 'id',
      device: 'id',
      pendingSales: 'idempotencyKey, businessId, status, createdAt',
      catalogueCache: 'businessId',
    });
  }
}

export const db = new JustbarmeDB();
