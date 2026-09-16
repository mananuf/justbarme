import { createSale } from '../api/sales';
import { db, type PendingSale } from './db';

export async function queuePendingSale(sale: PendingSale): Promise<void> {
  await db.pendingSales.put(sale);
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
