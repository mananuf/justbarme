import { apiRequest } from './client';

export interface CatalogueTemplateVariant {
  id: string;
  name: string;
  suggestedPriceKobo: number;
}

export interface CatalogueTemplate {
  id: string;
  name: string;
  categoryName: string;
  variants: CatalogueTemplateVariant[];
}

interface RawTemplateVariant {
  id: string;
  name: string;
  suggested_price_kobo: number;
}

interface RawTemplate {
  id: string;
  name: string;
  category_name: string;
  variants: RawTemplateVariant[];
}

// listCatalogueTemplates wraps GET /api/v1/catalogue-templates -- platform
// reference data, needs auth only, no X-Business-ID (see
// internal/httpapi/catalogue_template_handlers.go).
export async function listCatalogueTemplates(): Promise<CatalogueTemplate[]> {
  const raw = await apiRequest<RawTemplate[]>('/api/v1/catalogue-templates');
  return raw.map((t) => ({
    id: t.id,
    name: t.name,
    categoryName: t.category_name,
    variants: t.variants.map((v) => ({
      id: v.id,
      name: v.name,
      suggestedPriceKobo: v.suggested_price_kobo,
    })),
  }));
}

export interface AppliedVariant {
  id: string;
  name: string;
  currentPriceKobo: number;
}

export interface AppliedProduct {
  id: string;
  name: string;
  variants: AppliedVariant[];
}

interface RawVariant {
  id: string;
  name: string;
  current_price: { amount_kobo: number };
}

interface RawProduct {
  id: string;
  name: string;
  variants: RawVariant[];
}

// applyCatalogueTemplates wraps POST /api/v1/catalogue-templates/apply --
// creates business-owned categories/products/variants/prices from the
// selected platform templates, each variant starting at its template's
// suggested price. There is no way to override a price in this same call;
// use setVariantPrice afterward for anything the owner wants to change.
export async function applyCatalogueTemplates(
  templateIds: string[],
  businessId: string,
  csrfToken: string,
): Promise<AppliedProduct[]> {
  const raw = await apiRequest<RawProduct[]>('/api/v1/catalogue-templates/apply', {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken, 'X-Business-ID': businessId },
    body: JSON.stringify({ template_ids: templateIds }),
  });
  return raw.map((p) => ({
    id: p.id,
    name: p.name,
    variants: p.variants.map((v) => ({
      id: v.id,
      name: v.name,
      currentPriceKobo: v.current_price.amount_kobo,
    })),
  }));
}

// setVariantPrice wraps POST /api/v1/variants/{variant_id}/prices.
export async function setVariantPrice(
  variantId: string,
  amountKobo: number,
  businessId: string,
  csrfToken: string,
): Promise<void> {
  await apiRequest(`/api/v1/variants/${variantId}/prices`, {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken, 'X-Business-ID': businessId },
    body: JSON.stringify({ amount_kobo: amountKobo }),
  });
}
