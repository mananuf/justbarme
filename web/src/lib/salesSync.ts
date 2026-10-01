import { createSale, type Sale } from '../api/sales';
import { db, type PendingSale } from './db';

export async function queuePendingSale(sale: PendingSale): Promise<void> {
  await db.pendingSales.put(sale);
}

// trySyncPendingSale attempts to post one already-queued sale right away
// (called immediately after queuePendingSale, while the device is believed
// online) and returns the posted Sale on success, or null if it couldn't
// be posted -- in which case the row is left pending for the ordinary
// flushPendingSales path to pick up later, exactly as if this call had
// never been attempted. This exists purely so the unified Sell flow
// (docs/PHASE_UNIFIED_SELL_BILLS.md) can navigate straight to the real
// bill a walk-in sale posts against when the network cooperates, without
// weakening the "written to IndexedDB before any network call" guarantee
// queuePendingSale's caller already satisfies.
export async function trySyncPendingSale(
  sale: PendingSale,
  businessId: string,
  csrfToken: string,
): Promise<Sale | null> {
  try {
    const posted = await createSale(
      {
        idempotencyKey: sale.idempotencyKey,
        occurredAt: sale.occurredAt,
        items: sale.items.map((i) => ({
          variantId: i.variantId,
          quantity: i.quantity,
          unitPriceKobo: i.unitPriceKobo,
        })),
        payment: sale.payment,
      },
      businessId,
      csrfToken,
    );
    await db.pendingSales.delete(sale.idempotencyKey);
    return posted;
  } catch {
    return null;
  }
}

// flushPendingSales attempts to post every still-pending sale for
// businessId to the server, oldest first, stopping at the first failure --
// a genuinely offline device shouldn't spend a request per queued sale on
// every retry attempt. The next flush (app load, regained connectivity)
// picks up where this one left off. Each row's idempotencyKey is what
// makes this safe to call repeatedly: a sale already posted successfully
// is removed from the queue, never re-sent.
export async function flushPendingSales(businessId: string, csrfToken: string): Promise<void> {
  const pending = await db.pendingSales
    .where('businessId')
    .equals(businessId)
    .and((s) => s.status === 'pending')
    .sortBy('createdAt');

  for (const sale of pending) {
    try {
      await createSale(
        {
          idempotencyKey: sale.idempotencyKey,
          occurredAt: sale.occurredAt,
          items: sale.items.map((i) => ({
            variantId: i.variantId,
            quantity: i.quantity,
            unitPriceKobo: i.unitPriceKobo,
          })),
          payment: sale.payment,
        },
        businessId,
        csrfToken,
      );
      await db.pendingSales.delete(sale.idempotencyKey);
    } catch {
      return;
    }
  }
}
