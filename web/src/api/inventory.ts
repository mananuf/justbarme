import { apiRequest } from './client';

export interface ReceiveStockLine {
  variantId: string;
  quantity: number;
  totalCostKobo: number;
}

export interface ReceiptLineResult {
  variantId: string;
  quantity: number;
  totalCostKobo: number;
  newBalance: number;
}

export interface Receipt {
  id: string;
  receivedAt: string;
  lines: ReceiptLineResult[];
}

interface RawReceiptLine {
  variant_id: string;
  quantity: number;
  total_cost_kobo: number;
  new_balance: number;
}

interface RawReceipt {
  id: string;
  received_at: string;
  lines: RawReceiptLine[];
}

// receiveStock wraps POST /api/v1/stock-receipts. The server resolves the
// receiving location itself (the business's default location) -- the
// client never sends one, since multi-location isn't a pilot concern (see
// docs/PHASE_STOCK_RECEIVING.md).
export async function receiveStock(
  lines: ReceiveStockLine[],
  businessId: string,
  csrfToken: string,
): Promise<Receipt> {
  const raw = await apiRequest<RawReceipt>('/api/v1/stock-receipts', {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken, 'X-Business-ID': businessId },
    body: JSON.stringify({
      lines: lines.map((l) => ({
        variant_id: l.variantId,
        quantity: l.quantity,
        total_cost_kobo: l.totalCostKobo,
      })),
    }),
  });
  return {
    id: raw.id,
    receivedAt: raw.received_at,
    lines: raw.lines.map((l) => ({
      variantId: l.variant_id,
      quantity: l.quantity,
      totalCostKobo: l.total_cost_kobo,
      newBalance: l.new_balance,
    })),
  };
}

// docs/PHASE_INVENTORY_COUNTS_AND_ADJUSTMENTS.md -- stock counts, staff
// adjustment requests + owner approval, and stock history.

export type AdjustmentReasonCategory =
  'complimentary' | 'broken' | 'spoiled' | 'staff_use' | 'manual' | 'count_correction';

export interface StockCountLineInput {
  variantId: string;
  expectedQuantity: number;
  physicalQuantity: number;
}

export interface StockCountLineResult {
  id: string;
  variantId: string;
  expectedQuantity: number;
  physicalQuantity: number;
  variance: number;
  isStale: boolean;
}

export interface StockCount {
  id: string;
  lines: StockCountLineResult[];
}

interface RawStockCountLine {
  id: string;
  variant_id: string;
  expected_quantity: number;
  physical_quantity: number;
  variance: number;
  is_stale: boolean;
}

interface RawStockCount {
  id: string;
  lines: RawStockCountLine[];
}

function toStockCount(raw: RawStockCount): StockCount {
  return {
    id: raw.id,
    lines: raw.lines.map((l) => ({
      id: l.id,
      variantId: l.variant_id,
      expectedQuantity: l.expected_quantity,
      physicalQuantity: l.physical_quantity,
      variance: l.variance,
      isStale: l.is_stale,
    })),
  };
}

// submitStockCount wraps POST /api/v1/stock-counts (inventory:count,
// offline-safe). idempotencyKey makes a queued-offline submission safe to
// retry -- a resubmitted count returns the identical result rather than
// posting a second one. Staleness is decided server-side by comparing each
// line's expectedQuantity against the *live* balance at processing time,
// not the client's clock.
export async function submitStockCount(
  idempotencyKey: string,
  startedAt: string,
  lines: StockCountLineInput[],
  businessId: string,
  csrfToken: string,
): Promise<StockCount> {
  const raw = await apiRequest<RawStockCount>('/api/v1/stock-counts', {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken, 'X-Business-ID': businessId },
    body: JSON.stringify({
      idempotency_key: idempotencyKey,
      started_at: startedAt,
      lines: lines.map((l) => ({
        variant_id: l.variantId,
        expected_quantity: l.expectedQuantity,
        physical_quantity: l.physicalQuantity,
      })),
    }),
  });
  return toStockCount(raw);
}

export interface AdjustmentRequest {
  id: string;
  variantId: string;
  variantName: string;
  productName: string;
  quantityDelta: number;
  reasonCategory: AdjustmentReasonCategory;
  reasonNote: string;
  status: 'pending' | 'approved' | 'rejected';
  requestedByName: string;
  createdAt: string;
}

interface RawAdjustmentRequest {
  id: string;
  variant_id: string;
  variant_name?: string;
  product_name?: string;
  quantity_delta: number;
  reason_category: string;
  reason_note: string;
  status: string;
  requested_by_name?: string;
  created_at: string;
}

