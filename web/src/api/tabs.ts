import { apiRequest } from './client';
import type { PaymentMethod, SaleItemInput } from './sales';

export interface Table {
  id: string;
  label: string;
  active: boolean;
}

export interface Customer {
  id: string;
  name: string;
  phone?: string;
  email?: string;
  notes?: string;
}

export interface Bill {
  id: string;
  status: 'open' | 'closed_unpaid' | 'settled' | 'void';
  tableId: string | null;
  customerId: string | null;
  balanceKobo: number;
  openedAt: string;
}

export interface BillRound {
  id: string;
  occurredAt: string;
  totalKobo: number;
  // Whoever recorded this round -- shown next to the timestamp so more
  // than one staff member working the same tab can tell their own rounds
  // apart. "" if the lookup somehow came back empty; never absent.
  sellerName: string;
  items: {
    variantId: string;
    description: string;
    quantity: number;
    unitPriceKobo: number;
    lineTotalKobo: number;
  }[];
}

export interface BillPayment {
  amountKobo: number;
  method: string;
}

export interface BillWriteOff {
  id: string;
  amountKobo: number;
  reason: string;
  createdAt: string;
}

export interface BillDetail extends Bill {
  sales: BillRound[];
  payments: BillPayment[];
  writeOffs: BillWriteOff[];
}

interface RawTable {
  id: string;
  label: string;
  active: boolean;
}

interface RawCustomer {
  id: string;
  name: string;
  phone?: string;
  email?: string;
  notes?: string;
}

interface RawBill {
  id: string;
  status: Bill['status'];
  table_id?: string;
  customer_id?: string;
  balance_kobo: number;
  opened_at: string;
}

interface RawSaleItem {
  variant_id: string;
  description: string;
  quantity: number;
  unit_price_kobo: number;
  line_total_kobo: number;
}

interface RawRound {
  id: string;
  occurred_at: string;
  total_kobo: number;
  seller_name?: string;
  items?: RawSaleItem[];
}

interface RawPayment {
  amount_kobo: number;
  method: string;
}

interface RawWriteOff {
  id: string;
  amount_kobo: number;
  reason: string;
  created_at: string;
}

interface RawBillDetail extends RawBill {
  sales?: RawRound[];
  payments?: RawPayment[];
  write_offs?: RawWriteOff[];
}

function toTable(t: RawTable): Table {
  return { id: t.id, label: t.label, active: t.active };
}

function toCustomer(c: RawCustomer): Customer {
  return { id: c.id, name: c.name, phone: c.phone, email: c.email, notes: c.notes };
}

function toBill(b: RawBill): Bill {
  return {
    id: b.id,
    status: b.status,
    tableId: b.table_id ?? null,
    customerId: b.customer_id ?? null,
    balanceKobo: b.balance_kobo,
    openedAt: b.opened_at,
  };
}

function toBillDetail(b: RawBillDetail): BillDetail {
  return {
    ...toBill(b),
    sales: (b.sales ?? []).map((s) => ({
      id: s.id,
      occurredAt: s.occurred_at,
      totalKobo: s.total_kobo,
      sellerName: s.seller_name ?? '',
      items: (s.items ?? []).map((i) => ({
        variantId: i.variant_id,
        description: i.description,
        quantity: i.quantity,
        unitPriceKobo: i.unit_price_kobo,
        lineTotalKobo: i.line_total_kobo,
      })),
    })),
    payments: (b.payments ?? []).map((p) => ({ amountKobo: p.amount_kobo, method: p.method })),
    writeOffs: (b.write_offs ?? []).map((w) => ({
      id: w.id,
      amountKobo: w.amount_kobo,
      reason: w.reason,
      createdAt: w.created_at,
    })),
  };
}

export async function listTables(businessId: string): Promise<Table[]> {
  const raw = await apiRequest<RawTable[]>('/api/v1/tables', {
    headers: { 'X-Business-ID': businessId },
  });
  return raw.map(toTable);
}

export async function createTable(
  label: string,
  businessId: string,
  csrfToken: string,
): Promise<Table> {
  const raw = await apiRequest<RawTable>('/api/v1/tables', {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken, 'X-Business-ID': businessId },
    body: JSON.stringify({ label }),
  });
  return toTable(raw);
}

export async function listCustomers(businessId: string): Promise<Customer[]> {
  const raw = await apiRequest<RawCustomer[]>('/api/v1/customers', {
    headers: { 'X-Business-ID': businessId },
  });
  return raw.map(toCustomer);
}

export async function createCustomer(
  input: { name: string; phone?: string; email?: string; notes?: string },
  businessId: string,
  csrfToken: string,
): Promise<Customer> {
  const raw = await apiRequest<RawCustomer>('/api/v1/customers', {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken, 'X-Business-ID': businessId },
    body: JSON.stringify(input),
  });
  return toCustomer(raw);
}

export async function openBill(
  input: { tableId?: string; customerId?: string },
  businessId: string,
  csrfToken: string,
): Promise<Bill> {
  const raw = await apiRequest<RawBill>('/api/v1/bills', {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken, 'X-Business-ID': businessId },
    body: JSON.stringify({ table_id: input.tableId, customer_id: input.customerId }),
  });
  return toBill(raw);
}

