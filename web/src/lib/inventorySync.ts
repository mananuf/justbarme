import {
  requestAdjustment,
  submitStockCount,
  type AdjustmentReasonCategory,
} from '../api/inventory';
import { db, type PendingAdjustmentRequest, type PendingStockCount } from './db';

export async function queuePendingStockCount(count: PendingStockCount): Promise<void> {
  await db.pendingStockCounts.put(count);
}

export async function queuePendingAdjustmentRequest(
  request: PendingAdjustmentRequest,
): Promise<void> {
  await db.pendingAdjustmentRequests.put(request);
}

// flushPendingStockCounts / flushPendingAdjustmentRequests mirror
// flushPendingSales exactly -- oldest first, stop at the first failure, and
// rely on each row's idempotencyKey to make retrying safe. Each queue is
// flushed independently since a count and an adjustment request post to
// different endpoints and neither depends on the other having synced.
export async function flushPendingStockCounts(
  businessId: string,
  csrfToken: string,
): Promise<void> {
  const pending = await db.pendingStockCounts
    .where('businessId')
    .equals(businessId)
    .and((c) => c.status === 'pending')
    .sortBy('createdAt');

  for (const count of pending) {
    try {
      await submitStockCount(
        count.idempotencyKey,
        count.startedAt,
        count.lines,
        businessId,
        csrfToken,
      );
      await db.pendingStockCounts.delete(count.idempotencyKey);
    } catch {
      return;
    }
  }
}

export async function flushPendingAdjustmentRequests(
  businessId: string,
  csrfToken: string,
): Promise<void> {
  const pending = await db.pendingAdjustmentRequests
    .where('businessId')
    .equals(businessId)
    .and((r) => r.status === 'pending')
    .sortBy('createdAt');

  for (const request of pending) {
    try {
      await requestAdjustment(
        request.idempotencyKey,
        request.variantId,
        request.quantityDelta,
        request.reasonCategory as Exclude<AdjustmentReasonCategory, 'count_correction'>,
        request.reasonNote,
        businessId,
        csrfToken,
      );
      await db.pendingAdjustmentRequests.delete(request.idempotencyKey);
    } catch {
      return;
    }
  }
}
