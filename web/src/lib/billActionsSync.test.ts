import { afterEach, describe, expect, it, vi } from 'vitest';

import { ApiError, OfflineError } from '../api/client';
import type { Bill, BillDetail, BillRound, BillPayment } from '../api/tabs';
import { db, type PendingBillAction } from './db';
import { flushPendingBillActions, listFailedBillActions } from './billActionsSync';

const { openBill, addSaleRound, removeBillItem, recordPayment, closeBill, getBillDetail } =
  vi.hoisted(() => ({
    openBill: vi.fn(),
    addSaleRound: vi.fn(),
    removeBillItem: vi.fn(),
    recordPayment: vi.fn(),
    closeBill: vi.fn(),
    getBillDetail: vi.fn(),
  }));

vi.mock('../api/tabs', () => ({
  openBill,
  addSaleRound,
  removeBillItem,
  recordPayment,
  closeBill,
  getBillDetail,
}));

function bill(id: string): Bill {
  return { id, status: 'open', tableId: null, customerId: null, balanceKobo: 0, openedAt: 'now' };
}

function round(): BillRound {
  return { id: 'round-1', occurredAt: 'now', totalKobo: 100000, sellerName: '', items: [] };
}

function payment(): BillPayment {
  return { amountKobo: 100000, method: 'cash' };
}

function detail(status: BillDetail['status']): BillDetail {
  return { ...bill('b1'), status, sales: [], payments: [], writeOffs: [] };
}

function action(overrides: Partial<PendingBillAction>): PendingBillAction {
  return {
    id: crypto.randomUUID(),
    businessId: 'biz-1',
    localSessionId: 'session-1',
    billId: null,
    idempotencyKey: crypto.randomUUID(),
    occurredAt: 'now',
    createdAt: new Date().toISOString(),
    status: 'pending',
    payload: { type: 'open' },
    ...overrides,
  };
}

afterEach(async () => {
  vi.clearAllMocks();
  await db.pendingBillActions.clear();
});

