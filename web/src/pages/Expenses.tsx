import { useEffect, useState } from 'react';
import type { ButtonHTMLAttributes } from 'react';
import { Link } from 'react-router-dom';

import {
  createExpenseCategory,
  listExpenseCategories,
  listExpenses,
  reverseExpense,
  type Expense,
  type ExpenseCategory,
  type PaymentMethod,
} from '../api/expenses';
import { useConnectivity } from '../hooks/useConnectivity';
import { describeActionError } from '../lib/errors';
import { flushPendingExpenses, queuePendingExpense } from '../lib/expensesSync';
import { useSession } from '../lib/session';

function PrimaryButton({ children, ...props }: ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button
      {...props}
      className="w-full rounded-xl bg-jb-ink text-jb-cream text-[15px] font-medium py-4 hover:bg-jb-green transition-colors active:scale-[0.98] disabled:opacity-40 disabled:active:scale-100"
    >
      {children}
    </button>
  );
}

function formatNaira(kobo: number): string {
  return (kobo / 100).toLocaleString();
}

function formatWhen(iso: string): string {
  return new Date(iso).toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  });
}

const PAYMENT_METHODS: { value: PaymentMethod; label: string }[] = [
  { value: 'cash', label: 'Cash' },
  { value: 'transfer', label: 'Transfer' },
  { value: 'card', label: 'Card' },
];

