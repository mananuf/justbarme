import { apiRequest } from './client';
import type { PlatformBusiness } from './platform';

// Typed client for docs/PHASE_PLATFORM_ADMIN_DEEP_DRILL.md's full
// line-item read access and superadmin-only corrective actions. Every
// response here mirrors the exact shape the business's own (non-platform)
// endpoints already return -- these are the same response builders, just
// reached through the platform-scoped route instead.

export interface SaleItem {
  variant_id: string;
  description: string;
  quantity: number;
  unit_price_kobo: number;
  line_total_kobo: number;
}

export interface Payment {
  amount_kobo: number;
  method: string;
}

export interface Review {
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

export interface Sale {
  id: string;
  occurred_at: string;
  received_at: string;
  total_kobo: number;
  reversal_of?: string;
  items?: SaleItem[];
  payment: Payment;
  reviews?: Review[];
  seller_id: string;
  bill_id: string;
  seller_name?: string;
  other_sales_on_bill?: boolean;
}

export interface Expense {
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

export interface WriteOff {
  id: string;
  amount_kobo: number;
  reason: string;
  created_at: string;
}

export interface Bill {
  id: string;
  status: string;
  table_id?: string;
  customer_id?: string;
  balance_kobo: number;
  opened_at: string;
}

export interface BillDetail extends Bill {
  sales?: Sale[];
  payments?: Payment[];
  write_offs?: WriteOff[];
}

export interface StockReceiptLine {
  variant_id: string;
  variant_name: string;
  product_name: string;
  quantity: number;
  total_cost_kobo: number;
}

export interface StockReceipt {
  id: string;
  received_by: string;
  received_at: string;
  reversal_of?: string;
  reversal_of_this?: string;
  lines: StockReceiptLine[];
}

export interface AdjustmentRequest {
  id: string;
  variant_id: string;
  variant_name?: string;
  product_name?: string;
  quantity_delta: number;
  reason_category: string;
  reason_note: string;
  status: string;
  requested_by_name?: string;
  created_at: string;
  decided_by_name?: string;
  resolution_note?: string;
  decided_at?: string;
}

export interface InventoryReview {
  id: string;
  type: string;
  variant_id: string;
  variant_name?: string;
  product_name?: string;
  status: string;
  created_at: string;
  count_expected_quantity?: number;
  count_physical_quantity?: number;
}

export interface HistoryEntry {
  id: string;
  quantity_delta: number;
  created_at: string;
  event_type: string;
  actor_name?: string;
  adjustment_reason_category?: string;
  adjustment_reason_note?: string;
  adjustment_decided_by_name?: string;
}

export interface ActivityEntry {
  id: string;
  type: string;
  actor_id: string;
  actor_name?: string;
  occurred_at: string;
  summary: string;
  amount_kobo: number;
}

export interface Member {
  name: string;
  role: string;
  joined_at: string;
}

const base = (businessId: string) => `/api/v1/platform/businesses/${businessId}`;

function reasonBody(reason: string) {
  return JSON.stringify({ reason });
}

function postWithReason<T>(path: string, reason: string, csrfToken: string): Promise<T> {
  return apiRequest<T>(path, {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken },
    body: reasonBody(reason),
  });
}

export function listPlatformMembers(businessId: string): Promise<Member[]> {
  return apiRequest<Member[]>(`${base(businessId)}/members`);
}

export function listPlatformActivityFeed(businessId: string): Promise<ActivityEntry[]> {
  return apiRequest<ActivityEntry[]>(`${base(businessId)}/activity/feed`);
}

export function listPlatformSales(businessId: string): Promise<Sale[]> {
  return apiRequest<Sale[]>(`${base(businessId)}/sales`);
}

export function getPlatformSale(businessId: string, saleId: string): Promise<Sale> {
  return apiRequest<Sale>(`${base(businessId)}/sales/${saleId}`);
}

export function reversePlatformSale(
  businessId: string,
  saleId: string,
  reason: string,
  csrfToken: string,
): Promise<Sale> {
  return postWithReason<Sale>(`${base(businessId)}/sales/${saleId}/reverse`, reason, csrfToken);
}

export function listPlatformExpenses(businessId: string): Promise<Expense[]> {
  return apiRequest<Expense[]>(`${base(businessId)}/expenses`);
}

export function reversePlatformExpense(
  businessId: string,
  expenseId: string,
  reason: string,
  csrfToken: string,
): Promise<Expense> {
  return postWithReason<Expense>(
    `${base(businessId)}/expenses/${expenseId}/reverse`,
    reason,
    csrfToken,
  );
}

export function listPlatformBills(businessId: string, status?: string): Promise<Bill[]> {
  const qs = status ? `?status=${encodeURIComponent(status)}` : '';
  return apiRequest<Bill[]>(`${base(businessId)}/bills${qs}`);
}

export function getPlatformBillDetail(businessId: string, billId: string): Promise<BillDetail> {
  return apiRequest<BillDetail>(`${base(businessId)}/bills/${billId}`);
}

export function closePlatformBill(
  businessId: string,
  billId: string,
  reason: string,
  csrfToken: string,
): Promise<Bill> {
  return postWithReason<Bill>(`${base(businessId)}/bills/${billId}/close`, reason, csrfToken);
}

export function voidPlatformBill(
  businessId: string,
  billId: string,
  reason: string,
  csrfToken: string,
): Promise<Bill> {
  return postWithReason<Bill>(`${base(businessId)}/bills/${billId}/void`, reason, csrfToken);
}

export function getPlatformStockReceipt(
  businessId: string,
  receiptId: string,
): Promise<StockReceipt> {
  return apiRequest<StockReceipt>(`${base(businessId)}/stock-receipts/${receiptId}`);
}

export function reversePlatformStockReceipt(
  businessId: string,
  receiptId: string,
  reason: string,
  csrfToken: string,
): Promise<StockReceipt> {
  return postWithReason<StockReceipt>(
    `${base(businessId)}/stock-receipts/${receiptId}/reverse`,
    reason,
    csrfToken,
  );
}

export function forceReversePlatformStockReceipt(
  businessId: string,
  receiptId: string,
  reason: string,
  csrfToken: string,
): Promise<StockReceipt> {
  return postWithReason<StockReceipt>(
    `${base(businessId)}/stock-receipts/${receiptId}/force-reverse`,
    reason,
    csrfToken,
  );
}

export function listPlatformAdjustmentRequests(businessId: string): Promise<AdjustmentRequest[]> {
  return apiRequest<AdjustmentRequest[]>(`${base(businessId)}/inventory-adjustments`);
}

export function approvePlatformAdjustmentRequest(
  businessId: string,
  requestId: string,
  reason: string,
  csrfToken: string,
): Promise<AdjustmentRequest> {
  return postWithReason<AdjustmentRequest>(
    `${base(businessId)}/inventory-adjustments/${requestId}/approve`,
    reason,
    csrfToken,
  );
}

export function rejectPlatformAdjustmentRequest(
  businessId: string,
  requestId: string,
  reason: string,
  csrfToken: string,
): Promise<AdjustmentRequest> {
  return postWithReason<AdjustmentRequest>(
    `${base(businessId)}/inventory-adjustments/${requestId}/reject`,
    reason,
    csrfToken,
  );
}

export function getPlatformStockHistory(
  businessId: string,
  variantId: string,
): Promise<HistoryEntry[]> {
  return apiRequest<HistoryEntry[]>(`${base(businessId)}/stock/${variantId}/history`);
}

export function correctPlatformBalance(
  businessId: string,
  variantId: string,
  targetQuantity: number,
  reason: string,
  csrfToken: string,
): Promise<AdjustmentRequest> {
  return apiRequest<AdjustmentRequest>(`${base(businessId)}/stock/${variantId}/correct-balance`, {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken },
    body: JSON.stringify({ target_quantity: targetQuantity, reason }),
  });
}

export function listPlatformSaleReviews(businessId: string): Promise<Review[]> {
  return apiRequest<Review[]>(`${base(businessId)}/sale-reviews`);
}

export function resolvePlatformSaleReview(
  businessId: string,
  reviewId: string,
  reason: string,
  csrfToken: string,
): Promise<Review> {
  return postWithReason<Review>(
    `${base(businessId)}/sale-reviews/${reviewId}/resolve`,
    reason,
    csrfToken,
  );
}

export function listPlatformInventoryReviews(businessId: string): Promise<InventoryReview[]> {
  return apiRequest<InventoryReview[]>(`${base(businessId)}/inventory-reviews`);
}

export function resolvePlatformInventoryReview(
  businessId: string,
  reviewId: string,
  reason: string,
  csrfToken: string,
): Promise<InventoryReview> {
  return postWithReason<InventoryReview>(
    `${base(businessId)}/inventory-reviews/${reviewId}/resolve`,
    reason,
    csrfToken,
  );
}

export type { PlatformBusiness };
