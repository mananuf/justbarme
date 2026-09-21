import { apiRequest } from './client';

export interface BusinessSettings {
  id: string;
  name: string;
  timezone: string;
  currency: string;
  phone: string;
  address: string;
  receiptWording: string;
  receiptFooter: string;
  paymentInstructions: string;
  logoUrl: string;
}

interface RawBusinessSettings {
  id: string;
  name: string;
  timezone: string;
  currency: string;
  phone: string;
  address: string;
  receipt_wording: string;
  receipt_footer: string;
  payment_instructions: string;
  logo_url: string;
}

function toBusinessSettings(b: RawBusinessSettings): BusinessSettings {
  return {
    id: b.id,
    name: b.name,
    timezone: b.timezone,
    currency: b.currency,
    phone: b.phone,
    address: b.address,
    receiptWording: b.receipt_wording,
    receiptFooter: b.receipt_footer,
    paymentInstructions: b.payment_instructions,
    logoUrl: b.logo_url,
  };
}

// getBusiness wraps GET /api/v1/business (business:read -- both roles).
export async function getBusiness(businessId: string): Promise<BusinessSettings> {
  const raw = await apiRequest<RawBusinessSettings>('/api/v1/business', {
    headers: { 'X-Business-ID': businessId },
  });
  return toBusinessSettings(raw);
}

export interface BrandingInput {
  phone: string;
  address: string;
  receiptWording: string;
  receiptFooter: string;
  paymentInstructions: string;
}

// updateBusinessSettings wraps PATCH /api/v1/business (business:update --
// owner-only). A full replacement of the branding fields, matching this
// codebase's PATCH convention -- see docs/PHASE_PILOT_RELEASE.md §5.
export async function updateBusinessSettings(
  input: BrandingInput,
  businessId: string,
  csrfToken: string,
): Promise<BusinessSettings> {
  const raw = await apiRequest<RawBusinessSettings>('/api/v1/business', {
    method: 'PATCH',
    headers: { 'X-CSRF-Token': csrfToken, 'X-Business-ID': businessId },
    body: JSON.stringify({
      phone: input.phone,
      address: input.address,
      receipt_wording: input.receiptWording,
      receipt_footer: input.receiptFooter,
      payment_instructions: input.paymentInstructions,
    }),
  });
  return toBusinessSettings(raw);
}

// uploadBusinessLogo wraps POST /api/v1/business/logo (business:update --
// owner-only). Sends the raw file bytes with no multipart wrapper, per
// the backend's own convention -- see internal/business.Service.UpdateLogo
// for the server-side validation (real decode, not just a declared
// content type, max 2MB, re-encoded to PNG).
export async function uploadBusinessLogo(
  file: Blob,
  businessId: string,
  csrfToken: string,
): Promise<BusinessSettings> {
  const raw = await apiRequest<RawBusinessSettings>('/api/v1/business/logo', {
    method: 'POST',
    // apiRequest defaults an unset Content-Type to application/json for
    // any non-null body -- set the real one explicitly here so a binary
    // image upload isn't mislabeled (the backend doesn't actually inspect
    // this header, but sending it correctly is still the honest thing).
    headers: {
      'X-CSRF-Token': csrfToken,
      'X-Business-ID': businessId,
      'Content-Type': file.type || 'application/octet-stream',
    },
    body: file,
  });
  return toBusinessSettings(raw);
}