describe('flushPendingBillActions', () => {
  it('resolves an open-then-add_round session in order and empties the queue', async () => {
    openBill.mockResolvedValue(bill('real-bill-1'));
    addSaleRound.mockResolvedValue(round());

    const openAction = action({
      id: 'a1',
      createdAt: '2026-01-01T00:00:00Z',
      idempotencyKey: 'open-key',
      payload: { type: 'open', tableId: 'table-1' },
    });
    const roundAction = action({
      id: 'a2',
      createdAt: '2026-01-01T00:00:01Z',
      idempotencyKey: 'round-key',
      payload: {
        type: 'add_round',
        items: [{ variantId: 'v1', description: 'Beer', quantity: 1, unitPriceKobo: 100000 }],
      },
    });
    await db.pendingBillActions.bulkPut([openAction, roundAction]);

    const resolved = await flushPendingBillActions('biz-1', 'csrf');

    expect(resolved.get('session-1')).toBe('real-bill-1');
    expect(openBill).toHaveBeenCalledWith(
      { tableId: 'table-1', customerId: undefined, idempotencyKey: 'open-key' },
      'biz-1',
      'csrf',
    );
    expect(addSaleRound).toHaveBeenCalledWith(
      'real-bill-1',
      {
        idempotencyKey: 'round-key',
        occurredAt: 'now',
        items: [{ variantId: 'v1', quantity: 1, unitPriceKobo: 100000 }],
      },
      'biz-1',
      'csrf',
    );
    await expect(db.pendingBillActions.count()).resolves.toBe(0);
  });

  it('queues directly against an already-resolved bill with no open step needed', async () => {
    recordPayment.mockResolvedValue(payment());

    await db.pendingBillActions.put(
      action({
        id: 'a1',
        billId: 'already-open-bill',
        localSessionId: 'already-open-bill',
        payload: { type: 'record_payment', amountKobo: 50000, method: 'cash' },
      }),
    );

    const resolved = await flushPendingBillActions('biz-1', 'csrf');

    expect(resolved.get('already-open-bill')).toBe('already-open-bill');
    expect(recordPayment).toHaveBeenCalledWith(
      'already-open-bill',
      50000,
      'cash',
      'biz-1',
      'csrf',
      expect.any(String),
    );
    await expect(db.pendingBillActions.count()).resolves.toBe(0);
  });

  it('stops a session at the first OfflineError and leaves the rest pending for next time', async () => {
    openBill.mockResolvedValue(bill('real-bill-2'));
    addSaleRound.mockRejectedValue(new OfflineError());

    await db.pendingBillActions.bulkPut([
      action({ id: 'a1', createdAt: '1', payload: { type: 'open' } }),
      action({
        id: 'a2',
        createdAt: '2',
        payload: {
          type: 'add_round',
          items: [{ variantId: 'v1', description: 'Beer', quantity: 1, unitPriceKobo: 1000 }],
        },
      }),
    ]);

    const resolved = await flushPendingBillActions('biz-1', 'csrf');

    // The open step itself succeeded and is reported resolved...
    expect(resolved.get('session-1')).toBe('real-bill-2');
    // ...but the still-offline add_round is left pending, not deleted or
    // marked failed -- this is "still offline," not a rejection.
    const remaining = await db.pendingBillActions.toArray();
    expect(remaining).toHaveLength(1);
    expect(remaining[0]?.id).toBe('a2');
    expect(remaining[0]?.status).toBe('pending');
  });

  it('marks a real server rejection as failed with a reason, not retried forever', async () => {
    addSaleRound.mockRejectedValue(
      new ApiError(409, {
        code: 'CONFLICT',
        message: 'cannot remove more than is currently on the bill',
        request_id: 'req-1',
        details: {},
      }),
    );

    await db.pendingBillActions.put(
      action({
        id: 'a1',
        billId: 'resolved-bill',
        localSessionId: 'resolved-bill',
        payload: {
          type: 'add_round',
          items: [{ variantId: 'v1', description: 'Beer', quantity: 1, unitPriceKobo: 1000 }],
        },
      }),
    );

    await flushPendingBillActions('biz-1', 'csrf');

    const failed = await listFailedBillActions('biz-1', 'resolved-bill');
    expect(failed).toHaveLength(1);
    expect(failed[0]?.status).toBe('failed');
    expect(failed[0]?.error).toContain('cannot remove more than is currently on the bill');
  });

  it('treats a replayed close against an already-closed bill as fulfilled, not a failure', async () => {
    closeBill.mockRejectedValue(
      new ApiError(409, {
        code: 'CONFLICT',
        message: 'bill is not open',
        request_id: 'req-1',
        details: {},
      }),
    );
    getBillDetail.mockResolvedValue(detail('closed_unpaid'));

    await db.pendingBillActions.put(
      action({
        id: 'a1',
        billId: 'resolved-bill',
        localSessionId: 'resolved-bill',
        payload: { type: 'close' },
      }),
    );

    await flushPendingBillActions('biz-1', 'csrf');

    await expect(db.pendingBillActions.count()).resolves.toBe(0);
    const failed = await listFailedBillActions('biz-1', 'resolved-bill');
    expect(failed).toHaveLength(0);
  });

  it('does not let a stuck session block flushing a different bill', async () => {
    openBill.mockResolvedValue(bill('real-bill-3'));
    addSaleRound.mockImplementation((billId: string) => {
      if (billId === 'real-bill-3')
        return Promise.reject(
          new ApiError(409, {
            code: 'CONFLICT',
            message: 'insufficient quantity',
            request_id: 'req-1',
            details: {},
          }),
        );
      return Promise.resolve(round());
    });

    await db.pendingBillActions.bulkPut([
      action({
        id: 'stuck-open',
        localSessionId: 'stuck',
        createdAt: '1',
        payload: { type: 'open' },
      }),
      action({
        id: 'stuck-round',
        localSessionId: 'stuck',
        createdAt: '2',
        payload: {
          type: 'add_round',
          items: [{ variantId: 'v1', description: 'Beer', quantity: 1, unitPriceKobo: 1000 }],
        },
      }),
      action({
        id: 'other-payment',
        localSessionId: 'other-bill',
        billId: 'other-bill',
        createdAt: '1',
        payload: { type: 'record_payment', amountKobo: 1000, method: 'cash' },
      }),
    ]);
    recordPayment.mockResolvedValue(payment());

    await flushPendingBillActions('biz-1', 'csrf');

    // The unrelated bill's payment still went through despite the other
    // session failing.
    expect(recordPayment).toHaveBeenCalledWith(
      'other-bill',
      1000,
      'cash',
      'biz-1',
      'csrf',
      expect.any(String),
    );
    await expect(
      db.pendingBillActions.where('localSessionId').equals('other-bill').count(),
    ).resolves.toBe(0);
    const failed = await listFailedBillActions('biz-1', 'real-bill-3');
    expect(failed).toHaveLength(1);
  });
});
