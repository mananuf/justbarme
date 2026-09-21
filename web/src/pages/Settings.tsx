import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';

import {
  getBusiness,
  updateBusinessSettings,
  uploadBusinessLogo,
  type BusinessSettings,
} from '../api/business';
import {
  createCategory,
  listCategories,
  listProducts,
  updateCategory,
  updateProduct,
  updateVariant,
  setVariantPrice,
  type CatalogueProduct,
  type CatalogueVariant,
  type Category,
} from '../api/catalogue';
import { describeActionError } from '../lib/errors';
import { useSession } from '../lib/session';

// Owner-only Settings page: business branding + logo (docs/PHASE_PILOT_
// RELEASE.md §3, §5) and catalogue editing (renaming a product/category,
// deactivating something, editing a price outside the stock-receiving
// flow -- the long-standing gap CLAUDE.md's Phase 3 section flagged).
// Reuses the already-built internal/catalogue endpoints; nothing new on
// the backend for the catalogue half.
export function Settings() {
  const { csrfToken, selectedBusinessId } = useSession();

  const [business, setBusiness] = useState<BusinessSettings | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);

  const [phone, setPhone] = useState('');
  const [address, setAddress] = useState('');
  const [receiptWording, setReceiptWording] = useState('');
  const [receiptFooter, setReceiptFooter] = useState('');
  const [paymentInstructions, setPaymentInstructions] = useState('');
  const [savingBranding, setSavingBranding] = useState(false);
  const [brandingError, setBrandingError] = useState<string | null>(null);
  const [brandingSaved, setBrandingSaved] = useState(false);

  const [uploadingLogo, setUploadingLogo] = useState(false);
  const [logoError, setLogoError] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const [categories, setCategories] = useState<Category[] | null>(null);
  const [products, setProducts] = useState<CatalogueProduct[] | null>(null);
  const [catalogueError, setCatalogueError] = useState<string | null>(null);
  const [newCategoryName, setNewCategoryName] = useState('');
  const [savingCategory, setSavingCategory] = useState(false);
  const [busyItemId, setBusyItemId] = useState<string | null>(null);

  useEffect(() => {
    if (!selectedBusinessId) return;
    getBusiness(selectedBusinessId)
      .then((b) => {
        setBusiness(b);
        setPhone(b.phone);
        setAddress(b.address);
        setReceiptWording(b.receiptWording);
        setReceiptFooter(b.receiptFooter);
        setPaymentInstructions(b.paymentInstructions);
      })
      .catch((err: unknown) =>
        setLoadError(describeActionError(err, 'Could not load business settings.')),
      );
  }, [selectedBusinessId]);

  const loadCatalogue = () => {
    if (!selectedBusinessId) return;
    listCategories(selectedBusinessId)
      .then(setCategories)
      .catch((err: unknown) =>
        setCatalogueError(describeActionError(err, 'Could not load categories.')),
      );
    listProducts(selectedBusinessId)
      .then(setProducts)
      .catch((err: unknown) =>
        setCatalogueError(describeActionError(err, 'Could not load products.')),
      );
  };
  useEffect(loadCatalogue, [selectedBusinessId]);

  async function handleSaveBranding() {
    if (!selectedBusinessId || !csrfToken) return;
    setSavingBranding(true);
    setBrandingError(null);
    setBrandingSaved(false);
    try {
      const updated = await updateBusinessSettings(
        { phone, address, receiptWording, receiptFooter, paymentInstructions },
        selectedBusinessId,
        csrfToken,
      );
      setBusiness(updated);
      setBrandingSaved(true);
    } catch (err) {
      setBrandingError(describeActionError(err, 'Could not save these settings.'));
    } finally {
      setSavingBranding(false);
    }
  }

  async function handleLogoSelected(file: File | undefined) {
    if (!file || !selectedBusinessId || !csrfToken) return;
    setUploadingLogo(true);
    setLogoError(null);
    try {
      const updated = await uploadBusinessLogo(file, selectedBusinessId, csrfToken);
      setBusiness(updated);
    } catch (err) {
      setLogoError(describeActionError(err, 'Could not upload this logo.'));
    } finally {
      setUploadingLogo(false);
      if (fileInputRef.current) fileInputRef.current.value = '';
    }
  }

  async function handleAddCategory() {
    if (!selectedBusinessId || !csrfToken || !newCategoryName.trim()) return;
    setSavingCategory(true);
    setCatalogueError(null);
    try {
      const created = await createCategory(
        newCategoryName.trim(),
        categories?.length ?? 0,
        selectedBusinessId,
        csrfToken,
      );
      setCategories((prev) => [...(prev ?? []), created]);
      setNewCategoryName('');
    } catch (err) {
      setCatalogueError(describeActionError(err, 'Could not create this category.'));
    } finally {
      setSavingCategory(false);
    }
  }

  async function handleRenameCategory(category: Category, name: string) {
    if (!selectedBusinessId || !csrfToken || !name.trim() || name === category.name) return;
    setBusyItemId(category.id);
    try {
      const updated = await updateCategory(
        category.id,
        { name: name.trim(), sortOrder: category.sortOrder, active: category.active },
        selectedBusinessId,
        csrfToken,
      );
      setCategories((prev) => (prev ?? []).map((c) => (c.id === updated.id ? updated : c)));
    } catch (err) {
      setCatalogueError(describeActionError(err, 'Could not rename this category.'));
    } finally {
      setBusyItemId(null);
    }
  }

  async function handleToggleCategoryActive(category: Category) {
    if (!selectedBusinessId || !csrfToken) return;
    setBusyItemId(category.id);
    try {
      const updated = await updateCategory(
        category.id,
        { name: category.name, sortOrder: category.sortOrder, active: !category.active },
        selectedBusinessId,
        csrfToken,
      );
      setCategories((prev) => (prev ?? []).map((c) => (c.id === updated.id ? updated : c)));
    } catch (err) {
      setCatalogueError(describeActionError(err, 'Could not update this category.'));
    } finally {
      setBusyItemId(null);
    }
  }

  async function handleRenameProduct(product: CatalogueProduct, name: string) {
    if (!selectedBusinessId || !csrfToken || !name.trim() || name === product.name) return;
    setBusyItemId(product.id);
    try {
      await updateProduct(
        product.id,
        { name: name.trim(), categoryId: product.categoryId, active: true },
        selectedBusinessId,
        csrfToken,
      );
      loadCatalogue();
    } catch (err) {
      setCatalogueError(describeActionError(err, 'Could not rename this product.'));
    } finally {
      setBusyItemId(null);
    }
  }

  async function handleToggleProductActive(product: CatalogueProduct, active: boolean) {
    if (!selectedBusinessId || !csrfToken) return;
    setBusyItemId(product.id);
    try {
      await updateProduct(
        product.id,
        { name: product.name, categoryId: product.categoryId, active },
        selectedBusinessId,
        csrfToken,
      );
      loadCatalogue();
    } catch (err) {
      setCatalogueError(describeActionError(err, 'Could not update this product.'));
    } finally {
      setBusyItemId(null);
    }
  }

  async function handleRenameVariant(variant: CatalogueVariant, name: string) {
    if (!selectedBusinessId || !csrfToken || !name.trim() || name === variant.name) return;
    setBusyItemId(variant.id);
    try {
      await updateVariant(
        variant.id,
        { name: name.trim(), active: variant.active, tracksInventory: variant.tracksInventory },
        selectedBusinessId,
        csrfToken,
      );
      loadCatalogue();
    } catch (err) {
      setCatalogueError(describeActionError(err, 'Could not rename this item.'));
    } finally {
      setBusyItemId(null);
    }
  }

  async function handleToggleVariantActive(variant: CatalogueVariant) {
    if (!selectedBusinessId || !csrfToken) return;
    setBusyItemId(variant.id);
    try {
      await updateVariant(
        variant.id,
        { name: variant.name, active: !variant.active, tracksInventory: variant.tracksInventory },
        selectedBusinessId,
        csrfToken,
      );
      loadCatalogue();
    } catch (err) {
      setCatalogueError(describeActionError(err, 'Could not update this item.'));
    } finally {
      setBusyItemId(null);
    }
  }

  async function handleToggleVariantTracksInventory(variant: CatalogueVariant) {
    if (!selectedBusinessId || !csrfToken) return;
    setBusyItemId(variant.id);
    try {
      await updateVariant(
        variant.id,
        { name: variant.name, active: variant.active, tracksInventory: !variant.tracksInventory },
        selectedBusinessId,
        csrfToken,
      );
      loadCatalogue();
    } catch (err) {
      setCatalogueError(describeActionError(err, 'Could not update this item.'));
    } finally {
      setBusyItemId(null);
    }
  }

  async function handleChangePrice(variantId: string, priceText: string) {
    if (!selectedBusinessId || !csrfToken) return;
    const amountKobo = Math.round(parseFloat(priceText) * 100);
    if (!amountKobo || amountKobo <= 0) return;
    setBusyItemId(variantId);
    try {
      await setVariantPrice(variantId, amountKobo, selectedBusinessId, csrfToken);
      loadCatalogue();
    } catch (err) {
      setCatalogueError(describeActionError(err, 'Could not update this price.'));
    } finally {
      setBusyItemId(null);
    }
  }

  return (
    <div className="min-h-screen bg-jb-cream text-jb-ink pb-16">
      <div className="max-w-md mx-auto px-5 pt-6">
        <div className="flex items-center gap-4 mb-6">
          <Link
            to="/dashboard"
            aria-label="Back"
            className="text-jb-ink/40 hover:text-jb-ink transition-colors text-lg leading-none"
          >
            ←
          </Link>
          <div>
            <div className="text-[11px] text-jb-ink/45">Dashboard</div>
            <h1 className="text-lg font-medium">Settings</h1>
          </div>
        </div>

        {loadError && <p className="text-[13px] text-red-700 mb-3">{loadError}</p>}

        {business && (
          <div className="rounded-xl border border-jb-ink/10 bg-white/60 p-4 mb-6 space-y-3">
            <div className="text-[11px] text-jb-ink/45 tracking-wide">BRANDING</div>

            <div className="flex items-center gap-3">
              {business.logoUrl ? (
                <img
                  src={business.logoUrl}
                  alt=""
                  className="h-14 w-14 rounded-lg object-cover border border-jb-ink/10"
                />
              ) : (
                <div className="h-14 w-14 rounded-lg bg-jb-ink/5 border border-jb-ink/10 flex items-center justify-center text-[10px] text-jb-ink/30">
                  No logo
                </div>
              )}
              <div>
                <button
                  onClick={() => fileInputRef.current?.click()}
                  disabled={uploadingLogo}
                  className="text-[13px] font-medium text-jb-green disabled:opacity-40"
                >
                  {uploadingLogo ? 'Uploading…' : 'Change logo'}
                </button>
                <input
                  ref={fileInputRef}
                  type="file"
                  accept="image/png,image/jpeg,image/webp"
                  className="hidden"
                  onChange={(e) => void handleLogoSelected(e.target.files?.[0])}
                />
                <p className="text-[11px] text-jb-ink/40">PNG, JPEG, or WebP — up to 2MB</p>
              </div>
            </div>
            {logoError && <p className="text-[12px] text-red-700">{logoError}</p>}

            <input
              type="tel"
              placeholder="Phone"
              value={phone}
              onChange={(e) => setPhone(e.target.value)}
              className="w-full rounded-lg border border-jb-ink/15 bg-white px-3 py-2.5 text-[13.5px] focus:outline-none focus:border-jb-ink/40"
            />
            <input
              type="text"
              placeholder="Address"
              value={address}
              onChange={(e) => setAddress(e.target.value)}
              className="w-full rounded-lg border border-jb-ink/15 bg-white px-3 py-2.5 text-[13.5px] focus:outline-none focus:border-jb-ink/40"
            />
            <input
              type="text"
              placeholder="Receipt greeting (e.g. Thank you for your patronage!)"
              value={receiptWording}
              onChange={(e) => setReceiptWording(e.target.value)}
              className="w-full rounded-lg border border-jb-ink/15 bg-white px-3 py-2.5 text-[13.5px] focus:outline-none focus:border-jb-ink/40"
            />
            <input
              type="text"
              placeholder="Receipt footer (e.g. See you again soon)"
              value={receiptFooter}
              onChange={(e) => setReceiptFooter(e.target.value)}
              className="w-full rounded-lg border border-jb-ink/15 bg-white px-3 py-2.5 text-[13.5px] focus:outline-none focus:border-jb-ink/40"
            />
            <textarea
              placeholder="Payment instructions (e.g. Bank transfer to ...)"
              value={paymentInstructions}
              onChange={(e) => setPaymentInstructions(e.target.value)}
              rows={2}
              className="w-full rounded-lg border border-jb-ink/15 bg-white px-3 py-2.5 text-[13.5px] focus:outline-none focus:border-jb-ink/40"
            />

            {brandingError && <p className="text-[12px] text-red-700">{brandingError}</p>}
            {brandingSaved && !brandingError && <p className="text-[12px] text-jb-green">Saved.</p>}
            <button
              onClick={() => void handleSaveBranding()}
              disabled={savingBranding}
              className="w-full rounded-lg bg-jb-ink text-jb-cream text-[14px] font-medium py-2.5 hover:bg-jb-green transition-colors disabled:opacity-40"
            >
              {savingBranding ? 'Saving…' : 'Save'}
            </button>
          </div>
        )}

        <div className="mb-2 text-[11px] text-jb-ink/45 tracking-wide">CATALOGUE</div>
        {catalogueError && <p className="text-[13px] text-red-700 mb-2">{catalogueError}</p>}

        <div className="rounded-xl border border-jb-ink/10 bg-white/60 p-4 mb-4">
          <div className="flex gap-2">
            <input
              type="text"
              placeholder="New category name"
              value={newCategoryName}
              onChange={(e) => setNewCategoryName(e.target.value)}
              className="flex-1 rounded-lg border border-jb-ink/15 bg-white px-3 py-2 text-[13.5px] focus:outline-none focus:border-jb-ink/40"
            />
            <button
              onClick={() => void handleAddCategory()}
              disabled={savingCategory || !newCategoryName.trim()}
              className="rounded-lg bg-jb-ink text-jb-cream text-[13px] font-medium px-4 disabled:opacity-40"
            >
              Add
            </button>
          </div>
        </div>

        {categories === null && <p className="text-[13px] text-jb-ink/40 mb-4">Loading…</p>}
        {categories !== null && categories.length > 0 && (
          <div className="rounded-xl border border-jb-ink/10 bg-white/60 divide-y divide-jb-ink/[0.06] overflow-hidden mb-4">
            {categories.map((category) => (
              <div key={category.id} className="px-4 py-3 flex items-center gap-2">
                <input
                  type="text"
                  defaultValue={category.name}
                  onBlur={(e) => void handleRenameCategory(category, e.target.value)}
                  disabled={busyItemId === category.id}
                  className={`flex-1 bg-transparent text-[13.5px] focus:outline-none ${category.active ? '' : 'line-through text-jb-ink/40'}`}
                />
                <button
                  onClick={() => void handleToggleCategoryActive(category)}
                  disabled={busyItemId === category.id}
                  className="text-[12px] text-jb-ink/50 disabled:opacity-40 shrink-0"
                >
                  {category.active ? 'Deactivate' : 'Activate'}
                </button>
              </div>
            ))}
          </div>
        )}

        {products === null && <p className="text-[13px] text-jb-ink/40">Loading products…</p>}
        {products !== null && products.length === 0 && (
          <p className="text-[13px] text-jb-ink/40">No products yet.</p>
        )}
        {(products ?? []).map((product) => (
          <div key={product.id} className="rounded-xl border border-jb-ink/10 bg-white/60 p-4 mb-3">
            <div className="flex items-center gap-2 mb-2">
              <input
                type="text"
                defaultValue={product.name}
                onBlur={(e) => void handleRenameProduct(product, e.target.value)}
                disabled={busyItemId === product.id}
                className="flex-1 bg-transparent text-[14px] font-medium focus:outline-none"
              />
              <button
                onClick={() => void handleToggleProductActive(product, false)}
                disabled={busyItemId === product.id}
                className="text-[12px] text-jb-ink/50 disabled:opacity-40 shrink-0"
              >
                Deactivate
              </button>
            </div>
            <div className="space-y-1.5">
              {product.variants.map((variant) => (
                <div key={variant.id} className="pl-2">
                  <div className="flex items-center gap-2">
                    <input
                      type="text"
                      defaultValue={variant.name}
                      onBlur={(e) => void handleRenameVariant(variant, e.target.value)}
                      disabled={busyItemId === variant.id}
                      className={`flex-1 bg-transparent text-[13px] focus:outline-none ${variant.active ? '' : 'line-through text-jb-ink/40'}`}
                    />
                    <div className="flex items-center gap-1 shrink-0">
                      <span className="text-jb-ink/40 text-[12.5px]">₦</span>
                      <input
                        type="number"
                        inputMode="decimal"
                        defaultValue={variant.currentPriceKobo / 100}
                        onBlur={(e) => void handleChangePrice(variant.id, e.target.value)}
                        disabled={busyItemId === variant.id}
                        className="w-16 bg-transparent text-[12.5px] text-right focus:outline-none"
                      />
                    </div>
                    <button
                      onClick={() => void handleToggleVariantActive(variant)}
                      disabled={busyItemId === variant.id}
                      className="text-[11px] text-jb-ink/40 disabled:opacity-40 shrink-0"
                    >
                      {variant.active ? 'Off' : 'On'}
                    </button>
                  </div>
                  <button
                    onClick={() => void handleToggleVariantTracksInventory(variant)}
                    disabled={busyItemId === variant.id}
                    className="text-[10.5px] text-jb-ink/35 disabled:opacity-40 mt-0.5"
                  >
                    {variant.tracksInventory
                      ? 'Tracks stock — mark as a service instead'
                      : 'Service item (no stock tracking) — mark as stocked instead'}
                  </button>
                </div>
              ))}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