// Records ordinary expenses (docs/PHASE_EXPENSES_DASHBOARD_ACTIVITY_
// REPORTS.md) -- offline-first, mirroring Sell.tsx/Stock.tsx's
// queue-before-network discipline exactly, since expenses:record is
// offline-safe.
export function Expenses() {
  const { csrfToken, selectedBusinessId, memberships } = useSession();
  const isOwner = memberships.find((m) => m.businessId === selectedBusinessId)?.role === 'owner';
  const isOnline = useConnectivity();

  const [categories, setCategories] = useState<ExpenseCategory[] | null>(null);
  const [categoriesError, setCategoriesError] = useState<string | null>(null);
  const [newCategoryName, setNewCategoryName] = useState('');
  const [addingCategory, setAddingCategory] = useState(false);

  const [categoryId, setCategoryId] = useState<string | null>(null);
  const [description, setDescription] = useState('');
  const [amountNaira, setAmountNaira] = useState('');
  const [paymentMethod, setPaymentMethod] = useState<PaymentMethod>('cash');
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [done, setDone] = useState<{ description: string; queuedOffline: boolean } | null>(null);

  const [history, setHistory] = useState<Expense[] | null>(null);
  const [reversingId, setReversingId] = useState<string | null>(null);
  const [reverseError, setReverseError] = useState<string | null>(null);

  // A brand-new business has zero expense categories, which used to leave
  // the picker empty with no obvious next step -- "General" is
  // auto-provisioned the first time this loads empty, so there's always
  // at least one immediately-selectable option (and it's pre-selected,
  // since a single option is never ambiguous to default to). A business
  // that already has categories is never touched.
  useEffect(() => {
    if (!selectedBusinessId || !csrfToken) return;
    let cancelled = false;
    listExpenseCategories(selectedBusinessId)
      .then(async (list) => {
        if (cancelled) return;
        if (list.length === 0) {
          try {
            const created = await createExpenseCategory('General', selectedBusinessId, csrfToken);
            if (cancelled) return;
            list = [created];
          } catch {
            // Another request may have just created one (or we're
            // offline) -- fall back to whatever's there; the user can
            // still type their own category name below.
          }
        }
        if (cancelled) return;
        setCategories(list);
        const onlyCategory = list.length === 1 ? list[0] : undefined;
        if (onlyCategory) setCategoryId((prev) => prev ?? onlyCategory.id);
      })
      .catch((err: unknown) =>
        setCategoriesError(describeActionError(err, 'Could not load categories.')),
      );
    return () => {
      cancelled = true;
    };
  }, [selectedBusinessId, csrfToken]);

  useEffect(() => {
    if (!selectedBusinessId || !isOwner) return;
    // Owner-only (activity:read) -- Staff can record expenses but can't
    // see this history, same shape as sales listing's own capability
    // tier. Silently skipped for Staff rather than shown as an error.
    listExpenses(selectedBusinessId, 20)
      .then(setHistory)
      .catch(() => {
        /* Staff without activity:read simply sees no history section */
      });
  }, [selectedBusinessId, isOwner]);

  useEffect(() => {
    if (!isOnline || !selectedBusinessId || !csrfToken) return;
    void flushPendingExpenses(selectedBusinessId, csrfToken);
  }, [isOnline, selectedBusinessId, csrfToken]);

  async function handleAddCategory() {
    if (!selectedBusinessId || !csrfToken || !newCategoryName.trim()) return;
    setAddingCategory(true);
    setCategoriesError(null);
    try {
      const created = await createExpenseCategory(
        newCategoryName.trim(),
        selectedBusinessId,
        csrfToken,
      );
      setCategories((prev) => [...(prev ?? []), created]);
      setCategoryId(created.id);
      setNewCategoryName('');
    } catch (err) {
      setCategoriesError(describeActionError(err, 'Could not add this category.'));
    } finally {
      setAddingCategory(false);
    }
  }

  const amountKobo = Math.round((Number(amountNaira) || 0) * 100);
  // If no category is picked but the user has typed a new one, treat that
  // as intent -- the primary button creates it and records the expense in
  // one step, rather than leaving a silently-disabled button when someone
  // types a category name and forgets the separate "Add" tap. Creating a
  // category is an online-only action (same as it always was via "Add"),
  // so this path is only available while online.
  const pendingCategoryName = categoryId === null ? newCategoryName.trim() : '';
  const canCreateCategoryInline = pendingCategoryName !== '' && isOnline && !!csrfToken;
  const hasUsableCategory = categoryId !== null || canCreateCategoryInline;
  const canSubmit =
    hasUsableCategory && description.trim() !== '' && amountNaira.trim() !== '' && amountKobo > 0;

  function recordButtonLabel(): string {
    if (submitting) return 'Saving…';
    if (categoryId === null) {
      if (pendingCategoryName) {
        return canCreateCategoryInline
          ? `Add "${pendingCategoryName}" & record expense`
          : 'Connect to the internet to add this category';
      }
      return 'Add a category to continue';
    }
    return 'Record expense';
  }

  async function submitExpense() {
    if (!selectedBusinessId || !canSubmit) return;
    setSubmitting(true);
    setSubmitError(null);
    try {
      let resolvedCategoryId = categoryId;
      if (!resolvedCategoryId && pendingCategoryName && csrfToken) {
        try {
          const created = await createExpenseCategory(
            pendingCategoryName,
            selectedBusinessId,
            csrfToken,
          );
          setCategories((prev) => [...(prev ?? []), created]);
          setNewCategoryName('');
          resolvedCategoryId = created.id;
        } catch (err) {
          setSubmitError(describeActionError(err, 'Could not add this category.'));
          return;
        }
      }
      if (!resolvedCategoryId) return;

      const idempotencyKey = crypto.randomUUID();
      const occurredAt = new Date().toISOString();
      // Written to IndexedDB first, before any network call -- same
      // "this is what makes it true offline" rule Sell.tsx/Stock.tsx
      // already established.
      await queuePendingExpense({
        idempotencyKey,
        businessId: selectedBusinessId,
        categoryId: resolvedCategoryId,
        description: description.trim(),
        amountKobo,
        paymentMethod,
        occurredAt,
        status: 'pending',
        createdAt: occurredAt,
      });
      setDone({ description: description.trim(), queuedOffline: !isOnline || !csrfToken });
      if (isOnline && csrfToken) {
        void flushPendingExpenses(selectedBusinessId, csrfToken).then(() => {
          if (isOwner && selectedBusinessId) {
            listExpenses(selectedBusinessId, 20)
              .then(setHistory)
              .catch(() => {
                /* non-fatal */
              });
          }
        });
      }
    } finally {
      setSubmitting(false);
    }
  }

  async function handleReverse(id: string) {
    if (!selectedBusinessId || !csrfToken) return;
    setReversingId(id);
    setReverseError(null);
    try {
      await reverseExpense(id, selectedBusinessId, csrfToken);
      const refreshed = await listExpenses(selectedBusinessId, 20);
      setHistory(refreshed);
    } catch (err) {
      setReverseError(describeActionError(err, 'Could not reverse this expense.'));
    } finally {
      setReversingId(null);
    }
  }

  function reset() {
    setCategoryId(null);
    setDescription('');
    setAmountNaira('');
    setPaymentMethod('cash');
    setSubmitError(null);
    setDone(null);
  }

  return (
    <div className="min-h-screen bg-jb-cream text-jb-ink pb-16">
      <div className="max-w-md mx-auto px-5 pt-6">
        <div className="flex items-center gap-2.5 mb-6">
          <Link
            to="/dashboard"
            aria-label="Back to dashboard"
            className="text-jb-ink/40 hover:text-jb-ink text-lg leading-none"
          >
            ←
          </Link>
          <h1 className="text-lg font-medium">Expenses</h1>
        </div>

        {done ? (
          <div className="text-center py-10">
            <div className="w-14 h-14 rounded-full bg-jb-ink text-jb-cream flex items-center justify-center text-xl mb-5 mx-auto">
              ✓
            </div>
            <h2 className="text-lg font-medium mb-1">Recorded</h2>
            <p className="text-[13px] text-jb-ink/45 mb-1">{done.description}</p>
            {done.queuedOffline && (
              <p className="text-[12.5px] text-jb-ink/40 mb-8">
                Saved on this device — will send once you're back online.
              </p>
            )}
            <div className="flex flex-col gap-2.5 mt-8">
              <PrimaryButton onClick={reset}>Record another</PrimaryButton>
              <Link to="/dashboard" className="text-[13px] text-jb-ink/40 py-2">
                Done for now
              </Link>
            </div>
          </div>
        ) : (
          <div>
            {categoriesError && <p className="text-[13px] text-red-700 mb-3">{categoriesError}</p>}
            <div className="mb-4">
              <span className="block text-[13px] font-medium text-jb-ink/70 mb-1.5">Category</span>
              <div className="flex flex-wrap gap-2 mb-2">
                {(categories ?? []).map((c) => (
                  <button
                    key={c.id}
                    onClick={() => setCategoryId(c.id)}
                    className={`px-3.5 py-2 rounded-full text-[13px] border transition-colors ${
                      categoryId === c.id
                        ? 'bg-jb-ink text-jb-cream border-jb-ink'
                        : 'border-jb-ink/15 text-jb-ink/60 bg-white'
                    }`}
                  >
                    {c.name}
                  </button>
                ))}
              </div>
              <div className="flex gap-2">
                <input
                  type="text"
                  placeholder="New category…"
                  value={newCategoryName}
                  onChange={(e) => setNewCategoryName(e.target.value)}
                  className="flex-1 rounded-lg border border-jb-ink/15 bg-white px-3 py-2 text-[13px] text-jb-ink placeholder:text-jb-ink/30 focus:outline-none focus:border-jb-ink/40 transition-colors"
                />
                <button
                  onClick={() => void handleAddCategory()}
                  disabled={addingCategory || !newCategoryName.trim()}
                  className="text-[13px] text-jb-ink/60 hover:text-jb-ink transition-colors disabled:opacity-40 px-2"
                >
                  {addingCategory ? 'Adding…' : 'Add'}
                </button>
              </div>
            </div>

            <label className="block mb-4">
              <span className="block text-[13px] font-medium text-jb-ink/70 mb-1.5">
                Description
              </span>
              <input
                type="text"
                placeholder="e.g. Ice for the weekend"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                className="w-full rounded-xl border border-jb-ink/15 bg-white px-4 py-3.5 text-[15px] text-jb-ink focus:outline-none focus:border-jb-ink/40 transition-colors"
              />
            </label>

            <label className="block mb-4">
              <span className="block text-[13px] font-medium text-jb-ink/70 mb-1.5">
                Amount (₦)
              </span>
              <input
                type="number"
                inputMode="numeric"
                placeholder="0"
                value={amountNaira}
                onChange={(e) => setAmountNaira(e.target.value)}
                className="w-full rounded-xl border border-jb-ink/15 bg-white px-4 py-3.5 text-[15px] text-jb-ink focus:outline-none focus:border-jb-ink/40 transition-colors"
              />
            </label>

            <div className="mb-6">
              <span className="block text-[13px] font-medium text-jb-ink/70 mb-1.5">
                Payment method
              </span>
              <div className="flex gap-2 p-1 rounded-xl bg-jb-ink/[0.05]">
                {PAYMENT_METHODS.map((m) => (
                  <button
                    key={m.value}
                    onClick={() => setPaymentMethod(m.value)}
                    className={`flex-1 rounded-lg py-2.5 text-[13px] font-medium transition-colors ${
                      paymentMethod === m.value
                        ? 'bg-white text-jb-ink shadow-sm'
                        : 'text-jb-ink/45'
                    }`}
                  >
                    {m.label}
                  </button>
                ))}
              </div>
            </div>

            {submitError && (
              <p role="alert" className="text-[13px] text-red-700 mb-2">
                {submitError}
              </p>
            )}
            <PrimaryButton onClick={() => void submitExpense()} disabled={submitting || !canSubmit}>
              {recordButtonLabel()}
            </PrimaryButton>

            {isOwner && (
              <div className="mt-8">
                <div className="text-[11px] font-medium text-jb-ink/40 uppercase tracking-wide mb-2">
                  Recent expenses
                </div>
                {reverseError && <p className="text-[13px] text-red-700 mb-2">{reverseError}</p>}
                <div className="rounded-xl border border-jb-ink/10 bg-white/60 divide-y divide-jb-ink/[0.06] overflow-hidden">
                  {history === null && (
                    <p className="text-[13px] text-jb-ink/40 px-4 py-3.5">Loading…</p>
                  )}
                  {history !== null && history.length === 0 && (
                    <p className="text-[13px] text-jb-ink/40 px-4 py-3.5">
                      No expenses recorded yet.
                    </p>
                  )}
                  {(() => {
                    const reversedIds = new Set(
                      (history ?? []).filter((e) => e.reversalOf).map((e) => e.reversalOf),
                    );
                    return (history ?? []).map((e) => (
                      <div
                        key={e.id}
                        className="px-4 py-3.5 flex items-center justify-between gap-3"
                      >
                        <div className="min-w-0">
                          <div className="text-[13px] text-jb-ink/80">{e.description}</div>
                          <div className="text-[11px] text-jb-ink/40 mt-0.5">
                            {e.categoryName} · {formatWhen(e.occurredAt)}
                            {e.reversalOf && ' · reversal'}
                          </div>
                        </div>
                        <div className="flex items-center gap-3 shrink-0">
                          <div
                            className={`text-[13px] font-medium ${
                              e.amountKobo < 0 ? 'text-jb-green-dark' : 'text-jb-ink/80'
                            }`}
                          >
                            {e.amountKobo < 0 ? '+' : '−'}₦{formatNaira(Math.abs(e.amountKobo))}
                          </div>
                          {!e.reversalOf && !reversedIds.has(e.id) && (
                            <button
                              onClick={() => void handleReverse(e.id)}
                              disabled={reversingId === e.id}
                              className="text-[12px] text-jb-ink/40 hover:text-jb-ink transition-colors disabled:opacity-40"
                            >
                              {reversingId === e.id ? '…' : 'Reverse'}
                            </button>
                          )}
                        </div>
                      </div>
                    ));
                  })()}
                </div>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
