import { OfflineError } from '../api/client';
import { recordExpense } from '../api/expenses';
import { describeActionError } from './errors';
import { db, type PendingExpense } from './db';

export async function queuePendingExpense(expense: PendingExpense): Promise<void> {
  await db.pendingExpenses.put(expense);
}

// flushPendingExpenses attempts to post every still-pending expense for
// businessId, oldest first, relying on each row's idempotencyKey to make
// retrying safe. Two outcomes on failure, not one: still offline
// (OfflineError) stops the whole flush here, since a genuinely offline
// device shouldn't spend a request per queued expense on every retry
// attempt, and this one (and everything behind it) is retried next time;
// a real server rejection marks just this one row 'failed' with a plain
// reason and moves on to the next expense, since unrelated queued
// expenses have no dependency on this one and must not be blocked behind
// it forever (docs/PHASE_OFFLINE_QUEUE_FAILURES.md).
export async function flushPendingExpenses(businessId: string, csrfToken: string): Promise<void> {
  const pending = await db.pendingExpenses
    .where('businessId')
    .equals(businessId)
    .and((e) => e.status === 'pending')
    .sortBy('createdAt');

  for (const expense of pending) {
    try {
      await recordExpense(
        expense.idempotencyKey,
        expense.categoryId,
        expense.description,
        expense.amountKobo,
        expense.paymentMethod,
        expense.occurredAt,
        businessId,
        csrfToken,
      );
      await db.pendingExpenses.delete(expense.idempotencyKey);
    } catch (err) {
      if (err instanceof OfflineError) {
        return;
      }
      await db.pendingExpenses.update(expense.idempotencyKey, {
        status: 'failed',
        error: describeActionError(err, 'This expense could not be saved.'),
      });
    }
  }
}

// listFailedExpenses returns every queued expense the server has already
// rejected -- shown on Expenses.tsx so it isn't silently stuck forever
// with nothing on screen explaining why.
export async function listFailedExpenses(businessId: string): Promise<PendingExpense[]> {
  return db.pendingExpenses
    .where('businessId')
    .equals(businessId)
    .and((e) => e.status === 'failed')
    .toArray();
}

// discardPendingExpense removes one stuck, failed expense from the queue
// -- the manual "I've looked at this, drop it" resolution, since there is
// no server-side review mechanism for a rejected offline record.
export async function discardPendingExpense(idempotencyKey: string): Promise<void> {
  await db.pendingExpenses.delete(idempotencyKey);
}
