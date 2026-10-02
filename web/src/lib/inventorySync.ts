import { OfflineError } from '../api/client';
import {
  requestAdjustment,
  submitStockCount,
  type AdjustmentReasonCategory,
} from '../api/inventory';
import { describeActionError } from './errors';
import { db, type PendingAdjustmentRequest, type PendingStockCount } from './db';

export async function queuePendingStockCount(count: PendingStockCount): Promise<void> {
  await db.pendingStockCounts.put(count);
}

export async function queuePendingAdjustmentRequest(
  request: PendingAdjustmentRequest,
): Promise<void> {
  await db.pendingAdjustmentRequests.put(request);
}

// flushPendingStockCounts / flushPendingAdjustmentRequests: oldest first,
// relying on each row's idempotencyKey to make retrying safe. Same
// OfflineError-vs-real-rejection split as flushPendingExpenses (see its
// doc comment) -- still offline stops the whole flush here and retries
// everything next time; a real server rejection marks just that one row
// 'failed' and moves on, since neither queue has an ordering dependency
// between its own rows (docs/PHASE_OFFLINE_QUEUE_FAILURES.md).
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
    } catch (err) {
      if (err instanceof OfflineError) {
        return;
      }
      await db.pendingStockCounts.update(count.idempotencyKey, {
        status: 'failed',
        error: describeActionError(err, 'This count could not be saved.'),
      });
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
    } catch (err) {
      if (err instanceof OfflineError) {
        return;
      }
      await db.pendingAdjustmentRequests.update(request.idempotencyKey, {
        status: 'failed',
        error: describeActionError(err, 'This adjustment request could not be saved.'),
      });
    }
  }
}

// listFailedStockCounts / listFailedAdjustmentRequests / discard* mirror
// expensesSync.ts's equivalents -- surfaced on Stock.tsx so a rejected
// offline count or adjustment request isn't silently stuck forever.
export async function listFailedStockCounts(businessId: string): Promise<PendingStockCount[]> {
  return db.pendingStockCounts
    .where('businessId')
    .equals(businessId)
    .and((c) => c.status === 'failed')
    .toArray();
}

export async function discardPendingStockCount(idempotencyKey: string): Promise<void> {
  await db.pendingStockCounts.delete(idempotencyKey);
}

export async function listFailedAdjustmentRequests(
  businessId: string,
): Promise<PendingAdjustmentRequest[]> {
  return db.pendingAdjustmentRequests
    .where('businessId')
    .equals(businessId)
    .and((r) => r.status === 'failed')
    .toArray();
}

export async function discardPendingAdjustmentRequest(idempotencyKey: string): Promise<void> {
  await db.pendingAdjustmentRequests.delete(idempotencyKey);
}