// listBills wraps GET /api/v1/bills. status 'open' (default) returns open
// and closed_unpaid tabs; 'outstanding' returns every bill with a nonzero
// balance regardless of status.
export async function listBills(
  businessId: string,
  status: 'open' | 'outstanding' = 'open',
): Promise<Bill[]> {
  const raw = await apiRequest<RawBill[]>(`/api/v1/bills?status=${status}`, {
    headers: { 'X-Business-ID': businessId },
  });
  return raw.map(toBill);
}

export async function getBillDetail(billId: string, businessId: string): Promise<BillDetail> {
  const raw = await apiRequest<RawBillDetail>(`/api/v1/bills/${billId}`, {
    headers: { 'X-Business-ID': businessId },
  });
  return toBillDetail(raw);
}

export async function addSaleRound(
  billId: string,
  input: { idempotencyKey: string; occurredAt: string; items: SaleItemInput[] },
  businessId: string,
  csrfToken: string,
): Promise<BillRound> {
  const raw = await apiRequest<RawRound>(`/api/v1/bills/${billId}/rounds`, {
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
    }),
  });
  return {
    id: raw.id,
    occurredAt: raw.occurred_at,
    totalKobo: raw.total_kobo,
    sellerName: raw.seller_name ?? '',
    items: (raw.items ?? []).map((i) => ({
      variantId: i.variant_id,
      description: i.description,
      quantity: i.quantity,
      unitPriceKobo: i.unit_price_kobo,
      lineTotalKobo: i.line_total_kobo,
    })),
  };
}

// removeBillItem posts a compensating negative round that takes quantity
// units of one variant off billId -- never edits/deletes a previously
// posted round. Only allowed while the bill is open or closed_unpaid.
export async function removeBillItem(
  billId: string,
  input: {
    idempotencyKey: string;
    occurredAt: string;
    variantId: string;
    quantity: number;
    unitPriceKobo: number;
  },
  businessId: string,
  csrfToken: string,
): Promise<BillRound> {
  const raw = await apiRequest<RawRound>(`/api/v1/bills/${billId}/items/remove`, {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken, 'X-Business-ID': businessId },
    body: JSON.stringify({
      idempotency_key: input.idempotencyKey,
      occurred_at: input.occurredAt,
      variant_id: input.variantId,
      quantity: input.quantity,
      unit_price_kobo: input.unitPriceKobo,
    }),
  });
  return {
    id: raw.id,
    occurredAt: raw.occurred_at,
    totalKobo: raw.total_kobo,
    sellerName: raw.seller_name ?? '',
    items: (raw.items ?? []).map((i) => ({
      variantId: i.variant_id,
      description: i.description,
      quantity: i.quantity,
      unitPriceKobo: i.unit_price_kobo,
      lineTotalKobo: i.line_total_kobo,
    })),
  };
}

export async function closeBill(
  billId: string,
  businessId: string,
  csrfToken: string,
): Promise<Bill> {
  const raw = await apiRequest<RawBill>(`/api/v1/bills/${billId}/close`, {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken, 'X-Business-ID': businessId },
  });
  return toBill(raw);
}

export async function voidBill(
  billId: string,
  businessId: string,
  csrfToken: string,
): Promise<Bill> {
  const raw = await apiRequest<RawBill>(`/api/v1/bills/${billId}/void`, {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken, 'X-Business-ID': businessId },
  });
  return toBill(raw);
}

export async function writeOffBill(
  billId: string,
  amountKobo: number,
  reason: string,
  businessId: string,
  csrfToken: string,
): Promise<Bill> {
  const raw = await apiRequest<RawBill>(`/api/v1/bills/${billId}/write-off`, {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken, 'X-Business-ID': businessId },
    body: JSON.stringify({ amount_kobo: amountKobo, reason }),
  });
  return toBill(raw);
}

export async function recordPayment(
  billId: string,
  amountKobo: number,
  method: PaymentMethod,
  businessId: string,
  csrfToken: string,
): Promise<BillPayment> {
  const raw = await apiRequest<RawPayment>(`/api/v1/bills/${billId}/payments`, {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken, 'X-Business-ID': businessId },
    body: JSON.stringify({ amount_kobo: amountKobo, method }),
  });
  return { amountKobo: raw.amount_kobo, method: raw.method };
}

export interface BillShareLink {
  token: string;
  url: string;
}

interface RawBillShareLink {
  token: string;
  url: string;
}

// createOrRotateShareLink creates a public bill-share link, or replaces
// the existing one if the bill already has one -- see
// docs/PHASE_PILOT_RELEASE.md §4. The returned token is only ever shown
// here, at creation; calling this again produces a new, different token
// and the previous one stops working immediately.
export async function createOrRotateShareLink(
  billId: string,
  businessId: string,
  csrfToken: string,
): Promise<BillShareLink> {
  const raw = await apiRequest<RawBillShareLink>(`/api/v1/bills/${billId}/share-link`, {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken, 'X-Business-ID': businessId },
    body: JSON.stringify({}),
  });
  return { token: raw.token, url: raw.url };
}

export async function revokeShareLink(
  billId: string,
  businessId: string,
  csrfToken: string,
): Promise<void> {
  await apiRequest<{ revoked: boolean }>(`/api/v1/bills/${billId}/share-link/revoke`, {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken, 'X-Business-ID': businessId },
    body: JSON.stringify({}),
  });
}
