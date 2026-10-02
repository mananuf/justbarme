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

// A physical stock count recorded on this device, written here BEFORE any
// network call -- same "queue first" discipline as PendingSale. expectedQuantity
// is read from catalogueCache at the moment counting happens (the device's
// last-known balance), never re-derived later -- staleness is decided
// server-side by comparing this figure against the *live* balance when the
// count is actually processed (docs/PHASE_INVENTORY_COUNTS_AND_ADJUSTMENTS.md
// §3), not by any client-side clock.
export interface PendingStockCount {
  idempotencyKey: string;
  businessId: string;
  startedAt: string;
  lines: { variantId: string; expectedQuantity: number; physicalQuantity: number }[];
  status: 'pending' | 'synced';
  createdAt: string;
}

// A staff-initiated adjustment request (complimentary/broken/spoiled/
// staff_use/manual) recorded here before any network call -- same reasoning
// as PendingStockCount. Never carries reasonCategory 'count_correction';
// that one is only ever server-generated from a stock count.
export interface PendingAdjustmentRequest {
  idempotencyKey: string;
  businessId: string;
  variantId: string;
  quantityDelta: number;
  reasonCategory: string;
  reasonNote: string;
  status: 'pending' | 'synced';
  createdAt: string;
}

// A staff-recorded expense, written here before any network call --
// expenses:record is offline-safe (internal/tenancy/capabilities.go's
// offlineSafeCapabilities), same "queue first" discipline as every other
// pending* table here.
export interface PendingExpense {
  idempotencyKey: string;
  businessId: string;
  categoryId: string;
  description: string;
  amountKobo: number;
  paymentMethod: 'cash' | 'transfer' | 'card';
  occurredAt: string;
  status: 'pending' | 'synced';
  createdAt: string;
}

// A queued step of a bill's lifecycle (open, add a round, remove an item,
// record a payment, close), written here before any network call -- same
// "queue first" discipline as every other pending* table, extended to
// cover a *sequence* of dependent actions rather than one atomic one,
// since a bill (unlike a one-shot sale) is built up over several separate
// calls. docs/PHASE_UNIFIED_SELL_BILLS.md's "Revision" section: walk-in
// now goes through this exact same mechanism as a table/customer tab, so
// it needed a real offline story again once it stopped being a special
// case.
//
// localSessionId groups every action belonging to one bill's queue
// together, replayed in `createdAt` order. billId is null until this
// session's own 'open' action has synced -- every later action in the
// same session is queued not knowing the real bill id yet, and
// billActionsSync.ts resolves it at flush time, in order, rather than
// this needing to guess or wait synchronously for a round trip.
export type PendingBillActionPayload =
  | { type: 'open'; tableId?: string; customerId?: string }
  | {
      type: 'add_round';
      items: { variantId: string; description: string; quantity: number; unitPriceKobo: number }[];
    }
  | {
      type: 'remove_item';
      variantId: string;
      description: string;
      quantity: number;
      unitPriceKobo: number;
    }
  | { type: 'record_payment'; amountKobo: number; method: 'cash' | 'transfer' | 'card' }
  | { type: 'close' };

export interface PendingBillAction {
  id: string;
  businessId: string;
  localSessionId: string;
  billId: string | null;
  payload: PendingBillActionPayload;
  idempotencyKey: string;
  occurredAt: string;
  createdAt: string;
  // 'pending': not yet synced, will be retried. 'failed': synced enough to
  // discover the server rejected it for a reason that reflects real,
  // changed state (not connectivity) -- surfaced to the owner/staff
  // viewing that bill rather than retried forever or silently dropped
  // (docs/PHASE_UNIFIED_SELL_BILLS.md's "needs attention" design).
  status: 'pending' | 'failed';
  error?: string;
}

class JustbarmeDB extends Dexie {
  authMeta!: EntityTable<CachedIdentity, 'id'>;
  device!: EntityTable<DeviceRecord, 'id'>;
  pendingSales!: EntityTable<PendingSale, 'idempotencyKey'>;
  catalogueCache!: EntityTable<CachedCatalogue, 'businessId'>;
  pendingStockCounts!: EntityTable<PendingStockCount, 'idempotencyKey'>;
  pendingAdjustmentRequests!: EntityTable<PendingAdjustmentRequest, 'idempotencyKey'>;
  pendingExpenses!: EntityTable<PendingExpense, 'idempotencyKey'>;
  pendingBillActions!: EntityTable<PendingBillAction, 'id'>;

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
    this.version(4).stores({
      authMeta: 'id',
      device: 'id',
      pendingSales: 'idempotencyKey, businessId, status, createdAt',
      catalogueCache: 'businessId',
      pendingStockCounts: 'idempotencyKey, businessId, status, createdAt',
      pendingAdjustmentRequests: 'idempotencyKey, businessId, status, createdAt',
    });
    this.version(5).stores({
      authMeta: 'id',
      device: 'id',
      pendingSales: 'idempotencyKey, businessId, status, createdAt',
      catalogueCache: 'businessId',
      pendingStockCounts: 'idempotencyKey, businessId, status, createdAt',
      pendingAdjustmentRequests: 'idempotencyKey, businessId, status, createdAt',
      pendingExpenses: 'idempotencyKey, businessId, status, createdAt',
    });
    this.version(6).stores({
      authMeta: 'id',
      device: 'id',
      pendingSales: 'idempotencyKey, businessId, status, createdAt',
      catalogueCache: 'businessId',
      pendingStockCounts: 'idempotencyKey, businessId, status, createdAt',
      pendingAdjustmentRequests: 'idempotencyKey, businessId, status, createdAt',
      pendingExpenses: 'idempotencyKey, businessId, status, createdAt',
      pendingBillActions: 'id, businessId, localSessionId, billId, status, createdAt',
    });
  }
}

export const db = new JustbarmeDB();
