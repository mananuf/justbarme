import { afterEach, describe, expect, it, vi } from 'vitest';

import { ApiError, OfflineError } from '../api/client';
import { db, type PendingAdjustmentRequest, type PendingStockCount } from './db';
import {
  discardPendingAdjustmentRequest,
  discardPendingStockCount,
  flushPendingAdjustmentRequests,
  flushPendingStockCounts,
  listFailedAdjustmentRequests,
  listFailedStockCounts,
  queuePendingAdjustmentRequest,
  queuePendingStockCount,
} from './inventorySync';

const { submitStockCount, requestAdjustment } = vi.hoisted(() => ({
  submitStockCount: vi.fn(),
  requestAdjustment: vi.fn(),
}));

vi.mock('../api/inventory', () => ({ submitStockCount, requestAdjustment }));

function count(overrides: Partial<PendingStockCount> = {}): PendingStockCount {
  return {
    idempotencyKey: crypto.randomUUID(),
    businessId: 'biz-1',
    startedAt: 'now',
    lines: [{ variantId: 'v1', expectedQuantity: 10, physicalQuantity: 8 }],
    status: 'pending',
    createdAt: new Date().toISOString(),
    ...overrides,
  };
}

function adjustment(overrides: Partial<PendingAdjustmentRequest> = {}): PendingAdjustmentRequest {
  return {
    idempotencyKey: crypto.randomUUID(),
    businessId: 'biz-1',
    variantId: 'v1',
    quantityDelta: -2,
    reasonCategory: 'broken',
    reasonNote: 'dropped a bottle',
    status: 'pending',
    createdAt: new Date().toISOString(),
    ...overrides,
  };
}

afterEach(async () => {
  vi.clearAllMocks();
  await db.pendingStockCounts.clear();
  await db.pendingAdjustmentRequests.clear();
});

describe('flushPendingStockCounts', () => {
  it('posts every pending count oldest-first and empties the queue', async () => {
    submitStockCount.mockResolvedValue(undefined);
    const first = count({ idempotencyKey: 'c1', createdAt: '2026-01-01T00:00:00Z' });
    const second = count({ idempotencyKey: 'c2', createdAt: '2026-01-01T00:00:01Z' });
    await db.pendingStockCounts.bulkPut([second, first]);

    await flushPendingStockCounts('biz-1', 'csrf');

    expect(submitStockCount).toHaveBeenNthCalledWith(
      1,
      'c1',
      first.startedAt,
      first.lines,
      'biz-1',
      'csrf',
    );
    await expect(db.pendingStockCounts.count()).resolves.toBe(0);
  });

  it('stops at the first OfflineError and leaves it pending', async () => {
    submitStockCount.mockRejectedValue(new OfflineError());
    await db.pendingStockCounts.put(count({ idempotencyKey: 'c1' }));

    await flushPendingStockCounts('biz-1', 'csrf');

    const remaining = await db.pendingStockCounts.toArray();
    expect(remaining).toHaveLength(1);
    expect(remaining[0]?.status).toBe('pending');
  });

  it('marks a real server rejection failed and continues past it', async () => {
    submitStockCount.mockImplementation((idempotencyKey: string) => {
      if (idempotencyKey === 'bad') {
        return Promise.reject(
          new ApiError(409, {
            code: 'CONFLICT',
            message: 'variant is deactivated',
            request_id: 'req-1',
            details: {},
          }),
        );
      }
      return Promise.resolve(undefined);
    });
    await db.pendingStockCounts.bulkPut([
      count({ idempotencyKey: 'bad', createdAt: '1' }),
      count({ idempotencyKey: 'ok', createdAt: '2' }),
    ]);

    await flushPendingStockCounts('biz-1', 'csrf');

    expect(submitStockCount).toHaveBeenCalledTimes(2);
    await expect(db.pendingStockCounts.get('ok')).resolves.toBeUndefined();
    const failed = await listFailedStockCounts('biz-1');
    expect(failed).toHaveLength(1);
    expect(failed[0]?.idempotencyKey).toBe('bad');
    expect(failed[0]?.error).toContain('variant is deactivated');
  });
});

describe('discardPendingStockCount', () => {
  it('removes a failed count from the queue', async () => {
    await queuePendingStockCount(count({ idempotencyKey: 'c1', status: 'failed', error: 'nope' }));

    await discardPendingStockCount('c1');

    await expect(db.pendingStockCounts.get('c1')).resolves.toBeUndefined();
  });
});

describe('flushPendingAdjustmentRequests', () => {
  it('posts every pending request oldest-first and empties the queue', async () => {
    requestAdjustment.mockResolvedValue(undefined);
    const first = adjustment({ idempotencyKey: 'a1', createdAt: '2026-01-01T00:00:00Z' });
    const second = adjustment({ idempotencyKey: 'a2', createdAt: '2026-01-01T00:00:01Z' });
    await db.pendingAdjustmentRequests.bulkPut([second, first]);

    await flushPendingAdjustmentRequests('biz-1', 'csrf');

    expect(requestAdjustment).toHaveBeenNthCalledWith(
      1,
      'a1',
      first.variantId,
      first.quantityDelta,
      first.reasonCategory,
      first.reasonNote,
      'biz-1',
      'csrf',
    );
    await expect(db.pendingAdjustmentRequests.count()).resolves.toBe(0);
  });

  it('stops at the first OfflineError and leaves it pending', async () => {
    requestAdjustment.mockRejectedValue(new OfflineError());
    await db.pendingAdjustmentRequests.put(adjustment({ idempotencyKey: 'a1' }));

    await flushPendingAdjustmentRequests('biz-1', 'csrf');

    const remaining = await db.pendingAdjustmentRequests.toArray();
    expect(remaining).toHaveLength(1);
    expect(remaining[0]?.status).toBe('pending');
  });

  it('marks a real server rejection failed and continues past it', async () => {
    requestAdjustment.mockImplementation((idempotencyKey: string) => {
      if (idempotencyKey === 'bad') {
        return Promise.reject(
          new ApiError(403, {
            code: 'PERMISSION_DENIED',
            message: 'only an owner can approve this',
            request_id: 'req-1',
            details: {},
          }),
        );
      }
      return Promise.resolve(undefined);
    });
    await db.pendingAdjustmentRequests.bulkPut([
      adjustment({ idempotencyKey: 'bad', createdAt: '1' }),
      adjustment({ idempotencyKey: 'ok', createdAt: '2' }),
    ]);

    await flushPendingAdjustmentRequests('biz-1', 'csrf');

    expect(requestAdjustment).toHaveBeenCalledTimes(2);
    await expect(db.pendingAdjustmentRequests.get('ok')).resolves.toBeUndefined();
    const failed = await listFailedAdjustmentRequests('biz-1');
    expect(failed).toHaveLength(1);
    expect(failed[0]?.idempotencyKey).toBe('bad');
    expect(failed[0]?.error).toContain('only an owner can approve this');
  });
});

describe('discardPendingAdjustmentRequest', () => {
  it('removes a failed adjustment request from the queue', async () => {
    await queuePendingAdjustmentRequest(
      adjustment({ idempotencyKey: 'a1', status: 'failed', error: 'nope' }),
    );

    await discardPendingAdjustmentRequest('a1');

    await expect(db.pendingAdjustmentRequests.get('a1')).resolves.toBeUndefined();
  });
});
