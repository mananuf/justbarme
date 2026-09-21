import { apiRequest } from './client';

export interface PublicBillItem {
  description: string;
  quantity: number;
  unitPriceKobo: number;
  lineTotalKobo: number;
}

export interface PublicBill {
  businessName: string;
  logoUrl: string;
  phone: string;
  address: string;
  receiptWording: string;
  receiptFooter: string;
  paymentInstructions: string;
  status: string;
  openedAt: string;
  items: PublicBillItem[];
  totalKobo: number;
  balanceKobo: number;
}

interface RawPublicBill {
  business_name: string;
  logo_url: string;
  phone: string;
  address: string;
  receipt_wording: string;
  receipt_footer: string;
  payment_instructions: string;
  status: string;
  opened_at: string;
  items: {
    description: string;
    quantity: number;
    unit_price_kobo: number;
    line_total_kobo: number;
  }[];
  total_kobo: number;
  balance_kobo: number;
}

// getPublicBill fetches the branded, read-only view behind a bill share
// link -- fully public, no session or business context, per
// docs/PHASE_PILOT_RELEASE.md §4. Never call this with credentials the
// caller isn't meant to have; the token itself is the access control.
export async function getPublicBill(token: string): Promise<PublicBill> {
  const raw = await apiRequest<RawPublicBill>(`/api/v1/public/bills/${token}`);
  return {
    businessName: raw.business_name,
    logoUrl: raw.logo_url,
    phone: raw.phone,
    address: raw.address,
    receiptWording: raw.receipt_wording,
    receiptFooter: raw.receipt_footer,
    paymentInstructions: raw.payment_instructions,
    status: raw.status,
    openedAt: raw.opened_at,
    items: raw.items.map((item) => ({
      description: item.description,
      quantity: item.quantity,
      unitPriceKobo: item.unit_price_kobo,
      lineTotalKobo: item.line_total_kobo,
    })),
    totalKobo: raw.total_kobo,
    balanceKobo: raw.balance_kobo,
  };
}
