import { apiRequest } from './client';

function rangeParams(startAt: string, endAt: string): string {
  return new URLSearchParams({ start_at: startAt, end_at: endAt }).toString();
}

export interface SalesDay {
  day: string; // YYYY-MM-DD, bucketed in the business's own timezone
  totalKobo: number;
  saleCount: number;
}

interface RawSalesDay {
  day: string;
  total_kobo: number;
  sale_count: number;
}

// reportSales wraps GET /api/v1/reports/sales (reports:read) --
// day-bucketed totals backing the Sales report's heatmap and table.
export async function reportSales(
  businessId: string,
  startAt: string,
  endAt: string,
): Promise<SalesDay[]> {
  const raw = await apiRequest<RawSalesDay[]>(
    `/api/v1/reports/sales?${rangeParams(startAt, endAt)}`,
    {
      headers: { 'X-Business-ID': businessId },
    },
  );
  return raw.map((r) => ({ day: r.day, totalKobo: r.total_kobo, saleCount: r.sale_count }));
}

export interface ProductQuantity {
  variantId: string;
  variantName: string;
  productName: string;
  unitsSold: number;
}

interface RawProductQuantity {
  variant_id: string;
  variant_name: string;
  product_name: string;
  units_sold: number;
}

// reportProducts wraps GET /api/v1/reports/products (reports:read).
export async function reportProducts(
  businessId: string,
  startAt: string,
  endAt: string,
): Promise<ProductQuantity[]> {
  const raw = await apiRequest<RawProductQuantity[]>(
    `/api/v1/reports/products?${rangeParams(startAt, endAt)}`,
    {
      headers: { 'X-Business-ID': businessId },
    },
  );
  return raw.map((r) => ({
    variantId: r.variant_id,
    variantName: r.variant_name,
    productName: r.product_name,
    unitsSold: r.units_sold,
  }));
}

export interface StaffSales {
  sellerId: string;
  sellerName: string;
  totalKobo: number;
  saleCount: number;
}

interface RawStaffSales {
  seller_id: string;
  seller_name?: string;
  total_kobo: number;
  sale_count: number;
}

// reportStaffSales wraps GET /api/v1/reports/staff-sales (reports:read).
export async function reportStaffSales(
  businessId: string,
  startAt: string,
  endAt: string,
): Promise<StaffSales[]> {
  const raw = await apiRequest<RawStaffSales[]>(
    `/api/v1/reports/staff-sales?${rangeParams(startAt, endAt)}`,
    {
      headers: { 'X-Business-ID': businessId },
    },
  );
  return raw.map((r) => ({
    sellerId: r.seller_id,
    sellerName: r.seller_name ?? '',
    totalKobo: r.total_kobo,
    saleCount: r.sale_count,
  }));
}

export interface ExpensesByCategory {
  categoryId: string;
  categoryName: string;
  totalKobo: number;
  expenseCount: number;
}

interface RawExpensesByCategory {
  category_id: string;
  category_name: string;
  total_kobo: number;
  expense_count: number;
}

// reportExpenses wraps GET /api/v1/reports/expenses (reports:read) --
// totals grouped by category, the primary view for this report (a
// heatmap would answer "when," which is less actionable here than
// "what").
export async function reportExpenses(
  businessId: string,
  startAt: string,
  endAt: string,
): Promise<ExpensesByCategory[]> {
  const raw = await apiRequest<RawExpensesByCategory[]>(
    `/api/v1/reports/expenses?${rangeParams(startAt, endAt)}`,
    {
      headers: { 'X-Business-ID': businessId },
    },
  );
  return raw.map((r) => ({
    categoryId: r.category_id,
    categoryName: r.category_name,
    totalKobo: r.total_kobo,
    expenseCount: r.expense_count,
  }));
}

export interface StockDiscrepancy {
  id: string;
  variantId: string;
  variantName: string;
  productName: string;
  expectedQuantity: number;
  physicalQuantity: number;
  variance: number;
  isStale: boolean;
  countedAt: string;
  countedByName: string;
}

interface RawStockDiscrepancy {
  id: string;
  variant_id: string;
  variant_name: string;
  product_name: string;
  expected_quantity: number;
  physical_quantity: number;
  variance: number;
  is_stale: boolean;
  counted_at: string;
  counted_by_name?: string;
}

// reportStock wraps GET /api/v1/reports/stock (reports:read) -- reuses
// Phase 7's own stock-count/variance data directly, no new schema.
export async function reportStock(
  businessId: string,
  startAt: string,
  endAt: string,
): Promise<StockDiscrepancy[]> {
  const raw = await apiRequest<RawStockDiscrepancy[]>(
    `/api/v1/reports/stock?${rangeParams(startAt, endAt)}`,
    {
      headers: { 'X-Business-ID': businessId },
    },
  );
  return raw.map((r) => ({
    id: r.id,
    variantId: r.variant_id,
    variantName: r.variant_name,
    productName: r.product_name,
    expectedQuantity: r.expected_quantity,
    physicalQuantity: r.physical_quantity,
    variance: r.variance,
    isStale: r.is_stale,
    countedAt: r.counted_at,
    countedByName: r.counted_by_name ?? '',
  }));
}

// GrossMargin is one product's revenue/cost picture within a range
// (docs/PHASE_FIFO_COSTING.md §8). unresolvedUnits is the honesty flag
// this report exists to never hide -- a nonzero value means
// grossMarginKobo is a lower bound on the real figure, not the final
// number (some sold units' cost is still unresolved, from a sale that
// oversold before the real stock was ever logged).
export interface GrossMargin {
  variantId: string;
  variantName: string;
  productName: string;
  revenueKobo: number;
  resolvedCogsKobo: number;
  grossMarginKobo: number;
  unresolvedUnits: number;
}

interface RawGrossMargin {
  variant_id: string;
  variant_name: string;
  product_name: string;
  revenue_kobo: number;
  resolved_cogs_kobo: number;
  gross_margin_kobo: number;
  unresolved_units: number;
}

// reportGrossMargin wraps GET /api/v1/reports/gross-margin (reports:read).
export async function reportGrossMargin(
  businessId: string,
  startAt: string,
  endAt: string,
): Promise<GrossMargin[]> {
  const raw = await apiRequest<RawGrossMargin[]>(
    `/api/v1/reports/gross-margin?${rangeParams(startAt, endAt)}`,
    {
      headers: { 'X-Business-ID': businessId },
    },
  );
  return raw.map((r) => ({
    variantId: r.variant_id,
    variantName: r.variant_name,
    productName: r.product_name,
    revenueKobo: r.revenue_kobo,
    resolvedCogsKobo: r.resolved_cogs_kobo,
    grossMarginKobo: r.gross_margin_kobo,
    unresolvedUnits: r.unresolved_units,
  }));
}
