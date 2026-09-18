import { apiRequest } from './client';

export type PaymentMethod = 'cash' | 'transfer' | 'card';

export interface SaleItemInput {
  variantId: string;
  quantity: number;
  unitPriceKobo: number;
}

export interface CreateSaleInput {
  idempotencyKey: string;
  occurredAt: string; // RFC 3339
  items: SaleItemInput[];
  payment: { amountKobo: number; method: PaymentMethod };
}

export interface SaleItem {
  variantId: string;
  description: string;
  quantity: number;
  unitPriceKobo: number;
  lineTotalKobo: number;
}

export interface Sale {
  id: string;
  occurredAt: string;
  receivedAt: string;
  totalKobo: number;
  reversalOf: string | null;
  items: SaleItem[];
  payment: { amountKobo: number; method: string };
  // Whoever recorded this sale -- "" if the lookup somehow came back empty;
  // never absent. Dashboard.tsx shows it in the recent-activity feed so a
  // multi-staff shift can tell whose sale is whose.
  sellerName: string;
}

interface RawSaleItem {
  variant_id: string;
  description: string;
  quantity: number;
  unit_price_kobo: number;
  line_total_kobo: number;
}

interface RawSale {
  id: string;
  occurred_at: string;
  received_at: string;
  total_kobo: number;
  reversal_of?: string;
  items?: RawSaleItem[];
  payment: { amount_kobo: number; method: string };
  seller_name?: string;
}

function toSale(raw: RawSale): Sale {
  return {
    id: raw.id,
    occurredAt: raw.occurred_at,
    receivedAt: raw.received_at,
    totalKobo: raw.total_kobo,
    reversalOf: raw.reversal_of ?? null,
    items: (raw.items ?? []).map((i) => ({
      variantId: i.variant_id,
      description: i.description,
      quantity: i.quantity,
      unitPriceKobo: i.unit_price_kobo,
      lineTotalKobo: i.line_total_kobo,
    })),
    payment: { amountKobo: raw.payment.amount_kobo, method: raw.payment.method },
    sellerName: raw.seller_name ?? '',
  };
}

// createSale wraps POST /api/v1/sales. idempotencyKey makes a retried call
// (e.g. after a lost response while offline) safe -- the server returns
// the sale already posted the first time rather than creating a second
// one.
export async function createSale(
  input: CreateSaleInput,
  businessId: string,
  csrfToken: string,
): Promise<Sale> {
  const raw = await apiRequest<RawSale>('/api/v1/sales', {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken, 'X-Business-ID': businessId },
    body: JSON.stringify({
      idempotency_key: input.idempotencyKey,
      occurred_at: input.occurredAt,
      items: input.items.map((i) => ({
        variant_id: i.variantId,
        quantity: i.quantity,
        unit_price_kobo: i.unitPriceKobo,
      })),
      payment: { amount_kobo: input.payment.amountKobo, method: input.payment.method },
    }),
  });
  return toSale(raw);
}

// listSales wraps GET /api/v1/sales -- most recent first.
export async function listSales(businessId: string, limit = 20): Promise<Sale[]> {
  const raw = await apiRequest<RawSale[]>(`/api/v1/sales?limit=${limit}`, {
    headers: { 'X-Business-ID': businessId },
  });
  return raw.map(toSale);
}

// salesSummary wraps GET /api/v1/sales/summary -- "today" is computed
// server-side in the business's own timezone, not the browser's.
// todaySaleCount excludes reversals (a correction isn't "one more sale").
export async function salesSummary(
  businessId: string,
): Promise<{ todayTotalKobo: number; todaySaleCount: number }> {
  const raw = await apiRequest<{ today_total_kobo: number; today_sale_count: number }>(
    '/api/v1/sales/summary',
    {
      headers: { 'X-Business-ID': businessId },
    },
  );
  return { todayTotalKobo: raw.today_total_kobo, todaySaleCount: raw.today_sale_count };
}