function toAdjustmentRequest(raw: RawAdjustmentRequest): AdjustmentRequest {
  return {
    id: raw.id,
    variantId: raw.variant_id,
    variantName: raw.variant_name ?? '',
    productName: raw.product_name ?? '',
    quantityDelta: raw.quantity_delta,
    reasonCategory: raw.reason_category as AdjustmentReasonCategory,
    reasonNote: raw.reason_note,
    status: raw.status as AdjustmentRequest['status'],
    requestedByName: raw.requested_by_name ?? '',
    createdAt: raw.created_at,
  };
}

// requestAdjustment wraps POST /api/v1/inventory-adjustments
// (inventory:adjustment_request, offline-safe -- Staff may submit, only an
// Owner may later approve). reasonCategory must not be 'count_correction'
// -- that one only ever comes from submitStockCount itself.
export async function requestAdjustment(
  idempotencyKey: string,
  variantId: string,
  quantityDelta: number,
  reasonCategory: Exclude<AdjustmentReasonCategory, 'count_correction'>,
  reasonNote: string,
  businessId: string,
  csrfToken: string,
): Promise<AdjustmentRequest> {
  const raw = await apiRequest<RawAdjustmentRequest>('/api/v1/inventory-adjustments', {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken, 'X-Business-ID': businessId },
    body: JSON.stringify({
      idempotency_key: idempotencyKey,
      variant_id: variantId,
      quantity_delta: quantityDelta,
      reason_category: reasonCategory,
      reason_note: reasonNote,
    }),
  });
  return toAdjustmentRequest(raw);
}

// listPendingAdjustments wraps GET /api/v1/inventory-adjustments
// (inventory:adjustment_approve, Owner-only) -- the approval queue.
export async function listPendingAdjustments(businessId: string): Promise<AdjustmentRequest[]> {
  const raw = await apiRequest<RawAdjustmentRequest[]>('/api/v1/inventory-adjustments', {
    headers: { 'X-Business-ID': businessId },
  });
  return raw.map(toAdjustmentRequest);
}

// approveAdjustment wraps POST
// /api/v1/inventory-adjustments/{id}/approve (inventory:adjustment_approve,
// Owner-only, deliberately not offline-safe -- approval always requires
// online server authorization). Posts a real inventory movement.
export async function approveAdjustment(
  requestId: string,
  note: string,
  businessId: string,
  csrfToken: string,
): Promise<AdjustmentRequest> {
  const raw = await apiRequest<RawAdjustmentRequest>(
    `/api/v1/inventory-adjustments/${requestId}/approve`,
    {
      method: 'POST',
      headers: { 'X-CSRF-Token': csrfToken, 'X-Business-ID': businessId },
      body: JSON.stringify({ note }),
    },
  );
  return toAdjustmentRequest(raw);
}

// rejectAdjustment wraps POST /api/v1/inventory-adjustments/{id}/reject
// (inventory:adjustment_approve, Owner-only). No movement is ever posted.
export async function rejectAdjustment(
  requestId: string,
  note: string,
  businessId: string,
  csrfToken: string,
): Promise<AdjustmentRequest> {
  const raw = await apiRequest<RawAdjustmentRequest>(
    `/api/v1/inventory-adjustments/${requestId}/reject`,
    {
      method: 'POST',
      headers: { 'X-CSRF-Token': csrfToken, 'X-Business-ID': businessId },
      body: JSON.stringify({ note }),
    },
  );
  return toAdjustmentRequest(raw);
}

export interface StockHistoryEntry {
  id: string;
  quantityDelta: number;
  createdAt: string;
  eventType: 'receipt' | 'sale' | 'sale_reversal' | 'adjustment';
  actorName: string;
  adjustmentReasonCategory: string;
  adjustmentReasonNote: string;
  adjustmentDecidedByName: string;
}

interface RawStockHistoryEntry {
  id: string;
  quantity_delta: number;
  created_at: string;
  event_type: string;
  actor_name?: string;
  adjustment_reason_category?: string;
  adjustment_reason_note?: string;
  adjustment_decided_by_name?: string;
}

// getInventoryHistory wraps GET
// /api/v1/products/variants/{variant_id}/history (inventory:read) -- a
// flat, chronological, human-readable feed of every movement for one
// variant, most recent first.
export async function getInventoryHistory(
  variantId: string,
  businessId: string,
  limit = 50,
): Promise<StockHistoryEntry[]> {
  const raw = await apiRequest<RawStockHistoryEntry[]>(
    `/api/v1/products/variants/${variantId}/history?limit=${limit}`,
    { headers: { 'X-Business-ID': businessId } },
  );
  return raw.map((h) => ({
    id: h.id,
    quantityDelta: h.quantity_delta,
    createdAt: h.created_at,
    eventType: h.event_type as StockHistoryEntry['eventType'],
    actorName: h.actor_name ?? '',
    adjustmentReasonCategory: h.adjustment_reason_category ?? '',
    adjustmentReasonNote: h.adjustment_reason_note ?? '',
    adjustmentDecidedByName: h.adjustment_decided_by_name ?? '',
  }));
}
