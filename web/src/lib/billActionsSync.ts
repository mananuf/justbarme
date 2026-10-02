import { OfflineError } from '../api/client';
import {
  addSaleRound,
  closeBill,
  getBillDetail,
  openBill,
  recordPayment,
  removeBillItem,
} from '../api/tabs';
import { describeActionError } from './errors';
import { db, type PendingBillAction } from './db';

export async function queueBillAction(action: PendingBillAction): Promise<void> {
  await db.pendingBillActions.put(action);
}

// flushPendingBillActions replays every still-pending action, grouped by
// the bill session it belongs to and ordered oldest-first within each
// session, stopping at the first failure *for that session only* -- a
// stuck bill must not block flushing any other queued sale, count,
// expense, or bill action on the device, same discipline
// flushPendingSales already holds itself to.
//
// Returns a map of localSessionId -> the real bill id that session's
// 'open' action resolved to during this call, for every session that got
// that far (even if a later action in the same session then failed or
// hit the offline stop) -- the one thing a caller watching an
// *unresolved* session (Sell.tsx, before it has a real bill id at all)
// needs back, since the 'open' row is deleted once synced rather than
// kept around just to be read back later.
//
// Three outcomes per action, not two: synced (removed from the queue and
// gone), still offline (left 'pending', retried next flush, stop this
// session here), or a real server rejection that reflects changed state
// (marked 'failed' with a plain reason and left in the queue for the
// owner/staff to see and clear from that bill's own workspace --
// docs/PHASE_UNIFIED_SELL_BILLS.md's "needs attention" design, not a
// silent retry-forever or a silent drop).
export async function flushPendingBillActions(
  businessId: string,
  csrfToken: string,
): Promise<Map<string, string>> {
  const resolvedSessions = new Map<string, string>();
  const pending = await db.pendingBillActions
    .where('businessId')
    .equals(businessId)
    .and((a) => a.status === 'pending')
    .sortBy('createdAt');

  const sessions = new Map<string, PendingBillAction[]>();
  for (const action of pending) {
    const list = sessions.get(action.localSessionId) ?? [];
    list.push(action);
    sessions.set(action.localSessionId, list);
  }

  for (const [sessionId, actions] of sessions) {
    let resolvedBillId: string | null = actions.find((a) => a.billId !== null)?.billId ?? null;
    if (resolvedBillId) resolvedSessions.set(sessionId, resolvedBillId);
    let stop = false;

    for (const action of actions) {
      if (stop) break;
      try {
        if (action.payload.type === 'open') {
          const bill = await openBill(
            {
              tableId: action.payload.tableId,
              customerId: action.payload.customerId,
              idempotencyKey: action.idempotencyKey,
            },
            businessId,
            csrfToken,
          );
          resolvedBillId = bill.id;
          resolvedSessions.set(sessionId, bill.id);
          await db.pendingBillActions.delete(action.id);
          continue;
        }

        if (!resolvedBillId) {
          // This session's own 'open' hasn't synced yet (shouldn't happen
          // given createdAt ordering, but never guess a bill id) -- stop
          // here, not a failure, just not this session's turn yet.
          stop = true;
          break;
        }

        // Stamp the now-known real bill id onto this row before attempting
        // it, not after -- a row created while the session was still
        // unresolved (billId: null) otherwise stays permanently invisible
        // to listFailedBillActions'/BillWorkspace's by-billId lookups if
        // this specific action goes on to fail.
        if (action.billId !== resolvedBillId) {
          await db.pendingBillActions.update(action.id, { billId: resolvedBillId });
        }

        if (action.payload.type === 'add_round') {
          await addSaleRound(
            resolvedBillId,
            {
              idempotencyKey: action.idempotencyKey,
              occurredAt: action.occurredAt,
              items: action.payload.items.map((i) => ({
                variantId: i.variantId,
                quantity: i.quantity,
                unitPriceKobo: i.unitPriceKobo,
              })),
            },
            businessId,
            csrfToken,
          );
        } else if (action.payload.type === 'remove_item') {
          await removeBillItem(
            resolvedBillId,
            {
              idempotencyKey: action.idempotencyKey,
              occurredAt: action.occurredAt,
              variantId: action.payload.variantId,
              quantity: action.payload.quantity,
              unitPriceKobo: action.payload.unitPriceKobo,
            },
            businessId,
            csrfToken,
          );
        } else if (action.payload.type === 'record_payment') {
          await recordPayment(
            resolvedBillId,
            action.payload.amountKobo,
            action.payload.method,
            businessId,
            csrfToken,
            action.idempotencyKey,
          );
        } else if (action.payload.type === 'close') {
          try {
            await closeBill(resolvedBillId, businessId, csrfToken);
          } catch (err) {
            // A replayed close rejected because the bill is already past
            // 'open' is this action's successful outcome, not a failure --
            // confirmed by re-reading the bill rather than guessing from
            // the error alone, since the same 409 also covers a real
            // problem (closing with a balance and no customer attached).
            const current = await getBillDetail(resolvedBillId, businessId);
            if (current.status === 'open') throw err;
          }
        }

        await db.pendingBillActions.delete(action.id);
      } catch (err) {
        if (err instanceof OfflineError) {
          stop = true;
          break;
        }
        await db.pendingBillActions.update(action.id, {
          status: 'failed',
          error: describeActionError(err, 'This action could not be applied.'),
        });
        stop = true;
      }
    }
  }

  return resolvedSessions;
}

// listFailedBillActions returns every queued action the server has
// already rejected for billId -- shown as a "needs attention" banner on
// that bill's own workspace rather than anywhere generic, since only
// someone looking at that specific bill can judge what to do about it.
export async function listFailedBillActions(
  businessId: string,
  billId: string,
): Promise<PendingBillAction[]> {
  return db.pendingBillActions
    .where('businessId')
    .equals(businessId)
    .and((a) => a.status === 'failed' && a.billId === billId)
    .toArray();
}

// discardBillAction removes one stuck, failed action from the queue --
// the manual "I've looked at this, drop it" resolution
// docs/PHASE_UNIFIED_SELL_BILLS.md's local conflict surface relies on,
// since there is no server-side review mechanism for this in v1.
export async function discardBillAction(id: string): Promise<void> {
  await db.pendingBillActions.delete(id);
}
