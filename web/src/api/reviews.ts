import { apiRequest } from './client';

export type ReviewReason = 'deactivated_variant' | 'price_mismatch';

export interface SaleReview {
  id: string;
  saleId: string;
  saleItemId: string;
  reason: ReviewReason;
  status: 'open' | 'resolved';
  saleOccurredAt: string;
  sellerName: string;
  itemDescription: string;
  itemQuantity: number;
  itemUnitPriceKobo: number;
  itemLineTotalKobo: number;
}

interface RawSaleReview {
  id: string;
  sale_id: string;
  sale_item_id: string;
  reason: string;
  status: string;
  sale_occurred_at?: string;
  seller_name?: string;
  item_description?: string;
  item_quantity?: number;
  item_unit_price_kobo?: number;
  item_line_total_kobo?: number;
}

function toSaleReview(raw: RawSaleReview): SaleReview {
  return {
    id: raw.id,
    saleId: raw.sale_id,
    saleItemId: raw.sale_item_id,
    reason: raw.reason as ReviewReason,
    status: raw.status as 'open' | 'resolved',
    saleOccurredAt: raw.sale_occurred_at ?? '',
    sellerName: raw.seller_name ?? '',
    itemDescription: raw.item_description ?? '',
    itemQuantity: raw.item_quantity ?? 0,
    itemUnitPriceKobo: raw.item_unit_price_kobo ?? 0,
    itemLineTotalKobo: raw.item_line_total_kobo ?? 0,
  };
}

// listSaleReviews wraps GET /api/v1/sale-reviews -- owner-only
// (activity:read). Open reviews only; there is no separate endpoint for
// resolved ones (see docs/PHASE_OFFLINE_CATALOGUE_AND_REVIEWS.md).
export async function listSaleReviews(businessId: string): Promise<SaleReview[]> {
  const raw = await apiRequest<RawSaleReview[]>('/api/v1/sale-reviews', {
    headers: { 'X-Business-ID': businessId },
  });
  return raw.map(toSaleReview);
}

// resolveSaleReview wraps POST /api/v1/sale-reviews/{id}/resolve --
// owner-only (sales:reverse). A note is required; resolving never changes
// the underlying sale, it only acknowledges the flag.
export async function resolveSaleReview(
  reviewId: string,
  note: string,
  businessId: string,
  csrfToken: string,
): Promise<SaleReview> {
  const raw = await apiRequest<RawSaleReview>(`/api/v1/sale-reviews/${reviewId}/resolve`, {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken, 'X-Business-ID': businessId },
    body: JSON.stringify({ note }),
  });
  return toSaleReview(raw);
}
