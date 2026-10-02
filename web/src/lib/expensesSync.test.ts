import { afterEach, describe, expect, it, vi } from 'vitest';

import { ApiError, OfflineError } from '../api/client';
import { db, type PendingExpense } from './db';
import {
  discardPendingExpense,
  flushPendingExpenses,
  listFailedExpenses,
  queuePendingExpense,
} from './expensesSync';

const { recordExpense } = vi.hoisted(() => ({ recordExpense: vi.fn() }));

vi.mock('../api/expenses', () => ({ recordExpense }));

function expense(overrides: Partial<PendingExpense> = {}): PendingExpense {
  return {
    idempotencyKey: crypto.randomUUID(),
    businessId: 'biz-1',
    categoryId: 'cat-1',
    description: 'Ice',
    amountKobo: 50000,
    paymentMethod: 'cash',
    occurredAt: 'now',
    status: 'pending',
    createdAt: new Date().toISOString(),
    ...overrides,
  };
}

afterEach(async () => {
  vi.clearAllMocks();
  await db.pendingExpenses.clear();
});

describe('flushPendingExpenses', () => {
  it('posts every pending expense oldest-first and empties the queue', async () => {
    recordExpense.mockResolvedValue(undefined);
    const first = expense({ idempotencyKey: 'e1', createdAt: '2026-01-01T00:00:00Z' });
    const second = expense({ idempotencyKey: 'e2', createdAt: '2026-01-01T00:00:01Z' });
    await db.pendingExpenses.bulkPut([second, first]);

    await flushPendingExpenses('biz-1', 'csrf');

    expect(recordExpense).toHaveBeenNthCalledWith(
      1,
      'e1',
      first.categoryId,
      first.description,
      first.amountKobo,
      first.paymentMethod,
      first.occurredAt,
      'biz-1',
      'csrf',
    );
    await expect(db.pendingExpenses.count()).resolves.toBe(0);
  });

  it('stops at the first OfflineError and leaves it (and everything after it) pending', async () => {
    recordExpense.mockRejectedValue(new OfflineError());
    await db.pendingExpenses.bulkPut([
      expense({ idempotencyKey: 'e1', createdAt: '1' }),
      expense({ idempotencyKey: 'e2', createdAt: '2' }),
    ]);

    await flushPendingExpenses('biz-1', 'csrf');

    const remaining = await db.pendingExpenses.toArray();
    expect(remaining).toHaveLength(2);
    expect(remaining.every((e) => e.status === 'pending')).toBe(true);
  });

  it('marks a real server rejection failed and continues past it to the next expense', async () => {
    recordExpense.mockImplementation((idempotencyKey: string) => {
      if (idempotencyKey === 'bad') {
        return Promise.reject(
          new ApiError(422, {
            code: 'VALIDATION_FAILED',
            message: 'amount_kobo must be positive',
            request_id: 'req-1',
            details: {},
          }),
        );
      }
      return Promise.resolve(undefined);
    });
    await db.pendingExpenses.bulkPut([
      expense({ idempotencyKey: 'bad', createdAt: '1' }),
      expense({ idempotencyKey: 'ok', createdAt: '2' }),
    ]);

    await flushPendingExpenses('biz-1', 'csrf');

    expect(recordExpense).toHaveBeenCalledTimes(2);
    await expect(db.pendingExpenses.get('ok')).resolves.toBeUndefined();
    const failed = await listFailedExpenses('biz-1');
    expect(failed).toHaveLength(1);
    expect(failed[0]?.idempotencyKey).toBe('bad');
    expect(failed[0]?.error).toContain('amount_kobo must be positive');
  });
});

describe('discardPendingExpense', () => {
  it('removes a failed expense from the queue', async () => {
    await queuePendingExpense(expense({ idempotencyKey: 'e1', status: 'failed', error: 'nope' }));

    await discardPendingExpense('e1');

    await expect(db.pendingExpenses.get('e1')).resolves.toBeUndefined();
  });
});
