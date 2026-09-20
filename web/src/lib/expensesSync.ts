import { recordExpense } from '../api/expenses';
import { db, type PendingExpense } from './db';

export async function queuePendingExpense(expense: PendingExpense): Promise<void> {
  await db.pendingExpenses.put(expense);
}

// flushPendingExpenses mirrors flushPendingSales/flushPendingStockCounts
// exactly -- oldest first, stop at the first failure, rely on
// idempotencyKey to make retrying safe.
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
    } catch {
      return;
    }
  }
}
