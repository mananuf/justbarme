import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';

import { ApiError, OfflineError } from '../api/client';
import { getPublicBill, type PublicBill } from '../api/publicBill';
import { Logo } from '../components/Logo';
import { describeActionError } from '../lib/errors';

function formatNaira(kobo: number): string {
  return (kobo / 100).toLocaleString();
}

// Public, unauthenticated branded bill view behind a share link
// (docs/PHASE_PILOT_RELEASE.md §4) -- no RequireAuth, no session
// assumptions. Deliberately shows only what the backend already scrubbed
// (items, total, business branding): no customer name, no seller name, no
// table label.
export function BillReceipt() {
  const { token = '' } = useParams();
  const [bill, setBill] = useState<PublicBill | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const result = await getPublicBill(token);
        if (!cancelled) setBill(result);
      } catch (err) {
        if (cancelled) return;
        if (err instanceof ApiError && err.status === 404) {
          setError('This link is no longer valid. Ask for a new one.');
        } else if (err instanceof OfflineError) {
          setError(err.message);
        } else {
          setError(describeActionError(err, 'Could not load this bill.'));
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [token]);

  return (
    <main className="min-h-screen bg-jb-cream text-jb-ink flex flex-col items-center px-4 py-10">
      <div className="w-full max-w-sm">
        <div className="flex justify-center mb-6">
          <Logo className="h-8 w-auto text-jb-ink" />
        </div>

        {loading && <p className="text-center text-sm text-jb-ink/60">Loading…</p>}

        {!loading && error && (
          <div className="rounded-2xl bg-white border border-jb-ink/10 p-6 text-center space-y-3">
            <p className="text-sm text-jb-ink/70">{error}</p>
            <Link to="/" className="text-sm font-medium text-jb-green underline underline-offset-2">
              Go to justbarme
            </Link>
          </div>
        )}

        {!loading && !error && bill && (
          <div className="rounded-2xl bg-white border border-jb-ink/10 overflow-hidden">
            <div className="px-6 py-5 border-b border-jb-ink/10 text-center">
              {bill.logoUrl && (
                <img
                  src={bill.logoUrl}
                  alt=""
                  className="h-12 w-12 mx-auto mb-2 rounded-lg object-cover"
                />
              )}
              <h1 className="text-lg font-medium">{bill.businessName}</h1>
              {bill.address && <p className="text-xs text-jb-ink/60 mt-1">{bill.address}</p>}
              {bill.phone && <p className="text-xs text-jb-ink/60">{bill.phone}</p>}
              {bill.receiptWording && (
                <p className="text-sm text-jb-ink/80 mt-3">{bill.receiptWording}</p>
              )}
            </div>

            <div className="px-6 py-4 space-y-2">
              {bill.items.map((item, i) => (
                <div key={i} className="flex justify-between text-sm gap-3">
                  <span className="flex-1">
                    {item.description}
                    <span className="text-jb-ink/50"> ×{item.quantity}</span>
                  </span>
                  <span className="tabular-nums">₦{formatNaira(item.lineTotalKobo)}</span>
                </div>
              ))}
            </div>

            <div className="px-6 py-4 border-t border-jb-ink/10 space-y-1">
              <div className="flex justify-between text-sm font-medium">
                <span>Total</span>
                <span className="tabular-nums">₦{formatNaira(bill.totalKobo)}</span>
              </div>
              {bill.balanceKobo > 0 && (
                <div className="flex justify-between text-sm text-jb-gold">
                  <span>Outstanding</span>
                  <span className="tabular-nums">₦{formatNaira(bill.balanceKobo)}</span>
                </div>
              )}
              {bill.balanceKobo <= 0 && (
                <div className="flex justify-between text-sm text-jb-green">
                  <span>Status</span>
                  <span>Paid</span>
                </div>
              )}
            </div>

            {bill.paymentInstructions && (
              // whitespace-pre-line: this is the one free-text field in
              // Settings that's a <textarea>, not a single-line <input> --
              // an owner types their own line breaks (e.g. "BANK: ...",
              // "ACCOUNT: ...", "NAME: ..." each on its own line), and a
              // plain <div> collapses those, running them into one line on
              // the actual customer-facing receipt.
              <div className="px-6 py-4 border-t border-jb-ink/10 text-sm text-jb-ink/70 whitespace-pre-line">
                {bill.paymentInstructions}
              </div>
            )}

            {bill.receiptFooter && (
              <div className="px-6 py-4 border-t border-jb-ink/10 text-center text-xs text-jb-ink/50">
                {bill.receiptFooter}
              </div>
            )}
          </div>
        )}
      </div>
    </main>
  );
}
