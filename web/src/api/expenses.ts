import { apiRequest } from './client';

export type PaymentMethod = 'cash' | 'transfer' | 'card';

export interface ExpenseCategory {
  id: string;
  name: string;
  active: boolean;
}

interface RawExpenseCategory {
  id: string;
  name: string;
  active: boolean;
}

function toCategory(raw: RawExpenseCategory): ExpenseCategory {
  return { id: raw.id, name: raw.name, active: raw.active };
}

// listExpenseCategories wraps GET /api/v1/expense-categories
// (expenses:record -- anyone who can record an expense needs this list).
export async function listExpenseCategories(businessId: string): Promise<ExpenseCategory[]> {
  const raw = await apiRequest<RawExpenseCategory[]>('/api/v1/expense-categories', {
    headers: { 'X-Business-ID': businessId },
  });
  return raw.map(toCategory);
}

// createExpenseCategory wraps POST /api/v1/expense-categories.
export async function createExpenseCategory(
  name: string,
  businessId: string,
  csrfToken: string,
): Promise<ExpenseCategory> {
  const raw = await apiRequest<RawExpenseCategory>('/api/v1/expense-categories', {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken, 'X-Business-ID': businessId },
    body: JSON.stringify({ name }),
  });
  return toCategory(raw);
}

export interface Expense {
  id: string;
  categoryId: string;
  categoryName: string;
  description: string;
  amountKobo: number;
  paymentMethod: PaymentMethod;
  recordedBy: string;
  reversalOf: string | null;
  occurredAt: string;
}

interface RawExpense {
  id: string;
  category_id: string;
  category_name?: string;
  description: string;
  amount_kobo: number;
  payment_method: string;
  recorded_by: string;
  reversal_of?: string;
  occurred_at: string;
}

function toExpense(raw: RawExpense): Expense {
  return {
    id: raw.id,
    categoryId: raw.category_id,
    categoryName: raw.category_name ?? '',
    description: raw.description,
    amountKobo: raw.amount_kobo,
    paymentMethod: raw.payment_method as PaymentMethod,
    recordedBy: raw.recorded_by,
    reversalOf: raw.reversal_of ?? null,
    occurredAt: raw.occurred_at,
  };
}

// recordExpense wraps POST /api/v1/expenses (expenses:record,
// offline-safe). idempotencyKey makes a queued-offline submission safe to
// retry -- a resubmitted expense returns the identical result.
export async function recordExpense(
  idempotencyKey: string,
  categoryId: string,
  description: string,
  amountKobo: number,
  paymentMethod: PaymentMethod,
  occurredAt: string,
  businessId: string,
  csrfToken: string,
): Promise<Expense> {
  const raw = await apiRequest<RawExpense>('/api/v1/expenses', {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken, 'X-Business-ID': businessId },
    body: JSON.stringify({
      idempotency_key: idempotencyKey,
      category_id: categoryId,
      description,
      amount_kobo: amountKobo,
      payment_method: paymentMethod,
      occurred_at: occurredAt,
    }),
  });
  return toExpense(raw);
}

// listExpenses wraps GET /api/v1/expenses (activity:read, Owner-only
// today -- same open pilot question as sales listing).
export async function listExpenses(businessId: string, limit = 20): Promise<Expense[]> {
  const raw = await apiRequest<RawExpense[]>(`/api/v1/expenses?limit=${limit}`, {
    headers: { 'X-Business-ID': businessId },
  });
  return raw.map(toExpense);
}

// reverseExpense wraps POST /api/v1/expenses/{id}/reverse
// (expenses:reverse, Owner-only, not offline-safe). Creates a new,
// equal-and-opposite expense; never edits or deletes the original.
export async function reverseExpense(
  expenseId: string,
  businessId: string,
  csrfToken: string,
): Promise<Expense> {
  const raw = await apiRequest<RawExpense>(`/api/v1/expenses/${expenseId}/reverse`, {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken, 'X-Business-ID': businessId },
    body: JSON.stringify({}),
  });
  return toExpense(raw);
}
