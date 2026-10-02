# Offline queue failure handling

**Status:** Built and verified (`npm run check`: format, lint, 57 passing
tests including 12 new ones, production build). **Not** part of `docs/`'s
frozen plan — a targeted bug fix made during a session that validated a
pasted external audit against this codebase's actual code. Branch:
`fix/csrf-and-offline-and-login`.
**Depends on:** `web/src/lib/db.ts` (Dexie schema), and generalizes a
pattern `web/src/lib/billActionsSync.ts` already established for
`pendingBillActions` (`docs/PHASE_UNIFIED_SELL_BILLS.md`'s "needs
attention" design) to the three other offline queues:
`pendingExpenses`, `pendingStockCounts`, `pendingAdjustmentRequests`.

## The bug

Before this fix, `flushPendingExpenses`/`flushPendingStockCounts`/
`flushPendingAdjustmentRequests` each had the same shape:

```ts
for (const item of pending) {
  try {
    await postToServer(item);
    await db.pendingX.delete(item.idempotencyKey);
  } catch {
    return; // <-- the bug
  }
}
```

A bare `catch { return; }` cannot distinguish two completely different
situations:

1. **Still offline** (`OfflineError` — `fetch` itself failed, per
   `src/api/client.ts`). Correct response: stop here, leave everything
   from this point on `'pending'`, retry the whole queue next time.
2. **The server actually rejected this specific item** (e.g. a `409`
   because the variant was deactivated since this was queued, a `422`
   validation failure, a `403` if a capability changed). This item will
   **never** succeed by itself being retried unmodified — but the old
   code retried it anyway, every flush, forever, and — because it
   `return`s on the first error — silently blocked every other queued
   item behind it, even ones for a completely unrelated variant/category
   that would have posted fine.

`pendingSales`/`salesSync.ts` has the identical shape and the identical
bug, but is confirmed dead code (nothing calls `queuePendingSale`/
`flushPendingSales` since `Sell.tsx`'s unification onto
`billActionsSync.ts` — see `docs/PHASE_UNIFIED_SELL_BILLS.md`) and was
deliberately left untouched: fixing an unreachable code path has no
effect, and touching it would be scope creep.

## Confirmed design

Generalizes `pendingBillActions`'s existing `'pending' | 'failed'` +
`error?: string` shape (`db.ts`) to `PendingExpense`,
`PendingStockCount`, and `PendingAdjustmentRequest`. There is
deliberately no `'synced'` status anywhere in this scheme — a successful
sync deletes the row outright, since a synced row is never read back and
there is nothing to keep it for.

- **`OfflineError` stops the flush; a real rejection marks just that one
  row and moves on.** Each flush function now does:
  ```ts
  } catch (err) {
    if (err instanceof OfflineError) {
      return;
    }
    await db.pendingX.update(item.idempotencyKey, {
      status: 'failed',
      error: describeActionError(err, '<fallback text>'),
    });
  }
  ```
  `describeActionError` (`src/lib/errors.ts`) is the same helper every
  other page's catch block already uses to turn a failed mutation into
  user-facing text.
- **One deliberate improvement over `pendingBillActions`'s own
  behavior**: `flushPendingExpenses`/`flushPendingStockCounts`/
  `flushPendingAdjustmentRequests` `continue` past a failed item to the
  next one in the *same* flush call, rather than stopping the session the
  way `billActionsSync.ts` stops a bill's action sequence at its first
  failure. This is correct specifically because these three queues have
  **no ordering dependency between their own rows** — an expense, a
  stock count, or an adjustment request is one atomic action with no
  later row depending on an earlier one succeeding first (unlike a bill's
  `open` → `add_round` → `record_payment` sequence, where a later action
  genuinely can't resolve until an earlier one has). A rejected expense
  must never block a different, unrelated expense behind it.
- **`listFailedExpenses`/`listFailedStockCounts`/
  `listFailedAdjustmentRequests`** and **`discardPendingExpense`/
  `discardPendingStockCount`/`discardPendingAdjustmentRequest`** mirror
  `billActionsSync.ts`'s `listFailedBillActions`/`discardBillAction` —
  there is no server-side review mechanism for a rejected offline record,
  so discarding it from the local queue is the only resolution path,
  same as bill actions.
- **Frontend surfacing**: `Expenses.tsx` gained a "COULDN'T BE SAVED"
  section (above the record form) listing any failed expense with its
  error and a Discard button; `Stock.tsx` gained the equivalent section
  (shown regardless of which of Restock/Count/Report-issue/Pricing mode
  tab is active, since a rejected count or adjustment request isn't tied
  to whichever mode happens to be open when the queue is checked) listing
  both failed counts and failed adjustment requests together, each with
  its own Discard button. Both mirror `BillWorkspace.tsx`'s existing
  pending/failed-action list styling (red-tinted card, inline error text,
  underlined Discard link) for visual consistency across the app's three
  offline-queue surfaces.
- **Each page refreshes its failed-list state after every flush attempt**
  (on mount, on regaining connectivity, and right after submitting a new
  item) — a newly-failed item appears without requiring a manual reload,
  and a newly-fixed server-side condition (e.g. staff capability
  restored) that lets a previously-stuck item's *sibling* submissions
  succeed doesn't retroactively clear an already-failed row (by design —
  only an explicit Discard removes a failed row; the failure was real
  when it happened).

## Tests

New: `web/src/lib/expensesSync.test.ts` and
`web/src/lib/inventorySync.test.ts` (mirroring
`billActionsSync.test.ts`'s mocking pattern — `vi.mock` the relevant
`api/*` module, a real Dexie table via `fake-indexeddb`), covering per
queue:
- Oldest-first posting order, queue emptied on success.
- `OfflineError` stops the flush, leaves pending items untouched.
- A real server rejection (`ApiError`) marks only that row `'failed'`
  with a plain-language error, and the flush continues to post the next,
  unrelated item in the same call.
- `discardPendingX` removes a failed row.

## What this does not change

- `pendingSales`/`salesSync.ts` — confirmed dead code, deliberately left
  as-is.
- Tabs/bills (`pendingBillActions`/`billActionsSync.ts`) already had this
  shape before this fix; its own per-session-halt-on-failure behavior
  (not `continue`) is correct for its own write-ordering constraint and
  was not changed.
