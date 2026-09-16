import { apiRequest } from './client';

export interface ReceiveStockLine {
  variantId: string;
  quantity: number;
  totalCostKobo: number;
}

export interface ReceiptLineResult {
  variantId: string;
  quantity: number;
  totalCostKobo: number;
  newBalance: number;
}

export interface Receipt {
  id: string;
  receivedAt: string;
  lines: ReceiptLineResult[];
}

interface RawReceiptLine {
  variant_id: string;
  quantity: number;
  total_cost_kobo: number;
  new_balance: number;
}

interface RawReceipt {
  id: string;
  received_at: string;
  lines: RawReceiptLine[];
}

// receiveStock wraps POST /api/v1/stock-receipts. The server resolves the
// receiving location itself (the business's default location) -- the
// client never sends one, since multi-location isn't a pilot concern (see
// docs/PHASE_STOCK_RECEIVING.md).
export async function receiveStock(
  lines: ReceiveStockLine[],
  businessId: string,
  csrfToken: string,
): Promise<Receipt> {
  const raw = await apiRequest<RawReceipt>('/api/v1/stock-receipts', {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken, 'X-Business-ID': businessId },
    body: JSON.stringify({
      lines: lines.map((l) => ({
        variant_id: l.variantId,
        quantity: l.quantity,
        total_cost_kobo: l.totalCostKobo,
      })),
    }),
  });
  return {
    id: raw.id,
    receivedAt: raw.received_at,
    lines: raw.lines.map((l) => ({
      variantId: l.variant_id,
      quantity: l.quantity,
      totalCostKobo: l.total_cost_kobo,
      newBalance: l.new_balance,
    })),
  };
}
