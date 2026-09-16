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

export interface CatalogueVariant {
  id: string;
  name: string;
  active: boolean;
  currentPriceKobo: number;
  // From internal/inventory, merged in server-side -- see
  // docs/PHASE_STOCK_RECEIVING.md. Always 0 for a variant with no stock
  // receipts yet.
  currentStock: number;
}

export interface CatalogueProduct {
  id: string;
  name: string;
  categoryId: string | null;
  variants: CatalogueVariant[];
}

interface RawVariant {
  id: string;
  name: string;
  active: boolean;
  current_price: { amount_kobo: number };
  current_stock: number;
}

interface RawProduct {
  id: string;
  name: string;
  category_id: string | null;
  variants: RawVariant[];
}

function toCatalogueProduct(p: RawProduct): CatalogueProduct {
  return {
    id: p.id,
    name: p.name,
    categoryId: p.category_id,
    variants: p.variants.map((v) => ({
      id: v.id,
      name: v.name,
      active: v.active,
      currentPriceKobo: v.current_price.amount_kobo,
      currentStock: v.current_stock,
    })),
  };
}

// listProducts wraps GET /api/v1/products -- every product the business
// already sells, with each variant's current price and current stock in
// one call (no separate stock-list endpoint; see
// docs/PHASE_STOCK_RECEIVING.md for why that data is merged in here rather
// than fetched separately).
export async function listProducts(businessId: string): Promise<CatalogueProduct[]> {
  const raw = await apiRequest<RawProduct[]>('/api/v1/products', {
    headers: { 'X-Business-ID': businessId },
  });
  return raw.map(toCatalogueProduct);
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
): Promise<CatalogueProduct[]> {
  const raw = await apiRequest<RawProduct[]>('/api/v1/catalogue-templates/apply', {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken, 'X-Business-ID': businessId },
    body: JSON.stringify({ template_ids: templateIds }),
  });
  return raw.map(toCatalogueProduct);
}

// createProduct wraps POST /api/v1/products -- for a drink that isn't in
// the platform template list at all.
export async function createProduct(
  name: string,
  businessId: string,
  csrfToken: string,
): Promise<CatalogueProduct> {
  const raw = await apiRequest<RawProduct>('/api/v1/products', {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken, 'X-Business-ID': businessId },
    body: JSON.stringify({ name }),
  });
  return toCatalogueProduct(raw);
}

// createVariant wraps POST /api/v1/products/{product_id}/variants -- a
// product can never exist without at least one priced variant, so this is
// always the second call right after createProduct for a brand-new custom
// product.
export async function createVariant(
  productId: string,
  name: string,
  initialPriceKobo: number,
  businessId: string,
  csrfToken: string,
): Promise<CatalogueVariant> {
  const raw = await apiRequest<RawVariant>(`/api/v1/products/${productId}/variants`, {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken, 'X-Business-ID': businessId },
    body: JSON.stringify({ name, initial_price_kobo: initialPriceKobo }),
  });
  return {
    id: raw.id,
    name: raw.name,
    active: raw.active,
    currentPriceKobo: raw.current_price.amount_kobo,
    currentStock: raw.current_stock,
  };
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
